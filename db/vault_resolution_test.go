// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func TestEnsureSessionRetriesFailedStartupCredentialLookup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := NewDatabase(logger, "database", "db1", config.DatabaseConfig{
		URL:          "dbhost/service",
		PasswordFile: filepath.Join(t.TempDir(), "missing-password"),
	}, config.ClientInfoConfig{})

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
	if err := db.EnsureSession(t.Context()); !errors.Is(err, ErrConnectionNotReady) {
		t.Fatalf("expected acquisition to honor connection backoff, got %v", err)
	}
	db.clearInvalid()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := db.EnsureSession(ctx); !errors.Is(err, context.Canceled) || db.IsValid() != nil {
		t.Fatalf("canceled recovery should leave backoff clear, got %v", err)
	}
	if err := db.EnsureSession(t.Context()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected a fresh credential lookup without a metrics scrape, got %v", err)
	}
	if db.IsValid() == nil {
		t.Fatal("failed recovery should start connection backoff")
	}
}
