// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import "testing"

func TestDuplicatedLabels(t *testing.T) {
	tests := []struct {
		name        string
		constLabels map[string]string
		labels      []string
		expected    bool
	}{
		{
			name:        "No overlap",
			constLabels: map[string]string{"env": "prod"},
			labels:      []string{"service", "instance"},
			expected:    false,
		},
		{
			name:        "Overlap",
			constLabels: map[string]string{"env": "prod", "service": "app"},
			labels:      []string{"service", "instance"},
			expected:    true,
		},
		{
			name:        "Multiple overlaps",
			constLabels: map[string]string{"env": "prod"},
			labels:      []string{"env", "service"},
			expected:    true,
		},
		{
			name:        "Empty constLabels",
			constLabels: map[string]string{},
			labels:      []string{"service"},
			expected:    false,
		},
		{
			name:        "Empty labels",
			constLabels: map[string]string{"env": "prod"},
			labels:      []string{},
			expected:    false,
		},
		{
			name:        "Both empty",
			constLabels: map[string]string{},
			labels:      []string{},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := duplicatedLabels(tt.constLabels, tt.labels)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMetricsNormalizeIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, metrics *Metrics)
	}{
		{
			name: "metrics desc key",
			check: func(t *testing.T, metrics *Metrics) {
				if _, ok := metrics.Metric[0].MetricsDesc["sqlid_without_profile_on_wcr_pta_multi_deep_bin_v"]; !ok {
					t.Fatal("expected metricsdesc key to be normalized to lowercase")
				}
			},
		},
		{
			name: "metrics bucket key",
			check: func(t *testing.T, metrics *Metrics) {
				if _, ok := metrics.Metric[0].MetricsBuckets["sqlid_without_profile_on_wcr_pta_multi_deep_bin_v"]; !ok {
					t.Fatal("expected metricsbuckets key to be normalized to lowercase")
				}
			},
		},
		{
			name: "metrics bucket field key",
			check: func(t *testing.T, metrics *Metrics) {
				if _, ok := metrics.Metric[0].MetricsBuckets["sqlid_without_profile_on_wcr_pta_multi_deep_bin_v"]["bucket_1"]; !ok {
					t.Fatal("expected histogram bucket field key to be normalized to lowercase")
				}
			},
		},
		{
			name: "field to append",
			check: func(t *testing.T, metrics *Metrics) {
				if metrics.Metric[0].FieldToAppend != "sql_id" {
					t.Fatalf("expected fieldtoappend to be normalized to lowercase, got %q", metrics.Metric[0].FieldToAppend)
				}
			},
		},
		{
			name: "labels",
			check: func(t *testing.T, metrics *Metrics) {
				if metrics.Metric[0].Labels[0] != "sql_id" || metrics.Metric[0].Labels[1] != "inst_id" {
					t.Fatalf("expected labels to be normalized to lowercase, got %v", metrics.Metric[0].Labels)
				}
			},
		},
		{
			name: "all loaded metrics",
			check: func(t *testing.T, metrics *Metrics) {
				if metrics.Metric[1].FieldToAppend != "db_name" {
					t.Fatalf("expected second metric fieldtoappend to be normalized to lowercase, got %q", metrics.Metric[1].FieldToAppend)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &Metrics{
				Metric: []*Metric{
					{
						Labels:        []string{"SQL_ID", "Inst_ID"},
						FieldToAppend: "SQL_ID",
						MetricsDesc: map[string]string{
							"sqlid_without_profile_on_WCR_PTA_MULTI_DEEP_BIN_V": "test metric",
						},
						MetricsType: map[string]string{
							"sqlid_without_profile_on_WCR_PTA_MULTI_DEEP_BIN_V": "histogram",
						},
						MetricsBuckets: map[string]map[string]string{
							"sqlid_without_profile_on_WCR_PTA_MULTI_DEEP_BIN_V": {
								"Bucket_1": "1",
							},
						},
					},
					{
						Labels:        []string{"DB_NAME"},
						FieldToAppend: "DB_NAME",
					},
				},
			}

			metrics.normalizeIdentifiers()
			tt.check(t, metrics)
		})
	}
}
