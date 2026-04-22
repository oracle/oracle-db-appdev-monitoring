// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/internal/testdb"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestCollectorUtilityHelpers(t *testing.T) {
	if got := maskDsn("user:pass@dbhost/service"); got != "***@dbhost/service" {
		t.Fatalf("expected masked dsn, got %q", got)
	}
	if got := maskDsn("dbhost/service"); got != "dbhost/service" {
		t.Fatalf("expected dsn without @ to be unchanged, got %q", got)
	}
	if got := cleanName("User Calls/Second (*)"); got != "user_callssecond_" {
		t.Fatalf("expected cleaned metric name, got %q", got)
	}
	if got := metricNameSuffix(map[string]string{"name": "User Calls/Second (*)"}, "value", "name"); got != "user_callssecond_" {
		t.Fatalf("expected cleaned metric suffix, got %q", got)
	}
	if got := metricNameSuffix(map[string]string{"name": "ignored"}, "value", ""); got != "value" {
		t.Fatalf("expected metric name when no fieldToAppend, got %q", got)
	}
	if got := getMetricType("value", map[string]string{"value": "counter"}); got != prometheus.CounterValue {
		t.Fatalf("expected counter value type, got %v", got)
	}
	if got := getMetricType("missing", map[string]string{}); got != prometheus.GaugeValue {
		t.Fatalf("expected default gauge type, got %v", got)
	}
}

func TestIsScrapeMetricHonorsScheduleAndDatabaseEnablement(t *testing.T) {
	metric := &Metric{
		ID:             "sessions_value",
		Context:        "sessions",
		ScrapeInterval: "1m",
		Databases:      []string{"db1"},
	}
	cache := NewMetricsCache(map[string]*Metric{metric.ID: metric})
	last := time.Now().Add(-2 * time.Minute)
	cache.SetLastScraped(metric, &last)
	db := &Database{
		Name:         "db1",
		MetricsCache: cache,
	}
	now := time.Now()

	if !isScrapeMetric(testLogger(), &now, metric, db) {
		t.Fatal("expected metric to be scraped when interval has elapsed")
	}

	recent := time.Now().Add(-30 * time.Second)
	cache.SetLastScraped(metric, &recent)
	if isScrapeMetric(testLogger(), &now, metric, db) {
		t.Fatal("expected metric to be skipped when cached value is fresh")
	}

	db.Name = "db2"
	if isScrapeMetric(testLogger(), &now, metric, db) {
		t.Fatal("expected metric disabled for other database")
	}

	if !isScrapeMetric(testLogger(), nil, &Metric{Databases: nil}, db) {
		t.Fatal("expected nil tick to force scrape")
	}
}

func TestGetScrapeIntervalAndQueryTimeout(t *testing.T) {
	logger := testLogger()
	if got, ok := getScrapeInterval(logger, "sessions", "30s"); !ok || got != 30*time.Second {
		t.Fatalf("expected 30s scrape interval, got (%v, %v)", got, ok)
	}
	if _, ok := getScrapeInterval(logger, "sessions", "bad"); ok {
		t.Fatal("expected invalid scrape interval to return ok=false")
	}

	db := &Database{Config: DatabaseConfig{ConnectConfig: ConnectConfig{QueryTimeout: ptr(12)}}}
	if got := getQueryTimeout(logger, &Metric{Context: "sessions", QueryTimeout: "3s"}, db); got != 3*time.Second {
		t.Fatalf("expected metric-specific query timeout, got %v", got)
	}
	if got := getQueryTimeout(logger, &Metric{Context: "sessions", QueryTimeout: "bad"}, db); got != 12*time.Second {
		t.Fatalf("expected fallback query timeout, got %v", got)
	}
}

func TestShouldLogScrapeError(t *testing.T) {
	if !shouldLogScrapeError(errors.New("boom"), true) {
		t.Fatal("expected generic errors to be logged")
	}
	if shouldLogScrapeError(newZeroResultError(), true) {
		t.Fatal("expected ignored zero-result errors not to be logged")
	}
	if !shouldLogScrapeError(newZeroResultError(), false) {
		t.Fatal("expected zero-result errors to be logged when not ignored")
	}
}

func TestHashFileAndCheckIfMetricsChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.toml")
	if err := os.WriteFile(path, []byte("[[metric]]\ncontext=\"sessions\"\nmetricsdesc={ value=\"help\" }\nrequest=\"select 1 as value from dual\"\n"), 0o600); err != nil {
		t.Fatalf("write metrics file: %v", err)
	}

	exporter := &Exporter{
		logger:               testLogger(),
		MetricsConfiguration: &MetricsConfiguration{Metrics: MetricsFilesConfig{Custom: []string{path}}},
		customMetricsHashes:  map[string][]byte{},
	}

	if !exporter.checkIfMetricsChanged() {
		t.Fatal("expected first hash check to detect file")
	}
	if exporter.checkIfMetricsChanged() {
		t.Fatal("expected unchanged file not to trigger reload")
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(mustReadFile(t, path)), "select 1", "select 2")), 0o600); err != nil {
		t.Fatalf("update metrics file: %v", err)
	}
	if !exporter.checkIfMetricsChanged() {
		t.Fatal("expected modified file to trigger reload")
	}
}

func TestGeneratePrometheusMetricsLowersColumnNames(t *testing.T) {
	dbh, _ := testdb.New(testdb.Scenario{
		QueryFunc: func(ctx context.Context, query string, args []driver.NamedValue) testdb.QueryResult {
			return testdb.QueryResult{
				Columns: []string{"VALUE", "INST_ID"},
				Rows:    [][]driver.Value{{"7", "1"}},
			}
		},
	})
	defer dbh.Close()

	db := &Database{Name: "db1", Session: dbh}
	exporter := &Exporter{logger: testLogger()}
	var rows []map[string]string

	err := exporter.generatePrometheusMetrics(db, func(row map[string]string) error {
		rows = append(rows, row)
		return nil
	}, "select 1", time.Second)
	if err != nil {
		t.Fatalf("generate metrics: %v", err)
	}
	if len(rows) != 1 || rows[0]["value"] != "7" || rows[0]["inst_id"] != "1" {
		t.Fatalf("expected lowercase row keys, got %#v", rows)
	}
}

func TestGeneratePrometheusMetricsHandlesTimeout(t *testing.T) {
	dbh, _ := testdb.New(testdb.Scenario{
		QueryFunc: func(ctx context.Context, query string, args []driver.NamedValue) testdb.QueryResult {
			<-ctx.Done()
			return testdb.QueryResult{Err: ctx.Err()}
		},
	})
	defer dbh.Close()

	db := &Database{Name: "db1", Session: dbh}
	exporter := &Exporter{logger: testLogger()}

	err := exporter.generatePrometheusMetrics(db, func(map[string]string) error { return nil }, "select 1", 10*time.Millisecond)
	if err == nil || err.Error() != "Oracle query timed out" {
		t.Fatalf("expected Oracle query timed out, got %v", err)
	}
}

func TestScrapeGenericValuesEmitsGaugeMetricAndCachesIt(t *testing.T) {
	metric := &Metric{
		ID:          "sessions_value",
		Context:     "sessions",
		Labels:      []string{"instance"},
		MetricsDesc: map[string]string{"value": "Active sessions"},
		Request:     "select 1",
	}
	dbh, _ := testdb.New(testdb.Scenario{
		QueryFunc: func(ctx context.Context, query string, args []driver.NamedValue) testdb.QueryResult {
			return testdb.QueryResult{
				Columns: []string{"INSTANCE", "VALUE"},
				Rows:    [][]driver.Value{{"prod-1", "42.5"}},
			}
		},
	})
	defer dbh.Close()

	db := &Database{
		Name:          "db1",
		DatabaseLabel: "database",
		Session:       dbh,
		MetricsCache:  NewMetricsCache(map[string]*Metric{metric.ID: metric}),
	}
	exporter := &Exporter{
		logger:               testLogger(),
		MetricsConfiguration: &MetricsConfiguration{},
	}
	ch := make(chan prometheus.Metric, 2)

	if err := exporter.scrapeGenericValues(db, ch, metric); err != nil {
		t.Fatalf("scrapeGenericValues: %v", err)
	}
	if got := len(ch); got != 1 {
		t.Fatalf("expected 1 metric, got %d", got)
	}

	pm := <-ch
	var written dto.Metric
	if err := pm.Write(&written); err != nil {
		t.Fatalf("write prometheus metric: %v", err)
	}
	if written.Gauge == nil || written.Gauge.GetValue() != 42.5 {
		t.Fatalf("expected gauge value 42.5, got %#v", written.Gauge)
	}
	labelValues := map[string]string{}
	for _, label := range written.Label {
		labelValues[label.GetName()] = label.GetValue()
	}
	if labelValues["instance"] != "prod-1" || labelValues["database"] != "db1" {
		t.Fatalf("expected instance/database labels, got %#v", written.Label)
	}
	if len(db.MetricsCache.cache[metric].PrometheusMetrics) != 1 {
		t.Fatal("expected metric to be cached")
	}
}

