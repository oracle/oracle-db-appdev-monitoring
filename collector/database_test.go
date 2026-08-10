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
var errWarmupConnectionFailed = errors.New("warmup connection failed")

func (testQueryDriver) Open(name string) (driver.Conn, error) {
	return testQueryConn{rows: &testQueryRows{}}, nil
}

func (c testQueryConnector) Connect(context.Context) (driver.Conn, error) {
	rows := c.rows
	if rows == nil {
		rows = &testQueryRows{}
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

func (c testQueryConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return c.rows, nil
}

func (r *testQueryRows) Columns() []string {
	return []string{"value"}
}

func (r *testQueryRows) Close() error {
	return nil
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

type partialWarmupFailureConnector struct {
	successfulConnects int64
	connectAttempts    atomic.Int64
}

type partialWarmupFailureConn struct{}

func (c *partialWarmupFailureConnector) Connect(context.Context) (driver.Conn, error) {
	if c.connectAttempts.Add(1) > c.successfulConnects {
		return nil, errWarmupConnectionFailed
	}
	return partialWarmupFailureConn{}, nil
}

func (c *partialWarmupFailureConnector) Driver() driver.Driver {
	return testQueryDriver{}
}

func (partialWarmupFailureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (partialWarmupFailureConn) Close() error {
	return nil
}

func (partialWarmupFailureConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (partialWarmupFailureConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func (partialWarmupFailureConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &testQueryRows{}, nil
}

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
	if db.IsValid() == nil {
		t.Fatal("expected invalidUntil to be set after warmup failure")
	}
	if got := db.getUp(); got != 0 {
		t.Fatalf("expected database up metric to remain 0, got %v", got)
	}
}

func TestWarmupSessionClosesAcquiredConnectionsAfterPartialFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	connector := &partialWarmupFailureConnector{successfulConnects: 2}
	session := sql.OpenDB(connector)
	t.Cleanup(func() {
		_ = session.Close()
	})
	maxOpenConns := 3
	db := &Database{
		Name:          "db1",
		Config:        DatabaseConfig{ConnectConfig: ConnectConfig{MaxOpenConns: &maxOpenConns}},
		DatabaseLabel: "database",
	}

	err := db.warmupSession(logger, session)

	if !errors.Is(err, errWarmupConnectionFailed) {
		t.Fatalf("expected warmup connection failure, got %v", err)
	}
	if got := connector.connectAttempts.Load(); got != 3 {
		t.Fatalf("expected initdb plus partial warmup to make 3 connection attempts, got %d", got)
	}
	if got := session.Stats().InUse; got != 0 {
		t.Fatalf("expected acquired warmup connections to be returned to the pool, got %d in use", got)
	}
}

func TestDatabaseStateAccessIsRaceSafe(t *testing.T) {
	db := &Database{
		DatabaseLabel: "database",
	}
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if (i+j)%2 == 0 {
					db.invalidate(time.Millisecond)
				} else {
					db.clearInvalid()
				}
				db.setUp(float64((i + j) % 2))
				_ = db.IsValid()
				_, _ = db.UpMetric(map[string]string{}).Desc(), db.getUp()
			}
		}(i)
	}

	wg.Wait()
}

func TestQueryContextHoldsReadLockUntilUnlock(t *testing.T) {
	db := &Database{
		Session: openTestQueryDB(t),
	}

	rows, unlock, err := db.QueryContext(context.Background(), "select 1 from dual")
	if err != nil {
		t.Fatalf("expected query to succeed, got %v", err)
	}

	locked := make(chan struct{})
	go func() {
		db.reconnectMU.Lock()
		close(locked)
		db.reconnectMU.Unlock()
	}()

	select {
	case <-locked:
		t.Fatal("expected reconnect write lock to wait for active query reader")
	case <-time.After(100 * time.Millisecond):
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("expected rows close to succeed, got %v", err)
	}
	unlock()

	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("expected reconnect write lock to proceed after query reader released lock")
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

func TestRunScheduledScrapesRunsWhenDatabaseBecomesReady(t *testing.T) {
	exporter, database := newTestScheduledExporter(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go exporter.RunScheduledScrapes(ctx)
	waitForScheduledScrape(t, exporter)

	if hasScheduledMetric(exporter, "oracledb_test_value") {
		t.Fatal("did not expect test metric before database startup is ready")
	}

	database.startupReady.Store(true)
	database.setUp(1)
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

func newTestScheduledExporter(t *testing.T, scrapeInterval time.Duration) (*Exporter, *Database) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metric := &Metric{
		ID:          "test_value",
		Context:     "test",
		MetricsDesc: map[string]string{"value": "Test metric."},
		MetricsType: map[string]string{"value": "gauge"},
		Request:     "select 1 as value from dual",
	}
	metricsToScrape := map[string]*Metric{
		metric.ID: metric,
	}
	maxOpenConns := 1
	database := &Database{
		Name:          "db1",
		Session:       openTestQueryDB(t),
		Config:        DatabaseConfig{ConnectConfig: ConnectConfig{MaxOpenConns: &maxOpenConns}},
		DatabaseLabel: "database",
	}
	database.initCache(metricsToScrape)

	return &Exporter{
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
		databases:      []*Database{database},
		logger:         logger,
		MetricsConfiguration: &MetricsConfiguration{
			Metrics: MetricsFilesConfig{
				DatabaseLabel:  "database",
				ScrapeInterval: &scrapeInterval,
			},
		},
	}, database
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
