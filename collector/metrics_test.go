// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
)

func TestIsScrapeMetricCustomInterval(t *testing.T) {
	const interval = time.Minute
	tests := []struct {
		name    string
		elapsed time.Duration
		want    bool
	}{
		{name: "well within the interval", elapsed: 50 * time.Second, want: false},
		{name: "short of the interval by more than the tolerance", elapsed: interval - scrapeIntervalTolerance - time.Millisecond, want: false},
		{name: "short of the interval by tick jitter", elapsed: interval - 5*time.Microsecond, want: true},
		{name: "exactly the interval", elapsed: interval, want: true},
		{name: "past the interval", elapsed: interval + 10*time.Second, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, database, metric := newTestIntervalExporter(interval.String())
			lastScraped := time.Now()
			exporter.metricsCache(database).SetLastScraped(metric, &lastScraped)
			tick := lastScraped.Add(tt.elapsed)

			if got := exporter.isScrapeMetric(testLogger(), &tick, metric, database); got != tt.want {
				t.Fatalf("expected isScrapeMetric to be %t after %v, got %t", tt.want, tt.elapsed, got)
			}
		})
	}
}

// Scheduled ticks are aligned to metrics.scrapeInterval but their timestamps carry microsecond
// jitter. A metric whose custom interval is a multiple of the tick must refresh on every due tick,
// not one tick late (every 70s instead of every 60s with a 10s tick).
func TestIsScrapeMetricKeepsCadenceWithTickJitter(t *testing.T) {
	const tick = 10 * time.Second
	exporter, database, metric := newTestIntervalExporter("60s")
	start := time.Now()

	var refreshes []time.Duration
	for i := 0; i <= 60; i++ {
		jitter := 3 * time.Microsecond
		if i%2 == 1 {
			jitter = -jitter
		}
		tickTime := start.Add(time.Duration(i)*tick + jitter)
		if exporter.isScrapeMetric(testLogger(), &tickTime, metric, database) {
			exporter.metricsCache(database).SetLastScraped(metric, &tickTime)
			refreshes = append(refreshes, tickTime.Sub(start))
		}
	}

	if len(refreshes) != 11 {
		t.Fatalf("expected 11 refreshes over 10 minutes, got %d: %v", len(refreshes), refreshes)
	}
	for i := 1; i < len(refreshes); i++ {
		if got := (refreshes[i] - refreshes[i-1]).Round(tick); got != time.Minute {
			t.Fatalf("expected refreshes every minute, got %v between %v and %v", got, refreshes[i-1], refreshes[i])
		}
	}
}

func newTestIntervalExporter(scrapeInterval string) (*Exporter, *db.Database, *config.Metric) {
	metric := newTestDurationMetric(scrapeInterval)
	database := &db.Database{Name: "db1"}
	exporter := &Exporter{
		metricsToScrape: map[string]*config.Metric{metric.ID: metric},
		databases:       []*db.Database{database},
	}
	exporter.initCache()
	return exporter, database, metric
}

func TestParseFloat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name     string
		metric   string
		row      map[string]string
		expected float64
		ok       bool
	}{
		{
			name:     "parses valid float",
			metric:   "value",
			row:      map[string]string{"value": "42.5"},
			expected: 42.5,
			ok:       true,
		},
		{
			name:     "trims whitespace",
			metric:   "value",
			row:      map[string]string{"value": "  7.25  "},
			expected: 7.25,
			ok:       true,
		},
		{
			name:     "treats nil string as zero",
			metric:   "value",
			row:      map[string]string{"value": "<nil>"},
			expected: 0,
			ok:       true,
		},
		{
			name:     "returns zero and false when key is missing",
			metric:   "value",
			row:      map[string]string{},
			expected: 0,
			ok:       false,
		},
		{
			name:     "returns error sentinel for invalid float",
			metric:   "value",
			row:      map[string]string{"value": "not-a-number"},
			expected: -1,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseFloat(logger, tt.metric, "help", tt.row)
			if got != tt.expected || ok != tt.ok {
				t.Fatalf("expected (%v, %v), got (%v, %v)", tt.expected, tt.ok, got, ok)
			}
		})
	}
}
