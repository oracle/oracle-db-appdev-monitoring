// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const metricScrapeDurationName = "oracledb_exporter_last_metric_scrape_duration_seconds"

func TestScrapeDatabaseRecordsMetricScrapeDuration(t *testing.T) {
	tests := []struct {
		name       string
		rows       driver.Rows
		wantResult string
	}{
		{name: "successful scrape", rows: nil, wantResult: scrapeResultSuccess},
		{name: "failed scrape", rows: &testRowsWithIterationError{}, wantResult: scrapeResultError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := newTestDurationMetric("")
			exporter, database := newTestDurationExporter(t, openTestQueryDBWithRows(t, tt.rows), metric)

			runScrapeDatabase(t, exporter, database)

			series := gaugeSeries(t, exporter.metricScrapeDuration)
			if len(series) != 1 {
				t.Fatalf("expected exactly one duration series, got %d: %v", len(series), series)
			}
			wantKey := durationKey(metric.Context, metric.ID, database.Name, tt.wantResult)
			value, ok := series[wantKey]
			if !ok {
				t.Fatalf("expected duration series %q, got %v", wantKey, series)
			}
			if value < 0 {
				t.Fatalf("expected a non-negative duration for %q, got %v", wantKey, value)
			}
		})
	}
}

func TestScrapeDatabaseReplacesPreviousScrapeResult(t *testing.T) {
	metric := newTestDurationMetric("")
	exporter, database := newTestDurationExporter(t, openTestQueryDBWithRows(t, nil), metric)

	// Seed the opposite result, as a previously failing scrape would.
	exporter.observeMetricScrapeDuration(database, metric, scrapeResultError, 5*time.Second)

	runScrapeDatabase(t, exporter, database)

	series := gaugeSeries(t, exporter.metricScrapeDuration)
	if len(series) != 1 {
		t.Fatalf("expected the stale error series to be removed, got %d series: %v", len(series), series)
	}
	if _, ok := series[durationKey(metric.Context, metric.ID, database.Name, scrapeResultSuccess)]; !ok {
		t.Fatalf("expected only a success series after a successful scrape, got %v", series)
	}
}

func TestScrapeDatabaseKeepsDurationForSkippedMetric(t *testing.T) {
	metric := newTestDurationMetric("1h")
	exporter, database := newTestDurationExporter(t, openTestQueryDBWithRows(t, nil), metric)

	const seeded = 1.5
	exporter.observeMetricScrapeDuration(database, metric, scrapeResultSuccess, 1500*time.Millisecond)
	// The metric was just scraped, so its custom scrape interval means it is served from the cache.
	tick := time.Now()
	database.MetricsCache.SetLastScraped(metric, &tick)

	runScrapeDatabase(t, exporter, database)

	series := gaugeSeries(t, exporter.metricScrapeDuration)
	key := durationKey(metric.Context, metric.ID, database.Name, scrapeResultSuccess)
	if got := series[key]; got != seeded {
		t.Fatalf("expected the skipped metric to keep its previous duration %v, got %v", seeded, got)
	}
}

func TestReloadMetricsResetsMetricScrapeDuration(t *testing.T) {
	path := writeCustomMetricsFixture(t, `
[[metric]]
context = "custom_instances"
metricsdesc = { value = "Custom instances." }
request = "select 1 as value from dual"
`)
	exporter := newTestExporterWithCustomMetrics(path)
	database := &Database{Name: "db1", DatabaseLabel: "database"}
	removed := newTestDurationMetric("")
	exporter.observeMetricScrapeDuration(database, removed, scrapeResultSuccess, time.Second)

	if !exporter.reloadMetrics() {
		t.Fatal("expected metrics reload to succeed")
	}

	if series := gaugeSeries(t, exporter.metricScrapeDuration); len(series) != 0 {
		t.Fatalf("expected duration series to be cleared on reload, got %v", series)
	}
}

