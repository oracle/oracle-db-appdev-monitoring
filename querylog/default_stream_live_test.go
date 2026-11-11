// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
	"github.com/oracle/oracle-db-appdev-monitoring/querylog"
)

// These opt-in tests only read diagnostic data. Session NLS changes are confined
// to their own connection, which is closed with the test's database pool.
func TestAlertSourceLiveRead(t *testing.T) {
	url := os.Getenv("ORACLEDB_TEST_URL")
	if url == "" {
		t.Skip("ORACLEDB_TEST_URL is not set")
	}
	one := 1
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := db.NewDatabase(logger, "database", "log-integration", config.DatabaseConfig{URL: url, Username: os.Getenv("ORACLEDB_TEST_USER"), Password: os.Getenv("ORACLEDB_TEST_PASSWORD"), ConnectConfig: config.ConnectConfig{MaxOpenConns: &one, MaxIdleConns: &one}}, config.ClientInfoConfig{})
	defer database.Close()
	if err := database.WarmupConnectionPool(logger, time.Second); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	yes := true
	cfg := &config.MetricsConfiguration{}
	cfg.Logging.LogLevelEnabled = &yes
	streams, err := config.LoadLogStreams(cfg)
	if err != nil {
		t.Fatal(err)
	}
	source := &querylog.SQLSource{Database: database, Query: streams[0].Query}
	for _, language := range []string{"AMERICAN", "GERMAN"} {
		t.Run(language, func(t *testing.T) {
			conn, release, err := database.ConnectionContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = conn.ExecContext(ctx, "alter session set NLS_DATE_LANGUAGE = '"+language+"'")
			if err == nil {
				_, err = conn.ExecContext(ctx, "alter session set NLS_NUMERIC_CHARACTERS = ',.'")
			}
			closeErr := release()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			reader, err := source.Open(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			_, err = reader.Read(ctx, reader.Now().Add(-time.Minute), reader.Now(), 10000, 16<<20)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