func TestScrapeGenericValuesEmitsHistogramMetric(t *testing.T) {
	metric := &Metric{
		ID:          "latency_value",
		Context:     "latency",
		MetricsDesc: map[string]string{"value": "Latency"},
		MetricsType: map[string]string{"value": "histogram"},
		MetricsBuckets: map[string]map[string]string{
			"value": {"bucket_1": "1", "bucket_5": "5"},
		},
		Request: "select histogram",
	}
	dbh, _ := testdb.New(testdb.Scenario{
		QueryFunc: func(ctx context.Context, query string, args []driver.NamedValue) testdb.QueryResult {
			return testdb.QueryResult{
				Columns: []string{"VALUE", "COUNT", "BUCKET_1", "BUCKET_5"},
				Rows:    [][]driver.Value{{"6.5", "7", "3", "7"}},
			}
		},
	})
	defer dbh.Close()

	db := &Database{
		Name:          "db1",
		DatabaseLabel: "database",
		Session:       dbh,
		MetricsCache:  NewMetricsCache(map[string]*Metric{metric.ID: metric}),
	}
	exporter := &Exporter{logger: testLogger(), MetricsConfiguration: &MetricsConfiguration{}}
	ch := make(chan prometheus.Metric, 1)

	if err := exporter.scrapeGenericValues(db, ch, metric); err != nil {
		t.Fatalf("scrapeGenericValues: %v", err)
	}

	pm := <-ch
	var written dto.Metric
	if err := pm.Write(&written); err != nil {
		t.Fatalf("write prometheus metric: %v", err)
	}
	if written.Histogram == nil {
		t.Fatal("expected histogram metric")
	}
	if written.Histogram.GetSampleCount() != 7 || written.Histogram.GetSampleSum() != 6.5 {
		t.Fatalf("unexpected histogram values: %#v", written.Histogram)
	}
}

func TestScrapeGenericValuesReturnsZeroResultErrorWhenConfigured(t *testing.T) {
	metric := &Metric{
		ID:          "sessions_value",
		Context:     "sessions",
		MetricsDesc: map[string]string{"value": "Active sessions"},
		Request:     "select 1",
	}
	dbh, _ := testdb.New(testdb.Scenario{})
	defer dbh.Close()

	db := &Database{Name: "db1", Session: dbh, MetricsCache: NewMetricsCache(map[string]*Metric{metric.ID: metric})}
	exporter := &Exporter{logger: testLogger(), MetricsConfiguration: &MetricsConfiguration{}}

	err := exporter.scrapeGenericValues(db, make(chan prometheus.Metric, 1), metric)
	if err == nil || !errors.Is(err, newZeroResultError()) {
		t.Fatalf("expected zero result error, got %v", err)
	}

	metric.IgnoreZeroResult = true
	if err := exporter.scrapeGenericValues(db, make(chan prometheus.Metric, 1), metric); err != nil {
		t.Fatalf("expected ignorezeroresult to suppress error, got %v", err)
	}
}

func TestScrapeGenericValuesSkipsDuplicatedLabels(t *testing.T) {
	metric := &Metric{
		ID:          "sessions_value",
		Context:     "sessions",
		Labels:      []string{"database"},
		MetricsDesc: map[string]string{"value": "Active sessions"},
		Request:     "select 1",
	}
	dbh, _ := testdb.New(testdb.Scenario{})
	defer dbh.Close()

	db := &Database{
		Name:          "db1",
		DatabaseLabel: "database",
		Session:       dbh,
		MetricsCache:  NewMetricsCache(map[string]*Metric{metric.ID: metric}),
	}
	exporter := &Exporter{logger: testLogger(), MetricsConfiguration: &MetricsConfiguration{}}
	ch := make(chan prometheus.Metric, 1)

	if err := exporter.scrapeGenericValues(db, ch, metric); err != nil {
		t.Fatalf("expected duplicate labels to skip metric without error, got %v", err)
	}
	if len(ch) != 0 {
		t.Fatalf("expected no emitted metrics, got %d", len(ch))
	}
}

