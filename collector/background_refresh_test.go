// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
)

func TestBackgroundRefreshServesCachedValuesOnRequest(t *testing.T) {
	exporter, database, gate := newTestBackgroundRefreshExporter(t, false, 2, "test")
	metric := exporter.metricsToScrape["test_value"]

	// Nothing is cached yet: the first scrape returns without the metric and refreshes it.
	if values := collectWithin(t, exporter); hasValue(values, scheduledTestMetricName) {
		t.Fatalf("did not expect %s before its first refresh completed, got %v", scheduledTestMetricName, values)
	}
	waitForRefreshes(t, exporter)
	if got := collectWithin(t, exporter)[scheduledTestMetricName]; got != 1 {
		t.Fatalf("expected the refreshed value 1, got %v", got)
	}

	// A due metric is served from the cache while its slow query runs.
	markDue(exporter, database, metric)
	gate.value.Store("2")
	gate.hold.Store(true)
	if got := collectWithin(t, exporter)[scheduledTestMetricName]; got != 1 {
		t.Fatalf("expected the cached value 1 while the metric refreshes, got %v", got)
	}
	waitForQueryStart(t, gate)
	if got := collectWithin(t, exporter)[scheduledTestMetricName]; got != 1 {
		t.Fatalf("expected the cached value 1 while the metric refreshes, got %v", got)
	}
	if got := gate.queries.Load(); got != 2 {
		t.Fatalf("expected a single query per refresh, got %d queries", got)
	}

	gate.open()
	waitForRefreshes(t, exporter)
	if got := collectWithin(t, exporter)[scheduledTestMetricName]; got != 2 {
		t.Fatalf("expected the refreshed value 2, got %v", got)
	}
}

func TestBackgroundRefreshDoesNotHoldUpScheduledScrapes(t *testing.T) {
	exporter, database, gate := newTestBackgroundRefreshExporter(t, true, 2, "test")
	metric := exporter.metricsToScrape["test_value"]

	scrapeWithin(t, exporter)
	waitForRefreshes(t, exporter)
	scrapeWithin(t, exporter)
	if got := collectScheduledValues(exporter)[scheduledTestMetricName]; got != 1 {
		t.Fatalf("expected the refreshed value 1, got %v", got)
	}

	markDue(exporter, database, metric)
	gate.value.Store("2")
	gate.hold.Store(true)
	scrapeWithin(t, exporter)
	waitForQueryStart(t, gate)
	if got := collectScheduledValues(exporter)[scheduledTestMetricName]; got != 1 {
		t.Fatalf("expected the cached value 1 while the metric refreshes, got %v", got)
	}

	gate.open()
	waitForRefreshes(t, exporter)
	scrapeWithin(t, exporter)
	if got := collectScheduledValues(exporter)[scheduledTestMetricName]; got != 2 {
		t.Fatalf("expected the refreshed value 2, got %v", got)
	}
}

func TestBackgroundRefreshReportsAndRetriesFailures(t *testing.T) {
	exporter, _, gate := newTestBackgroundRefreshExporter(t, false, 2, "test")
	gate.fail.Store(true)

	collectWithin(t, exporter)
	waitForRefreshes(t, exporter)

	// The failed refresh is reported by the next scrape, which also retries it.
	values := collectWithin(t, exporter)
	if got := values["oracledb_exporter_last_scrape_error"]; got != 1 {
		t.Fatalf("expected the failed refresh to be reported as a scrape error, got %v", got)
	}
	if got := values["oracledb_exporter_scrape_errors_total"]; got < 1 {
		t.Fatalf("expected the failed refresh to be counted, got %v", got)
	}
	waitForRefreshes(t, exporter)
	if got := gate.queries.Load(); got != 2 {
		t.Fatalf("expected the failed refresh to be retried, got %d queries", got)
	}
}

// Background refreshes must leave a connection free, or a scrape waits for one to ping the database.
func TestBackgroundRefreshKeepsAConnectionForTheScrape(t *testing.T) {
	exporter, _, gate := newTestBackgroundRefreshExporter(t, false, 2, "test", "other")
	gate.hold.Store(true)

	collectWithin(t, exporter)
	waitForQueryStart(t, gate)
	if got := collectWithin(t, exporter)["oracledb_up"]; got != 1 {
		t.Fatalf("expected the scrape to ping the database while refreshes run, got oracledb_up=%v", got)
	}
	if got := gate.queries.Load(); got != 1 {
		t.Fatalf("expected one background refresh per free connection, got %d queries", got)
	}

	gate.open()
	waitForRefreshes(t, exporter)
	if got := gate.queries.Load(); got != 2 {
		t.Fatalf("expected the waiting refresh to run once a connection was free, got %d queries", got)
	}
}

func TestStopRefreshesCancelsRunningRefreshes(t *testing.T) {
	exporter, _, gate := newTestBackgroundRefreshExporter(t, false, 2, "test")
	gate.hold.Store(true)

	collectWithin(t, exporter)
	waitForQueryStart(t, gate)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := exporter.StopRefreshes(ctx); err != nil {
		t.Fatalf("expected the running refresh to be cancelled, got %v", err)
	}

	collectWithin(t, exporter)
	if got := gate.queries.Load(); got != 1 {
		t.Fatalf("did not expect new refreshes after StopRefreshes, got %d queries", got)
	}
}

