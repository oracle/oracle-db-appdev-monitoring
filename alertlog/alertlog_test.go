// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package alertlog

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"io"
	"log/slog"

	"github.com/oracle/oracle-db-appdev-monitoring/collector"
)

func TestNullStringValue(t *testing.T) {
	t.Run("returns string for valid value", func(t *testing.T) {
		got := nullStringValue(sql.NullString{String: "ecid-123", Valid: true})
		if got != "ecid-123" {
			t.Fatalf("expected valid null string to unwrap, got %q", got)
		}
	})

	t.Run("returns empty string for null value", func(t *testing.T) {
		got := nullStringValue(sql.NullString{})
		if got != "" {
			t.Fatalf("expected null string to convert to empty string, got %q", got)
		}
	})
}

func TestLogDestinationForDatabase(t *testing.T) {
	t.Run("returns shared destination when disabled", func(t *testing.T) {
		got := logDestinationForDatabase("/log/alert.log", "db2", false)
		if got != "/log/alert.log" {
			t.Fatalf("expected shared destination, got %q", got)
		}
	})

	t.Run("inserts database name before extension", func(t *testing.T) {
		got := logDestinationForDatabase("/log/alert.log", "db2", true)
		if got != "/log/alert-db2.log" {
			t.Fatalf("expected per-database destination, got %q", got)
		}
	})

	t.Run("supports paths without extension", func(t *testing.T) {
		got := logDestinationForDatabase("/log/alert", "db2", true)
		if got != "/log/alert-db2" {
			t.Fatalf("expected per-database destination without extension, got %q", got)
		}
	})
}

func TestRetryTrackerShouldRetry(t *testing.T) {
	var tracker retryTracker
	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)

	shouldRetry, retryAfter := tracker.shouldRetry("db2", now)
	if !shouldRetry {
		t.Fatalf("expected retry to be allowed for unknown database, got retry_after=%v", retryAfter)
	}

	retryAfter = tracker.recordFailure("db2", now)
	if retryAfter != now.Add(initialRetryBackoff) {
		t.Fatalf("expected first backoff to end at %v, got %v", now.Add(initialRetryBackoff), retryAfter)
	}

	shouldRetry, retryAfter = tracker.shouldRetry("db2", now.Add(30*time.Second))
	if shouldRetry {
		t.Fatal("expected retry to be blocked during backoff")
	}

	shouldRetry, retryAfter = tracker.shouldRetry("db2", now.Add(initialRetryBackoff))
	if !shouldRetry {
		t.Fatalf("expected retry to resume after backoff, got retry_after=%v", retryAfter)
	}
}

func TestReadLastMatchingLogRecord(t *testing.T) {
	t.Run("shared log returns latest record for matching database", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "alert.log")
		content := "" +
			"{\"timestamp\":\"2026-03-13T12:00:00.000Z\",\"database\":\"db1\",\"moduleId\":\"\",\"ecid\":\"\",\"message\":\"db1-old\"}\n" +
			"{\"timestamp\":\"2026-03-13T12:05:00.000Z\",\"database\":\"db2\",\"moduleId\":\"\",\"ecid\":\"\",\"message\":\"db2-newest\"}\n" +
			"{\"timestamp\":\"2026-03-13T12:03:00.000Z\",\"database\":\"db1\",\"moduleId\":\"\",\"ecid\":\"\",\"message\":\"db1-newest\"}\n"
		if err := os.WriteFile(logPath, []byte(content), 0600); err != nil {
			t.Fatalf("write log: %v", err)
		}

		record, err := readLastMatchingLogRecord(logPath, "db1", false)
		if err != nil {
			t.Fatalf("read last matching log record: %v", err)
		}
		if record.Timestamp != "2026-03-13T12:03:00.000Z" {
			t.Fatalf("expected latest db1 timestamp, got %q", record.Timestamp)
		}
	})

	t.Run("shared log falls back when no matching database exists", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "alert.log")
		content := "{\"timestamp\":\"2026-03-13T12:05:00.000Z\",\"database\":\"db2\",\"moduleId\":\"\",\"ecid\":\"\",\"message\":\"db2-only\"}\n"
		if err := os.WriteFile(logPath, []byte(content), 0600); err != nil {
			t.Fatalf("write log: %v", err)
		}

		record, err := readLastMatchingLogRecord(logPath, "db1", false)
		if err != nil {
			t.Fatalf("read last matching log record: %v", err)
		}
		if record.Timestamp != defaultLastLogRecord.Timestamp {
			t.Fatalf("expected default timestamp fallback, got %q", record.Timestamp)
		}
	})

	t.Run("per-database file uses latest line without filtering", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "alert-db1.log")
		content := "" +
			"{\"timestamp\":\"2026-03-13T12:00:00.000Z\",\"database\":\"db1\",\"moduleId\":\"\",\"ecid\":\"\",\"message\":\"old\"}\n" +
			"{\"timestamp\":\"2026-03-13T12:03:00.000Z\",\"database\":\"db1\",\"moduleId\":\"\",\"ecid\":\"\",\"message\":\"new\"}\n"
		if err := os.WriteFile(logPath, []byte(content), 0600); err != nil {
			t.Fatalf("write log: %v", err)
		}

		record, err := readLastMatchingLogRecord(logPath, "db1", true)
		if err != nil {
			t.Fatalf("read last matching log record: %v", err)
		}
		if record.Timestamp != "2026-03-13T12:03:00.000Z" {
			t.Fatalf("expected latest timestamp from per-database file, got %q", record.Timestamp)
		}
	})
}

func TestRetryTrackerBackoffCaps(t *testing.T) {
	var tracker retryTracker
	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)

	var retryAfter time.Time
	for i := 0; i < 10; i++ {
		retryAfter = tracker.recordFailure("db2", now)
	}

	if retryAfter != now.Add(maxRetryBackoff) {
		t.Fatalf("expected backoff to cap at %v, got %v", now.Add(maxRetryBackoff), retryAfter)
	}
}

func TestRetryTrackerRecordSuccessResetsState(t *testing.T) {
	var tracker retryTracker
	now := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)

	tracker.recordFailure("db2", now)
	tracker.recordSuccess("db2")

	shouldRetry, retryAfter := tracker.shouldRetry("db2", now)
	if !shouldRetry {
		t.Fatalf("expected retry to be allowed after success reset, got retry_after=%v", retryAfter)
	}
}

func TestUpdateLogSkipsWhenStartupNotReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	logPath := filepath.Join(t.TempDir(), "alert.log")
	db := &collector.Database{Name: "db1"}

	UpdateLog(logPath, false, logger, db)

	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("expected log file to not be created while startup is in progress, got err=%v", err)
	}
}