// The exporter emits its built-in metrics from two separate code paths, and both must include
// the per-metric scrape duration.
func TestCollectEmitsMetricScrapeDuration(t *testing.T) {
	t.Run("scheduled scrapes", func(t *testing.T) {
		exporter, database := newTestScheduledExporter(t, time.Hour)
		database.startupReady.Store(true)
		database.setUp(1)

		tick := time.Now()
		exporter.scheduledScrape(&tick)

		if !hasScheduledMetric(exporter, metricScrapeDurationName) {
			t.Fatalf("expected %s in the scheduled scrape results", metricScrapeDurationName)
		}
	})

	t.Run("on demand scrapes", func(t *testing.T) {
		metric := newTestDurationMetric("")
		exporter, database := newTestDurationExporter(t, openTestQueryDBWithRows(t, nil), metric)
		exporter.mu = &sync.Mutex{}
		exporter.totalScrapes = prometheus.NewCounter(prometheus.CounterOpts{Name: "test_scrapes_total", Help: "test"})
		exporter.duration = prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_duration_seconds", Help: "test"})
		exporter.error = prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_error", Help: "test"})
		exporter.databases = []*Database{database}

		var descs []string
		ch := make(chan prometheus.Metric)
		done := make(chan struct{})
		go func() {
			for m := range ch {
				descs = append(descs, m.Desc().String())
			}
			close(done)
		}()
		exporter.Collect(ch)
		close(ch)
		<-done

		found := false
		for _, desc := range descs {
			if strings.Contains(desc, `fqName: "`+metricScrapeDurationName+`"`) {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected %s in the on-demand collect results, got %v", metricScrapeDurationName, descs)
		}
	})
}

func newTestDurationMetric(scrapeInterval string) *Metric {
	metric := &Metric{
		Context:        "test",
		MetricsDesc:    map[string]string{"value": "Test metric."},
		MetricsType:    map[string]string{"value": "gauge"},
		Request:        "select 1 as value from dual",
		ScrapeInterval: scrapeInterval,
	}
	metric.normalizeIdentifiers()
	return metric
}

func newTestDurationExporter(t *testing.T, session *sql.DB, metric *Metric) (*Exporter, *Database) {
	t.Helper()

	metricsToScrape := map[string]*Metric{metric.ID: metric}
	database := &Database{
		Name:          "db1",
		Session:       session,
		DatabaseLabel: "database",
	}
	database.startupReady.Store(true)
	database.initCache(metricsToScrape)

	exporter := &Exporter{
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		MetricsConfiguration: &MetricsConfiguration{},
		metricsToScrape:      metricsToScrape,
		databaseDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_database_scrape_duration_seconds",
			Help:      "test",
		}, []string{"database"}),
		metricScrapeDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_metric_scrape_duration_seconds",
			Help:      "test",
		}, []string{"collector", "metric", "database", "result"}),
		scrapeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "scrape_errors_total",
			Help:      "test",
		}, []string{"collector", "database"}),
	}
	return exporter, database
}

// runScrapeDatabase performs a single database scrape and waits for every per-metric goroutine to finish.
func runScrapeDatabase(t *testing.T, exporter *Exporter, database *Database) {
	t.Helper()

	metricCh := make(chan prometheus.Metric, len(exporter.metricsToScrape)+1)
	errChan := make(chan error, len(exporter.metricsToScrape)+1)
	now := time.Now()

	// scrapeDatabase waits for its metric goroutines before returning.
	exporter.scrapeDatabase(metricCh, errChan, database, &now)
	close(metricCh)
	close(errChan)
	for range metricCh {
	}
	for range errChan {
	}
}

// gaugeSeries returns the current values of a GaugeVec, keyed by a stable rendering of its labels.
func gaugeSeries(t *testing.T, vec *prometheus.GaugeVec) map[string]float64 {
	t.Helper()

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(vec)
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	series := map[string]float64{}
	for _, family := range families {
		for _, m := range family.GetMetric() {
			pairs := make([]string, 0, len(m.GetLabel()))
			for _, label := range m.GetLabel() {
				pairs = append(pairs, label.GetName()+"="+label.GetValue())
			}
			sort.Strings(pairs)
			series[fmt.Sprintf("%s{%s}", family.GetName(), strings.Join(pairs, ","))] = m.GetGauge().GetValue()
		}
	}
	return series
}

func durationKey(collector, metricID, database, result string) string {
	pairs := []string{
		"collector=" + collector,
		"database=" + database,
		"metric=" + metricID,
		"result=" + result,
	}
	sort.Strings(pairs)
	return fmt.Sprintf("%s{%s}", metricScrapeDurationName, strings.Join(pairs, ","))
}
