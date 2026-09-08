// Copyright (c) 2024, 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl

package config

import (
	"maps"
	"slices"
	"strings"
)

// Metric is an object description loaded from a metric definition file.
type Metric struct {
	ID               string
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

// Metrics is a container structure for metric definitions.
type Metrics struct {
	Metric []*Metric `yaml:"metrics"`
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

// IsEnabledForDatabase checks if a metric is enabled for a database name.
// If the m.Databases slice is nil, the metric is enabled for all databases.
// If the m.Databases slice contains the database name, the metric is enabled for that database.
// Otherwise, the metric is disabled for all databases (non-nil, empty m.Databases slice).
func (m *Metric) IsEnabledForDatabase(databaseName string) bool {
	if m.Databases == nil || slices.Contains(m.Databases, databaseName) {
		return true
	}
	return false
}

func createMetricID(m *Metric) string {
	var id strings.Builder

	id.WriteString(m.Context)

	for _, key := range slices.Sorted(maps.Keys(m.MetricsDesc)) {
		id.WriteString("_")
		id.WriteString(key)
	}

	return id.String()
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
	result := map[string]*Metric{}
	for _, metric := range metrics.Metric {
		result[metric.ID] = metric
	}
	return result
}
