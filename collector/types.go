// Copyright (c) 2025, Oracle and/or its affiliates.

package collector

import (
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
	"sync"
	"time"
)

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	*MetricsConfiguration
	mu              *sync.Mutex
	metricsToScrape map[string]*Metric
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    *prometheus.CounterVec
	scrapeResults   []prometheus.Metric
	databases       []*Database
	logger          *slog.Logger
	allConstLabels  []string
}

type Database struct {
	Name    string
	Up      float64
	Session *sql.DB
	Config  DatabaseConfig
	// MetricsCache holds computed metrics for a database, so these metrics are available on each scrape.
	// Given a metric's scrape configuration, it may not be computed on the same interval as other metrics.
	MetricsCache *MetricsCache

	Valid bool
}

type MetricsCache struct {
	// The outer map is to be initialized at startup, and when metrics are reloaded.
	// Read access is concurrent, write access is (and must) be from a single thread.
	cache map[*Metric]*MetricCacheRecord
}

// MetricCacheRecord stores metadata associated with a given Metric
// As one metric may have multiple prometheus.Metric representations,
// These are cached as a map value.
type MetricCacheRecord struct {
	// PrometheusMetrics stores cached prometheus metric values.
	// Used when custom scrape intervals are used, and the metric must be returned to the collector, but not scraped.
	PrometheusMetrics map[string]prometheus.Metric
	// LastScraped is the collector tick time when the metric was last computed.
	LastScraped *time.Time
}

type Config struct {
	ConfigFile         string
	User               string
	Password           string
	ConnectString      string
	DbRole             string
	ConfigDir          string
	ExternalAuth       bool
	MaxIdleConns       int
	MaxOpenConns       int
	PoolIncrement      int
	PoolMaxConnections int
	PoolMinConnections int
	CustomMetrics      string
	QueryTimeout       int
	DefaultMetricsFile string
	ScrapeInterval     time.Duration
	LoggingConfig      LoggingConfig
}

// Metric is an object description
type Metric struct {
	Context          string
	Labels           []string
	MetricsDesc      map[string]string
	MetricsType      map[string]string
	MetricsBuckets   map[string]map[string]string
	FieldToAppend    string
	Request          string
	IgnoreZeroResult bool
	QueryTimeout     string
	ScrapeInterval   string
	Databases        []string
}

// Metrics is a container structure for prometheus metrics
type Metrics struct {
	Metric []*Metric `yaml:"metrics"`
}

type ScrapeContext struct {
}
