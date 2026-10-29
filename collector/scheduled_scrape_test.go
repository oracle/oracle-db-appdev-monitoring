// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const scheduledTestMetricName = "oracledb_test_value"

// A scheduled scrape holds the scrape lock for as long as its slowest query runs. Collect must keep
// serving the last completed snapshot meanwhile; otherwise /metrics requests exceed the Prometheus
// scrape timeout and every series of the target shows a gap.
func TestCollectDoesNotWaitForScheduledScrapeInProgress(t *testing.T) {
	exporter, database := newTestScheduledExporter(t, time.Hour)
	gate := newQueryGate()
	t.Cleanup(gate.open)
	session := sql.OpenDB(gatedQueryConnector{gate: gate})
	t.Cleanup(func() { _ = session.Close() })
	database.Session = session
	if err := database.WarmupConnectionPool(testLogger(), time.Hour); err != nil {
		t.Fatalf("expected test database warmup to succeed, got %v", err)
	}

	gate.value.Store("1")
	exporter.doScrape(time.Now())
	previous := collectScheduledValues(exporter)
	if got, ok := previous[scheduledTestMetricName]; !ok || got != 1 {
		t.Fatalf("expected the first scheduled scrape to publish %s=1, got %v", scheduledTestMetricName, previous)
	}

	gate.value.Store("2")
	gate.hold.Store(true)
	scrapeDone := make(chan struct{})
	go func() {
		defer close(scrapeDone)
		exporter.doScrape(time.Now())
	}()
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the scheduled scrape to start its query")
	}

	collected := make(chan map[string]float64, 1)
	go func() { collected <- collectScheduledValues(exporter) }()
	select {
	case during := <-collected:
		if got := during[scheduledTestMetricName]; got != 1 {
			t.Fatalf("expected the previous snapshot (%s=1) while a scrape is in progress, got %v", scheduledTestMetricName, got)
		}
		if len(during) != len(previous) {
			t.Fatalf("expected the complete previous snapshot while a scrape is in progress, got %v, want %v", during, previous)
		}
	case <-time.After(time.Second):
		t.Fatal("Collect blocked on the scheduled scrape in progress")
	}

	gate.open()
	select {
	case <-scrapeDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the scheduled scrape to finish")
	}
	if got := collectScheduledValues(exporter)[scheduledTestMetricName]; got != 2 {
		t.Fatalf("expected the completed scrape to publish %s=2, got %v", scheduledTestMetricName, got)
	}
}

// collectScheduledValues runs Collect and returns the value of every gauge and counter, keyed by
// metric name.
func collectScheduledValues(exporter *Exporter) map[string]float64 {
	ch := make(chan prometheus.Metric)
	done := make(chan map[string]float64, 1)
	go func() {
		values := map[string]float64{}
		for metric := range ch {
			var m dto.Metric
			if err := metric.Write(&m); err != nil {
				continue
			}
			name := fqNameOf(metric.Desc())
			switch {
			case m.Gauge != nil:
				values[name] = m.GetGauge().GetValue()
			case m.Counter != nil:
				values[name] = m.GetCounter().GetValue()
			default:
				values[name] = 0
			}
		}
		done <- values
	}()
	exporter.Collect(ch)
	close(ch)
	return <-done
}

func fqNameOf(desc *prometheus.Desc) string {
	const marker = `fqName: "`
	s := desc.String()
	start := strings.Index(s, marker)
	if start < 0 {
		return s
	}
	start += len(marker)
	end := strings.IndexByte(s[start:], '"')
	if end < 0 {
		return s
	}
	return s[start : start+end]
}

// queryGate lets a test hold database queries open, simulating a slow scheduled scrape.
type queryGate struct {
	hold        atomic.Bool
	value       atomic.Value
	started     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newQueryGate() *queryGate {
	return &queryGate{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (g *queryGate) open() {
	g.releaseOnce.Do(func() { close(g.release) })
}

type gatedQueryConnector struct {
	gate *queryGate
}

func (c gatedQueryConnector) Connect(context.Context) (driver.Conn, error) {
	return gatedQueryConn{gate: c.gate}, nil
}

func (gatedQueryConnector) Driver() driver.Driver { return testQueryDriver{} }

type gatedQueryConn struct {
	testQueryConn
	gate *queryGate
}

func (c gatedQueryConn) QueryContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.gate.hold.Load() {
		select {
		case c.gate.started <- struct{}{}:
		default:
		}
		select {
		case <-c.gate.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	value, _ := c.gate.value.Load().(string)
	return &valueRows{value: value}, nil
}

// valueRows returns a single row with the given value column.
type valueRows struct {
	value string
	read  bool
}

func (r *valueRows) Columns() []string { return []string{"value"} }

func (r *valueRows) Close() error { return nil }

func (r *valueRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = r.value
	return nil
}
