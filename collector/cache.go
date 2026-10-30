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
	r := c.cache[m]
	r.mu.Lock()
	defer r.mu.Unlock()
	r.LastScraped = tick
}

func (c *MetricsCache) GetLastScraped(m *config.Metric) *time.Time {
	r := c.cache[m]
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.LastScraped
}

func (c *MetricsCache) SendAll(ch chan<- prometheus.Metric, m *config.Metric) {
	r := c.cache[m]
	r.mu.RLock()
	metrics := r.PrometheusMetrics
	r.mu.RUnlock()
	// Stored values are replaced, never modified, so they can be sent without holding the lock.
	for _, pm := range metrics {
		ch <- pm
	}
}

// Store replaces the cached values of a metric with the complete result of a scrape.
func (c *MetricsCache) Store(m *config.Metric, metrics []prometheus.Metric) {
	r := c.cache[m]
	r.mu.Lock()
	defer r.mu.Unlock()
	r.PrometheusMetrics = metrics
}

// StartRefresh marks a background refresh of the metric as running. It returns false if one is
// already running.
func (c *MetricsCache) StartRefresh(m *config.Metric) bool {
	r := c.cache[m]
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.refreshing {
		return false
	}
	r.refreshing = true
	return true
}

// FinishRefresh marks the background refresh of the metric as done and records its error. The
// metric only counts as scraped at tick if the refresh succeeded, so a failed refresh is retried.
func (c *MetricsCache) FinishRefresh(m *config.Metric, tick *time.Time, err error) {
	r := c.cache[m]
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshing = false
	r.lastRefreshErr = err
	if err == nil {
		r.LastScraped = tick
	}
}

// LastRefreshError returns the error of the last completed background refresh of the metric.
func (c *MetricsCache) LastRefreshError(m *config.Metric) error {
	r := c.cache[m]
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastRefreshErr
}