// A refresh that completes after a metrics reload must not report the duration of a metric that may
// no longer exist.
func TestBackgroundRefreshAfterReloadDoesNotReportDuration(t *testing.T) {
	exporter, _, gate := newTestBackgroundRefreshExporter(t, false, 2, "test")
	gate.hold.Store(true)

	collectWithin(t, exporter)
	waitForQueryStart(t, gate)
	// Rebuild the caches and reset the durations, as reloadMetrics does.
	exporter.initCache()
	exporter.metricScrapeDuration.Reset()
	gate.open()
	waitForRefreshes(t, exporter)

	if series := gaugeSeries(t, exporter.metricScrapeDuration); len(series) != 0 {
		t.Fatalf("expected no duration series after the reload, got %v", series)
	}
}

func TestMetricsScrapedDuringTheScrapeWithoutBackgroundRefresh(t *testing.T) {
	tests := []struct {
		name   string
		adjust func(*Exporter, *config.Metric)
	}{
		{
			name:   "metric without a custom scrape interval",
			adjust: func(_ *Exporter, metric *config.Metric) { metric.ScrapeInterval = "" },
		},
		{
			name: "background refresh disabled",
			adjust: func(exporter *Exporter, _ *config.Metric) {
				disabled := false
				exporter.MetricsConfiguration.Metrics.BackgroundRefresh.Enabled = &disabled
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, _, gate := newTestBackgroundRefreshExporter(t, false, 2, "test")
			tt.adjust(exporter, exporter.metricsToScrape["test_value"])

			if got := collectWithin(t, exporter)[scheduledTestMetricName]; got != 1 {
				t.Fatalf("expected the first scrape to return the queried value 1, got %v", got)
			}
			if got := gate.queries.Load(); got != 1 {
				t.Fatalf("expected the metric to be queried during the scrape, got %d queries", got)
			}
		})
	}
}

// newTestBackgroundRefreshExporter returns an exporter that refreshes metrics with a custom scrape
// interval in the background, and the gate controlling its database queries. Each context defines
// one metric, <context>_value, with a 1m scrape interval.
func newTestBackgroundRefreshExporter(t *testing.T, scheduled bool, maxOpenConns int, contexts ...string) (*Exporter, *db.Database, *queryGate) {
	t.Helper()

	exporter, database := newTestScheduledExporter(t, time.Hour)
	if !scheduled {
		exporter.MetricsConfiguration.Metrics.ScrapeInterval = nil
	}
	enabled := true
	exporter.MetricsConfiguration.Metrics.BackgroundRefresh.Enabled = &enabled
	exporter.metricsToScrape = map[string]*config.Metric{}
	for _, c := range contexts {
		metric := &config.Metric{
			ID:             c + "_value",
			Context:        c,
			MetricsDesc:    map[string]string{"value": "Test metric."},
			MetricsType:    map[string]string{"value": "gauge"},
			Request:        "select 1 as value from dual",
			ScrapeInterval: "1m",
		}
		exporter.metricsToScrape[metric.ID] = metric
	}
	exporter.initCache()

	gate := newQueryGate()
	gate.value.Store("1")
	session := sql.OpenDB(gatedQueryConnector{gate: gate})
	t.Cleanup(func() { _ = session.Close() })
	database.Session = session
	database.Config.ConnectConfig.MaxOpenConns = &maxOpenConns
	if err := database.WarmupConnectionPool(testLogger(), time.Hour); err != nil {
		t.Fatalf("expected test database warmup to succeed, got %v", err)
	}
	// Warmup checks the database role with a query.
	gate.queries.Store(0)
	t.Cleanup(func() {
		gate.open()
		if err := exporter.StopRefreshes(context.Background()); err != nil {
			t.Errorf("expected background refreshes to stop, got %v", err)
		}
	})
	return exporter, database, gate
}

func markDue(exporter *Exporter, database *db.Database, metric *config.Metric) {
	past := time.Now().Add(-2 * time.Minute)
	exporter.metricsCache(database).SetLastScraped(metric, &past)
}

func hasValue(values map[string]float64, name string) bool {
	_, ok := values[name]
	return ok
}

// collectWithin runs Collect, failing the test if it waits for a background refresh.
func collectWithin(t *testing.T, exporter *Exporter) map[string]float64 {
	t.Helper()

	collected := make(chan map[string]float64, 1)
	go func() { collected <- collectScheduledValues(exporter) }()
	select {
	case values := <-collected:
		return values
	case <-time.After(time.Second):
		t.Fatal("Collect waited for a background refresh")
		return nil
	}
}

// scrapeWithin runs a scheduled scrape, failing the test if it waits for a background refresh.
func scrapeWithin(t *testing.T, exporter *Exporter) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		exporter.doScrape(time.Now())
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduled scrape waited for a background refresh")
	}
}

func waitForQueryStart(t *testing.T, gate *queryGate) {
	t.Helper()

	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a background refresh to start its query")
	}
}

func waitForRefreshes(t *testing.T, exporter *Exporter) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		exporter.refreshes.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background refreshes")
	}
}
