// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

func NewMetricsCache(metrics map[string]*Metric) *MetricsCache {
	c := map[*Metric]*MetricCacheRecord{}

	for _, metric := range metrics {
		c[metric] = &MetricCacheRecord{
			PrometheusMetrics: map[string]prometheus.Metric{},
			LastScraped:       nil,
		}
	}
	return &MetricsCache{
		cache: c,
	}
}

func (c *MetricsCache) SetLastScraped(m *Metric, tick *time.Time) {
	c.cache[m].LastScraped = tick
}

func (c *MetricsCache) GetLastScraped(m *Metric) *time.Time {
	return c.cache[m].LastScraped
}

func (c *MetricsCache) SendAll(ch chan<- prometheus.Metric, m *Metric) {
	for _, pm := range c.cache[m].PrometheusMetrics {
		ch <- pm
	}
}

func (c *MetricsCache) CacheAndSend(ch chan<- prometheus.Metric, m *Metric, metric prometheus.Metric) {
	c.cache[m].PrometheusMetrics[metric.Desc().String()] = metric
	ch <- metric
}
