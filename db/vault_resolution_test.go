// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func TestWarmupConnectionPoolWithConfigurationLookupErrorUsesBackoff(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := NewDatabase(logger, "database", "db1", config.DatabaseConfig{
		URL:          "dbhost/service",
		PasswordFile: filepath.Join(t.TempDir(), "missing-password"),
	})

	if db.Session != nil {
		t.Fatal("expected session initialization to fail when configuration lookup fails")
	}

	err := db.WarmupConnectionPool(logger, time.Minute)
	if err == nil {
		t.Fatal("expected warmup to fail after configuration lookup error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing password file error to be preserved, got %v", err)
	}
	if db.invalidUntil == nil {
		t.Fatal("expected invalidUntil to be set after configuration lookup failure")
	}
}
