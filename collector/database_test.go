// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		name         string
		invalidUntil *time.Time
		wantNil      bool
	}{
		{
			name:         "Nil invalidUntil",
			invalidUntil: nil,
			wantNil:      true,
		},
		{
			name:         "Future invalidUntil",
			invalidUntil: func() *time.Time { t := time.Now().Add(time.Minute); return &t }(),
			wantNil:      false,
		},
		{
			name:         "Past invalidUntil",
			invalidUntil: func() *time.Time { t := time.Now().Add(-time.Minute); return &t }(),
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &Database{invalidUntil: tt.invalidUntil}
			result := db.IsValid()
			if tt.wantNil {
				if result != nil {
					t.Fatalf("expected nil retryAfter, got %v", *result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil retryAfter")
			}
			if *result <= 0 {
				t.Fatalf("expected positive retryAfter, got %v", *result)
			}
		})
	}
}

func TestInvalidate(t *testing.T) {
	db := &Database{}
	backoff := time.Minute
	db.invalidate(backoff)
	if db.invalidUntil == nil {
		t.Fatal("Expected non-nil invalidUntil")
	}
	if time.Now().After(*db.invalidUntil) {
		t.Error("Expected invalidUntil in the future")
	}
}

func TestClearInvalid(t *testing.T) {
	db := &Database{}
	db.invalidate(time.Minute)
	db.clearInvalid()
	if db.invalidUntil != nil {
		t.Fatal("Expected invalidUntil to be cleared")
	}
}

func TestIsClosedDatabaseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sql err conn done",
			err:  sql.ErrConnDone,
			want: true,
		},
		{
			name: "closed database text",
			err:  errors.New("sql: database is closed"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("other"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClosedDatabaseError(tt.err); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestWarmupConnectionPoolWithNilSessionSetsStartupReadyAndBackoff(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &Database{}

	err := db.WarmupConnectionPool(logger, time.Minute)
	if err == nil {
		t.Fatal("expected warmup to fail for nil session")
	}
	if !db.StartupReady() {
		t.Fatal("expected startupReady to be true after warmup attempt")
	}
	if db.invalidUntil == nil {
		t.Fatal("expected invalidUntil to be set after warmup failure")
	}
	if db.Up != 0 {
		t.Fatalf("expected database up metric to remain 0, got %v", db.Up)
	}
}

func TestScrapeDatabaseSkipsWhileStartupInProgress(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exporter := &Exporter{
		logger:               logger,
		MetricsConfiguration: &MetricsConfiguration{},
		databaseDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "database_duration_seconds",
				Help:      "test",
			},
			[]string{"database"},
		),
	}
	database := &Database{
		Name:          "db1",
		DatabaseLabel: "database",
	}
	errChan := make(chan error, 1)
	metricCh := make(chan prometheus.Metric, 1)
	now := time.Now()

	exporter.scrapeDatabase(metricCh, errChan, database, &now)

	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("expected nil error while startup is in progress, got %v", err)
		}
	default:
		t.Fatal("expected scrapeDatabase to send an error result")
	}

	select {
	case <-metricCh:
		t.Fatal("did not expect metrics while startup is in progress")
	default:
	}
}
