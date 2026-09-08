// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/prometheus/client_golang/prometheus"
)

func NewMetricsCache(metrics map[string]*config.Metric) *MetricsCache {
	c := map[*config.Metric]*MetricCacheRecord{}

	for _, metric := range metrics {
		c[metric] = &MetricCacheRecord{
			PrometheusMetrics: nil,
			LastScraped:       nil,
		}
	}
	return &MetricsCache{
		cache: c,
	}
}

func (c *MetricsCache) SetLastScraped(m *config.Metric, tick *time.Time) {
	c.cache[m].LastScraped = tick
}

func (c *MetricsCache) GetLastScraped(m *config.Metric) *time.Time {
	return c.cache[m].LastScraped
}

func (c *MetricsCache) SendAll(ch chan<- prometheus.Metric, m *config.Metric) {
	for _, pm := range c.cache[m].PrometheusMetrics {
		ch <- pm
	}
}

func (c *MetricsCache) Reset(m *config.Metric) {
	c.cache[m].PrometheusMetrics = nil
}

func (c *MetricsCache) CacheAndSend(ch chan<- prometheus.Metric, m *config.Metric, metric prometheus.Metric) {
	c.cache[m].PrometheusMetrics = append(c.cache[m].PrometheusMetrics, metric)
	ch <- metric
}
