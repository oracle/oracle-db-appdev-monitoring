// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build oracledb && !goora

package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

// Run against a disposable database with ORACLEDB_TEST_URL, ORACLEDB_TEST_USER,
// and ORACLEDB_TEST_PASSWORD set. Ordinary unit-test runs do not need a database.
func TestOracleDBLiveSessions(t *testing.T) {
	url := os.Getenv("ORACLEDB_TEST_URL")
	if url == "" {
		t.Skip("ORACLEDB_TEST_URL is not set")
	}
	cfg := config.DatabaseConfig{URL: url, Username: os.Getenv("ORACLEDB_TEST_USER"), Password: os.Getenv("ORACLEDB_TEST_PASSWORD")}
	pool, err := connectContext(context.Background(), testLogger(), "integration", cfg, config.ClientInfoConfig{Label: "exporter's integration test"})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	check := func(conn *sql.Conn) {
		t.Helper()
		var label string
		var number, fraction float64
		if err := conn.QueryRowContext(ctx, "SELECT SYS_CONTEXT('USERENV', 'CLIENT_INFO'), 42.5, CAST(1/3 AS BINARY_DOUBLE) FROM DUAL").Scan(&label, &number, &fraction); err != nil {
			t.Fatal(err)
		}
		if label != "exporter's integration test" || number != 42.5 || fraction < 0.333333333333333 || fraction > 0.333333333333334 {
			t.Fatalf("unexpected row: %q, %v", label, number)
		}
	}
	first, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	check(first)
	check(second)
	first.Close()
	second.Close()
	pool.SetMaxIdleConns(0)
	replacement, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	check(replacement)
}
