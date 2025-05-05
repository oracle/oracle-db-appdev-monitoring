// Copyright (c) 2025, Oracle and/or its affiliates.

package collector

import (
	"database/sql"
	"github.com/godror/godror/dsn"
	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	config          *Config
	mu              *sync.Mutex
	metricsToScrape Metrics
	scrapeInterval  time.Duration
	user            string
	password        string
	connectString   string
	configDir       string
	externalAuth    bool
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    *prometheus.CounterVec
	scrapeResults   []prometheus.Metric
	up              prometheus.Gauge
	dbtype          int
	dbtypeGauge     prometheus.Gauge
	db              *sql.DB
	logger          *slog.Logger
	lastScraped     map[string]*time.Time
}

type Config struct {
	User               string
	Password           string
	ConnectString      string
	DbRole             dsn.AdminRole
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
}

// Metrics is a container structure for prometheus metrics
type Metrics struct {
	Metric []Metric
}

func (m Metric) id() string {
	builder := strings.Builder{}
	builder.WriteString(m.Context)
	for _, d := range m.MetricsDesc {
		builder.WriteString(d)
	}
	return builder.String()
}
