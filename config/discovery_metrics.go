// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type discoveryMetrics struct {
	mu              sync.Mutex
	success         float64
	duration        float64
	lastSuccess     float64
	targets         float64
	matched         float64
	skipped         float64
	successDesc     *prometheus.Desc
	durationDesc    *prometheus.Desc
	lastSuccessDesc *prometheus.Desc
	targetsDesc     *prometheus.Desc
	matchedDesc     *prometheus.Desc
	skippedDesc     *prometheus.Desc
}

func newDiscoveryMetrics() discoveryMetrics {
	return discoveryMetrics{
		successDesc:     prometheus.NewDesc("oracledb_exporter_discovery_success", "Whether the last OCI discovery attempt succeeded.", nil, nil),
		durationDesc:    prometheus.NewDesc("oracledb_exporter_discovery_duration_seconds", "Duration of the last OCI discovery attempt.", nil, nil),
		lastSuccessDesc: prometheus.NewDesc("oracledb_exporter_discovery_last_success_timestamp_seconds", "Unix time of the last successful OCI discovery.", nil, nil),
		targetsDesc:     prometheus.NewDesc("oracledb_exporter_discovery_targets", "Accepted discovered targets after static target precedence in the last successful discovery.", nil, nil),
		matchedDesc:     prometheus.NewDesc("oracledb_exporter_discovery_matched_targets", "OCI resources matching lifecycle and required tag filters in the last successful discovery.", nil, nil),
		skippedDesc:     prometheus.NewDesc("oracledb_exporter_discovery_skipped_targets", "Matched OCI resources skipped for invalid configuration, duplicate names, or static target precedence in the last successful discovery.", nil, nil),
	}
}

func (m *discoveryMetrics) record(start time.Time, err error, accepted, matched, skipped int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.duration = time.Since(start).Seconds()
	m.success = 0
	if err == nil {
		m.success = 1
		m.lastSuccess = float64(time.Now().UnixNano()) / 1e9
		m.targets = float64(accepted)
		m.matched = float64(matched)
		m.skipped = float64(skipped)
	}
}

func (m *discoveryMetrics) Describe(out chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, out)
}

func (m *discoveryMetrics) Collect(out chan<- prometheus.Metric) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out <- prometheus.MustNewConstMetric(m.successDesc, prometheus.GaugeValue, m.success)
	out <- prometheus.MustNewConstMetric(m.durationDesc, prometheus.GaugeValue, m.duration)
	out <- prometheus.MustNewConstMetric(m.lastSuccessDesc, prometheus.GaugeValue, m.lastSuccess)
	out <- prometheus.MustNewConstMetric(m.targetsDesc, prometheus.GaugeValue, m.targets)
	out <- prometheus.MustNewConstMetric(m.matchedDesc, prometheus.GaugeValue, m.matched)
	out <- prometheus.MustNewConstMetric(m.skippedDesc, prometheus.GaugeValue, m.skipped)
}