func TestCollectUsesScheduledScrapeResults(t *testing.T) {
	interval := time.Second
	exporter := &Exporter{
		mu: &sync.Mutex{},
		MetricsConfiguration: &MetricsConfiguration{
			Metrics: MetricsFilesConfig{ScrapeInterval: &interval},
		},
		scrapeResults: []prometheus.Metric{
			prometheus.MustNewConstMetric(
				prometheus.NewDesc("oracledb_test", "test", nil, nil),
				prometheus.GaugeValue,
				1,
			),
		},
	}
	ch := make(chan prometheus.Metric, 2)

	exporter.Collect(ch)

	if got := len(ch); got != 1 {
		t.Fatalf("expected 1 cached scrape result, got %d", got)
	}
}

func TestScheduledScrapeAndDoScrapeStoreResults(t *testing.T) {
	interval := time.Second
	exporter := &Exporter{
		mu: &sync.Mutex{},
		MetricsConfiguration: &MetricsConfiguration{
			Metrics: MetricsFilesConfig{ScrapeInterval: &interval},
		},
		duration:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_duration_scheduled", Help: "help"}),
		databaseDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_db_duration_scheduled", Help: "help"}, []string{"database"}),
		totalScrapes:     prometheus.NewCounter(prometheus.CounterOpts{Name: "test_scrapes_total_scheduled", Help: "help"}),
		scrapeErrors:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_scrape_errors_total_scheduled", Help: "help"}, []string{"collector", "database"}),
		error:            prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_error_scheduled", Help: "help"}),
		logger:           testLogger(),
		metricsToScrape:  map[string]*Metric{},
	}

	exporter.doScrape(time.Now())

	if len(exporter.scrapeResults) != 3 {
		t.Fatalf("expected scheduled scrape to store 3 metadata metrics without database samples, got %d", len(exporter.scrapeResults))
	}
}

