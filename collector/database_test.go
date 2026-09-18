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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
	"github.com/prometheus/client_golang/prometheus"
)

type testQueryDriver struct{}

type testQueryConn struct {
	rows driver.Rows
}

type testQueryRows struct {
	read bool
}

type testQueryConnector struct {
	rows driver.Rows
}

var testQueryDriverID atomic.Uint64

func (testQueryDriver) Open(string) (driver.Conn, error) {
	return testQueryConn{rows: &testQueryRows{}}, nil
}

func (c testQueryConnector) Connect(context.Context) (driver.Conn, error) {
	rows := c.rows
	if rows == nil {
		rows = &testQueryRows{}
	} else if cloneable, ok := rows.(interface{ CloneTestRows() driver.Rows }); ok {
		rows = cloneable.CloneTestRows()
	}
	return testQueryConn{rows: rows}, nil
}

func (testQueryConnector) Driver() driver.Driver {
	return testQueryDriver{}
}

func (testQueryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (testQueryConn) Close() error {
	return nil
}

func (testQueryConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (testQueryConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func (c testQueryConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	if cloneable, ok := c.rows.(interface{ CloneTestRows() driver.Rows }); ok {
		return cloneable.CloneTestRows(), nil
	}
	return c.rows, nil
}

func (r *testQueryRows) Columns() []string {
	return []string{"value"}
}

func (r *testQueryRows) Close() error {
	return nil
}

func (r *testQueryRows) CloneTestRows() driver.Rows {
	return &testQueryRows{}
}

func (r *testQueryRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = "1"
	return nil
}

func openTestQueryDB(t *testing.T) *sql.DB {
	return openTestQueryDBWithRows(t, nil)
}

func openTestQueryDBWithRows(t *testing.T, rows driver.Rows) *sql.DB {
	t.Helper()

	name := "collector-test-query-" + strconv.FormatUint(testQueryDriverID.Add(1), 10)
	sql.Register(name, testQueryDriver{})

	db := sql.OpenDB(testQueryConnector{rows: rows})
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestScrapeDatabaseSkipsWhileStartupInProgress(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exporter := &Exporter{
		logger:               logger,
		MetricsConfiguration: &config.MetricsConfiguration{},
		databaseDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "database_duration_seconds",
				Help:      "test",
			},
			[]string{"database"},
		),
	}
	database := &db.Database{
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

func TestRunScheduledScrapesRunsWhenDatabaseBecomesReady(t *testing.T) {
	exporter, database := newTestScheduledExporter(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go exporter.RunScheduledScrapes(ctx)
	waitForScheduledScrape(t, exporter)

	if hasScheduledMetric(exporter, "oracledb_test_value") {
		t.Fatal("did not expect test metric before database startup is ready")
	}

	if err := database.WarmupConnectionPool(testLogger(), time.Hour); err != nil {
		t.Fatalf("expected test database warmup to succeed, got %v", err)
	}
	exporter.requestScheduledScrape()

	waitForScheduledMetric(t, exporter, "oracledb_test_value")
}

func TestInitializeDatabasesRequestsScheduledScrapeAfterWarmup(t *testing.T) {
	exporter, database := newTestScheduledExporter(t, time.Hour)

	exporter.InitializeDatabases()

	if !database.StartupReady() {
		t.Fatal("expected database startup to be marked ready after warmup")
	}
	if got := len(exporter.scrapeRequests); got != 1 {
		t.Fatalf("expected one scheduled scrape request after warmup, got %d", got)
	}
}

func newTestScheduledExporter(t *testing.T, scrapeInterval time.Duration) (*Exporter, *db.Database) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metric := &config.Metric{
		ID:          "test_value",
		Context:     "test",
		MetricsDesc: map[string]string{"value": "Test metric."},
		MetricsType: map[string]string{"value": "gauge"},
		Request:     "select 1 as value from dual",
	}
	metricsToScrape := map[string]*config.Metric{
		metric.ID: metric,
	}
	maxOpenConns := 1
	database := &db.Database{
		Name:          "db1",
		Session:       openTestQueryDB(t),
		Config:        config.DatabaseConfig{ConnectConfig: config.ConnectConfig{MaxOpenConns: &maxOpenConns}},
		DatabaseLabel: "database",
	}

	exporter := &Exporter{
		mu:              &sync.Mutex{},
		metricsToScrape: metricsToScrape,
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_scrape_duration_seconds",
			Help:      "test",
		}),
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
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "scrapes_total",
			Help:      "test",
		}),
		error: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_scrape_error",
			Help:      "test",
		}),
		scrapeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "scrape_errors_total",
			Help:      "test",
		}, []string{"collector", "database"}),
		scrapeRequests: make(chan struct{}, 1),
		databases:      []*db.Database{database},
		logger:         logger,
		MetricsConfiguration: &config.MetricsConfiguration{
			Metrics: config.MetricsFilesConfig{
				DatabaseLabel:  "database",
				ScrapeInterval: &scrapeInterval,
			},
		},
	}
	exporter.initCache()
	return exporter, database
}

func waitForScheduledScrape(t *testing.T, exporter *Exporter) {
	t.Helper()

	deadline := time.After(time.Second)
	for {
		if len(collectScheduledMetrics(exporter)) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for scheduled scrape")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForScheduledMetric(t *testing.T, exporter *Exporter, fqName string) {
	t.Helper()

	deadline := time.After(time.Second)
	for {
		if hasScheduledMetric(exporter, fqName) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for scheduled metric %q", fqName)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func hasScheduledMetric(exporter *Exporter, fqName string) bool {
	for _, desc := range collectScheduledMetrics(exporter) {
		if strings.Contains(desc, `fqName: "`+fqName+`"`) {
			return true
		}
	}
	return false
}

func collectScheduledMetrics(exporter *Exporter) []string {
	ch := make(chan prometheus.Metric)
	done := make(chan []string, 1)
	go func() {
		var descs []string
		for metric := range ch {
			descs = append(descs, metric.Desc().String())
		}
		done <- descs
	}()
	exporter.Collect(ch)
	close(ch)
	return <-done
}
