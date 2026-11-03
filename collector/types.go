// Copyright (c) 2025, 2026, Oracle and/or its affiliates.

package collector

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
	"github.com/prometheus/client_golang/prometheus"
)

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	*config.MetricsConfiguration
	// mu serializes scrapes. Collect must not take it in scheduled mode, as a scrape may hold it
	// for as long as its slowest query.
	mu *sync.Mutex
	// resultsMu guards scrapeResults, the last completed scheduled scrape snapshot.
	resultsMu           sync.RWMutex
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
	databases            []*db.Database
	metricsCaches        map[*db.Database]*MetricsCache
	logger               *slog.Logger
	allConstLabels       []string

	// Background refreshes of metrics with a custom scrape interval. refreshMu guards
	// refreshStopped and the start of new refreshes, so StopRefreshes can wait for all of them.
	refreshMu       sync.Mutex
	refreshCtx      context.Context
	cancelRefreshes context.CancelFunc
	refreshStopped  bool
	refreshes       sync.WaitGroup
	// refreshSlots limits the concurrent background refreshes per database. A nil channel means no limit.
	refreshSlots map[*db.Database]chan struct{}
	// generationMu guards cacheGeneration, which changes whenever the metrics caches are rebuilt.
	// A background refresh started before a reload must not report metrics that no longer exist.
	generationMu    sync.Mutex
	cacheGeneration uint64
}

type MetricsCache struct {
	// The outer map is to be initialized at startup, and when metrics are reloaded, and is
	// read-only afterwards. Records are safe for concurrent use.
	cache map[*config.Metric]*MetricCacheRecord
}

// MetricCacheRecord stores metadata associated with a given Metric
// As one metric may have multiple prometheus.Metric representations,
// These are cached as a map value.
type MetricCacheRecord struct {
	mu sync.RWMutex
	// PrometheusMetrics stores cached prometheus metric values.
	// Used when custom scrape intervals are used, and the metric must be returned to the collector, but not scraped.
	// The slice is replaced as a whole by each scrape and never modified in place.
	PrometheusMetrics []prometheus.Metric
	// LastScraped is the collector tick time when the metric was last computed.
	LastScraped *time.Time
	// refreshing is set while a background refresh of the metric runs.
	refreshing bool
	// lastRefreshErr is the error of the last completed background refresh of the metric.
	lastRefreshErr error
}
