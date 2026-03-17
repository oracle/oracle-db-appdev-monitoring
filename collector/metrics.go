// Copyright (c) 2024, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl

package collector

import (
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

// isScrapeMetric returns true if a metric should be scraped. Metrics may not be scraped if they have a custom scrape interval,
// and the time since the last scrape is less than the custom scrape interval.
// If there is no tick time or last known tick, the metric is always scraped.
func isScrapeMetric(logger *slog.Logger, tick *time.Time, metric *Metric, d *Database) bool {
	// If the metric isn't enabled for the database, don't scrape it.
	if !metric.IsEnabledForDatabase(d) {
		return false
	}

	// Always scrape the metric if we don't have a current tick.
	if tick == nil {
		return true
	}
	// If the metric doesn't have a custom scrape interval, scrape it.
	interval, ok := getScrapeInterval(logger, metric.Context, metric.ScrapeInterval)
	if !ok {
		return true
	}
	lastScraped := d.MetricsCache.GetLastScraped(metric)
	shouldScrape := lastScraped == nil ||
		// If the metric's scrape interval is less than the time elapsed since the last scrape,
		// we should scrape the metric.
		interval < tick.Sub(*lastScraped)
	return shouldScrape
}

func getScrapeInterval(logger *slog.Logger, context, scrapeInterval string) (time.Duration, bool) {
	if len(scrapeInterval) > 0 {
		si, err := time.ParseDuration(scrapeInterval)
		if err != nil {
			logger.Error("Unable to convert scrapeinterval to duration (metric=" + context + ")")
			return 0, false
		}
		return si, true
	}
	return 0, false
}

func getQueryTimeout(logger *slog.Logger, metric *Metric, d *Database) time.Duration {
	if len(metric.QueryTimeout) > 0 {
		qt, err := time.ParseDuration(metric.QueryTimeout)
		if err != nil {
			logger.Error("Unable to convert querytimeout to duration (metric=" + metric.Context + ")")
			return time.Duration(d.Config.GetQueryTimeout()) * time.Second
		}
		return qt
	}
	return time.Duration(d.Config.GetQueryTimeout()) * time.Second
}

func parseFloat(logger *slog.Logger, metric, metricHelp string, row map[string]string) (float64, bool) {
	value, ok := row[metric]
	if !ok || value == "<nil>" {
		// treat nil value as 0
		return 0.0, ok
	}
	valueFloat, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		logger.Error("Unable to convert current value to float (metric=" + metric +
			",metricHelp=" + metricHelp + ",value=<" + row[metric] + ">)")
		return -1, false
	}
	return valueFloat, true
}

func createMetricID(m *Metric) string {
	sb := strings.Builder{}

	sb.WriteString(m.Context)

	for _, key := range slices.Sorted(maps.Keys(m.MetricsDesc)) {
		sb.WriteString("_")
		sb.WriteString(key)
	}

	return sb.String()
}

func (m *Metric) GetLabels() []string {
	if len(m.FieldToAppend) == 0 {
		return m.Labels
	}
	// Do not include FieldToAppend in metric labels,
	// as this field is appended to the metric FQDN.
	var labels []string
	for _, label := range m.Labels {
		if label != m.FieldToAppend {
			labels = append(labels, label)
		}
	}
	return labels
}

// IsEnabledForDatabase checks if a metric is enabled for a database.
// If the m.Databases slice is nil, the metric is enabled for all databases.
// If the m.Databases slice contains the database name, the metric is enabled for that database.
// Otherwise, the metric is disabled for all databases (non-nil, empty m.Databases slice)
func (m *Metric) IsEnabledForDatabase(d *Database) bool {
	if m.Databases == nil || slices.Contains(m.Databases, d.Name) {
		return true
	}
	return false
}

func (m *Metric) normalizeIdentifiers() {
	// The configured metric key is used to read the SQL row value, and row keys are lowercased.
	normalizedDesc := make(map[string]string, len(m.MetricsDesc))
	for name, desc := range m.MetricsDesc {
		normalizedDesc[strings.ToLower(name)] = desc
	}
	m.MetricsDesc = normalizedDesc

	normalizedTypes := make(map[string]string, len(m.MetricsType))
	for name, metricType := range m.MetricsType {
		normalizedTypes[strings.ToLower(name)] = metricType
	}
	m.MetricsType = normalizedTypes

	// A histogram metric defined with mixed case will stop matching its bucket metadata.
	normalizedBuckets := make(map[string]map[string]string, len(m.MetricsBuckets))
	for name, buckets := range m.MetricsBuckets {
		normalizedName := strings.ToLower(name)
		normalizedFields := make(map[string]string, len(buckets))
		for field, value := range buckets {
			normalizedFields[strings.ToLower(field)] = value
		}
		normalizedBuckets[normalizedName] = normalizedFields
	}
	m.MetricsBuckets = normalizedBuckets

	// mixed-case label names are not allowed
	for i, label := range m.Labels {
		m.Labels[i] = strings.ToLower(label)
	}
	// mixed-case field-to-append values are not allowed
	m.FieldToAppend = strings.ToLower(m.FieldToAppend)
	m.ID = createMetricID(m)
}

func (metrics Metrics) normalizeIdentifiers() {
	for _, metric := range metrics.Metric {
		metric.normalizeIdentifiers()
	}
}

func (metrics Metrics) toMap() map[string]*Metric {
	m := map[string]*Metric{}
	for _, metric := range metrics.Metric {
		m[metric.ID] = metric
	}
	return m
}
