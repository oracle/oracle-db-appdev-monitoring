// Copyright (c) 2025, 2026, Oracle and/or its affiliates.

package collector

import (
	"database/sql"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/prometheus/client_golang/prometheus"
)

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	*config.MetricsConfiguration
	mu                  *sync.Mutex
	metricsToScrape     map[string]*config.Metric
	customMetricsHashes map[string][]byte
	duration, error     prometheus.Gauge
	databaseDuration    *prometheus.GaugeVec
	// metricScrapeDuration tracks how long the last scrape of each individual metric took, per database.
	// It is nil unless metrics.perMetricScrapeDuration.enabled is set; a nil vector disables the metric.
	metricScrapeDuration *prometheus.GaugeVec
	totalScrapes         prometheus.Counter
	scrapeErrors         *prometheus.CounterVec
	scrapeResults        []prometheus.Metric
	scrapeRequests       chan struct{}
	databases            []*Database
	logger               *slog.Logger
	allConstLabels       []string
}

type Database struct {
	Name       string
	Up         float64
	Session    *sql.DB
	Config     config.DatabaseConfig
	connectErr error
	// MetricsCache holds computed metrics for a database, so these metrics are available on each scrape.
	// Given a metric's scrape configuration, it may not be computed on the same interval as other metrics.
	MetricsCache *MetricsCache

	invalidUntil  *time.Time
	DatabaseLabel string
	startupReady  atomic.Bool

	reconnectMU        sync.RWMutex
	reconnectAttemptMU sync.Mutex
}

type MetricsCache struct {
	// The outer map is to be initialized at startup, and when metrics are reloaded.
	// Read access is concurrent, write access is (and must) be from a single thread.
	cache map[*config.Metric]*MetricCacheRecord
}

// MetricCacheRecord stores metadata associated with a given Metric
// As one metric may have multiple prometheus.Metric representations,
// These are cached as a map value.
type MetricCacheRecord struct {
	// PrometheusMetrics stores cached prometheus metric values.
	// Used when custom scrape intervals are used, and the metric must be returned to the collector, but not scraped.
	PrometheusMetrics []prometheus.Metric
	// LastScraped is the collector tick time when the metric was last computed.
	LastScraped *time.Time
}
