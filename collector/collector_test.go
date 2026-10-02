// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/db"
	"github.com/prometheus/client_golang/prometheus"
)

var errTestRowsIteration = errors.New("row iteration failed")

type testRowsWithIterationError struct {
	read bool
}

func (r *testRowsWithIterationError) CloneTestRows() driver.Rows {
	return &testRowsWithIterationError{}
}

func (r *testRowsWithIterationError) Columns() []string {
	return []string{"value"}
}

func (r *testRowsWithIterationError) Close() error {
	return nil
}

func (r *testRowsWithIterationError) Next(dest []driver.Value) error {
	if r.read {
		return errTestRowsIteration
	}
	r.read = true
	dest[0] = "1"
	return nil
}

func TestDuplicatedLabels(t *testing.T) {
	tests := []struct {
		name        string
		constLabels map[string]string
		labels      []string
		expected    bool
	}{
		{
			name:        "No overlap",
			constLabels: map[string]string{"env": "prod"},
			labels:      []string{"service", "instance"},
			expected:    false,
		},
		{
			name:        "Overlap",
			constLabels: map[string]string{"env": "prod", "service": "app"},
			labels:      []string{"service", "instance"},
			expected:    true,
		},
		{
			name:        "Multiple overlaps",
			constLabels: map[string]string{"env": "prod"},
			labels:      []string{"env", "service"},
			expected:    true,
		},
		{
			name:        "Empty constLabels",
			constLabels: map[string]string{},
			labels:      []string{"service"},
			expected:    false,
		},
		{
			name:        "Empty labels",
			constLabels: map[string]string{"env": "prod"},
			labels:      []string{},
			expected:    false,
		},
		{
			name:        "Both empty",
			constLabels: map[string]string{},
			labels:      []string{},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := duplicatedLabels(tt.constLabels, tt.labels)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetMetricTypeDefaultsToGaugeForUnknownType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	got := getMetricType(logger, "value", map[string]string{
		"value": "totally-unknown",
	})

	if got != prometheus.GaugeValue {
		t.Fatalf("expected unknown metric type to default to gauge, got %v", got)
	}
}

func TestGeneratePrometheusMetricsReturnsRowsErr(t *testing.T) {
	exporter := &Exporter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	db := &db.Database{
		Session: openTestQueryDBWithRows(t, &testRowsWithIterationError{}),
	}
	parseCalls := 0

	err := exporter.generatePrometheusMetrics(db, func(row map[string]string) error {
		parseCalls++
		if row["value"] != "1" {
			t.Fatalf("expected row value to be scanned before iteration error, got %q", row["value"])
		}
		return nil
	}, "select 1 from dual", time.Second)

	if !errors.Is(err, errTestRowsIteration) {
		t.Fatalf("expected rows iteration error, got %v", err)
	}
	if parseCalls != 1 {
		t.Fatalf("expected parse to be called once before iteration error, got %d", parseCalls)
	}
}

// deadlineConnector models a driver returning rows as the query deadline expires.
type deadlineConnector struct{}

func (deadlineConnector) Connect(context.Context) (driver.Conn, error) {
	return deadlineConn{}, nil
}

func (deadlineConnector) Driver() driver.Driver { return testQueryDriver{} }

type deadlineConn struct{ testQueryConn }

func (deadlineConn) QueryContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	<-ctx.Done()
	return &testQueryRows{}, nil
}

func TestQueryDeadlineReleasesLifecycleLock(t *testing.T) {
	pool := sql.OpenDB(deadlineConnector{})
	defer pool.Close()
	database := &db.Database{Session: pool}
	err := (&Exporter{}).generatePrometheusMetrics(database, func(map[string]string) error { return nil }, "select 1", time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected query deadline, got %v", err)
	}
	done := make(chan struct{})
	go func() {
		// Ping updates lifecycle state under the write lock, so it must finish.
		_ = database.Ping(testLogger(), time.Minute)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("query deadline leaked lifecycle lock")
	}
}