func TestRunScheduledScrapesStopsOnContextCancel(t *testing.T) {
	interval := 5 * time.Millisecond
	exporter := &Exporter{
		mu: &sync.Mutex{},
		MetricsConfiguration: &MetricsConfiguration{
			Metrics: MetricsFilesConfig{ScrapeInterval: &interval},
		},
		duration:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_duration_runner", Help: "help"}),
		databaseDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_db_duration_runner", Help: "help"}, []string{"database"}),
		totalScrapes:     prometheus.NewCounter(prometheus.CounterOpts{Name: "test_scrapes_total_runner", Help: "help"}),
		scrapeErrors:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_scrape_errors_total_runner", Help: "help"}, []string{"collector", "database"}),
		error:            prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_error_runner", Help: "help"}),
		logger:           testLogger(),
		metricsToScrape:  map[string]*Metric{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		exporter.RunScheduledScrapes(ctx)
	}()
	time.Sleep(15 * time.Millisecond)
	cancel()
	<-done

	if len(exporter.scrapeResults) == 0 {
		t.Fatal("expected scheduled runner to perform at least one scrape")
	}
}

func TestCollectOnDemandIncludesMetadataMetrics(t *testing.T) {
	zero := time.Duration(0)
	dbh, _ := testdb.New(testdb.Scenario{})
	defer dbh.Close()

	db := &Database{
		Name:          "db1",
		DatabaseLabel: "database",
		Session:       dbh,
		Config:        DatabaseConfig{ConnectConfig: ConnectConfig{QueryTimeout: ptr(5)}},
	}
	db.startupReady.Store(true)

	exporter := &Exporter{
		mu: &sync.Mutex{},
		MetricsConfiguration: &MetricsConfiguration{
			Metrics: MetricsFilesConfig{ScrapeInterval: &zero},
		},
		duration:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_duration", Help: "help"}),
		databaseDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_db_duration", Help: "help"}, []string{"database"}),
		totalScrapes:     prometheus.NewCounter(prometheus.CounterOpts{Name: "test_scrapes_total", Help: "help"}),
		scrapeErrors:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_scrape_errors_total", Help: "help"}, []string{"collector", "database"}),
		error:            prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_error", Help: "help"}),
		databases:        []*Database{db},
		logger:           testLogger(),
		metricsToScrape:  map[string]*Metric{},
	}
	ch := make(chan prometheus.Metric, 10)

	exporter.Collect(ch)

	if got := len(ch); got != 5 {
		t.Fatalf("expected 5 metadata metrics, got %d", got)
	}
}

func TestDescribeForwardsDescriptorsFromCollect(t *testing.T) {
	interval := time.Second
	exporter := &Exporter{
		mu: &sync.Mutex{},
		MetricsConfiguration: &MetricsConfiguration{
			Metrics: MetricsFilesConfig{ScrapeInterval: &interval},
		},
		scrapeResults: []prometheus.Metric{
			prometheus.MustNewConstMetric(
				prometheus.NewDesc("oracledb_test_metric", "test metric", nil, nil),
				prometheus.GaugeValue,
				1,
			),
		},
	}
	ch := make(chan *prometheus.Desc, 1)

	exporter.Describe(ch)

	select {
	case desc := <-ch:
		if desc == nil || !strings.Contains(desc.String(), "oracledb_test_metric") {
			t.Fatalf("unexpected descriptor %v", desc)
		}
	default:
		t.Fatal("expected descriptor from Describe")
	}
}

func TestAfterScrapeSetsDurationAndErrorGauge(t *testing.T) {
	exporter := &Exporter{
		duration: prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_duration_after", Help: "help"}),
		error:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_error_after", Help: "help"}),
	}

	exporter.afterScrape(time.Now().Add(-2*time.Second), 3)

	var m dto.Metric
	if err := exporter.error.Write(&m); err != nil {
		t.Fatalf("write error metric: %v", err)
	}
	if m.Gauge.GetValue() != 3 {
		t.Fatalf("expected error gauge 3, got %v", m.Gauge.GetValue())
	}
}

func TestGetDBsAndConstLabels(t *testing.T) {
	exporter := &Exporter{
		allConstLabels: []string{"env", "cluster"},
		databases: []*Database{
			{Name: "db1"},
		},
	}

	labels := exporter.constLabels()
	if labels["env"] != "" || labels["cluster"] != "" {
		t.Fatalf("expected empty const label values, got %v", labels)
	}
	if got := exporter.GetDBs(); len(got) != 1 || got[0].Name != "db1" {
		t.Fatalf("unexpected databases slice %#v", got)
	}
}

func TestInitializeDatabasesWarmsEachDatabase(t *testing.T) {
	exporter := &Exporter{
		logger:               testLogger(),
		MetricsConfiguration: &MetricsConfiguration{},
		databases: []*Database{
			{Name: "db1"},
		},
	}

	exporter.InitializeDatabases()

	if !exporter.databases[0].StartupReady() {
		t.Fatal("expected initialize databases to mark startup ready")
	}
}

func TestScrapeMetricDelegatesToGenericScrape(t *testing.T) {
	metric := &Metric{
		ID:          "sessions_value",
		Context:     "sessions",
		Labels:      []string{"instance"},
		MetricsDesc: map[string]string{"value": "Active sessions"},
		Request:     "select 1",
	}
	dbh, _ := testdb.New(testdb.Scenario{
		QueryFunc: func(ctx context.Context, query string, args []driver.NamedValue) testdb.QueryResult {
			return testdb.QueryResult{
				Columns: []string{"INSTANCE", "VALUE"},
				Rows:    [][]driver.Value{{"prod-1", "1"}},
			}
		},
	})
	defer dbh.Close()

	db := &Database{
		Name:          "db1",
		DatabaseLabel: "database",
		Session:       dbh,
		MetricsCache:  NewMetricsCache(map[string]*Metric{metric.ID: metric}),
	}
	exporter := &Exporter{logger: testLogger(), MetricsConfiguration: &MetricsConfiguration{}}

	if err := exporter.ScrapeMetric(db, make(chan prometheus.Metric, 1), metric); err != nil {
		t.Fatalf("expected scrape metric delegation to succeed, got %v", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
