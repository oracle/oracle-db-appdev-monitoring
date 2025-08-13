// Copyright (c) 2025, Oracle and/or its affiliates.

package collector

import (
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/godror/godror/dsn"
	"github.com/prometheus/client_golang/prometheus"
)

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	*MetricsConfiguration
	mu              *sync.Mutex
	metricsToScrape Metrics
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    *prometheus.CounterVec
	scrapeResults   []prometheus.Metric
	databases       []*Database
	logger          *slog.Logger
	lastScraped     map[string]*time.Time
	allConstLabels  []string
}

type Database struct {
	Name    string
	Up      float64
	Session *sql.DB
	Type    float64
	Config  DatabaseConfig
}

type Config struct {
	ConfigFile         string
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
	Metric []Metric
}

type ScrapeContext struct {
}

func (m Metric) id(dbname string) string {
	builder := strings.Builder{}
	builder.WriteString(dbname)
	builder.WriteString(m.Context)
	for _, d := range m.MetricsDesc {
		builder.WriteString(d)
	}
	return builder.String()
}
