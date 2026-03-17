// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"io"
	"log/slog"
	"testing"
)

func TestMetricNormalizeIdentifiersSetsDeterministicID(t *testing.T) {
	metric := &Metric{
		Context: "sessions",
		MetricsDesc: map[string]string{
			"B": "second",
			"a": "first",
		},
	}

	metric.normalizeIdentifiers()

	if metric.ID != "sessions_a_b" {
		t.Fatalf("expected normalized metric ID %q, got %q", "sessions_a_b", metric.ID)
	}
}

func TestMetricsToMapUsesStableMetricID(t *testing.T) {
	firstDesc := make(map[string]string, 2)
	firstDesc["b"] = "second"
	firstDesc["a"] = "first"

	secondDesc := make(map[string]string, 2)
	secondDesc["a"] = "first"
	secondDesc["b"] = "second"

	metrics := Metrics{
		Metric: []*Metric{
			{
				Context:     "sessions",
				MetricsDesc: firstDesc,
				Request:     "first",
			},
			{
				Context:     "sessions",
				MetricsDesc: secondDesc,
				Request:     "second",
			},
		},
	}

	metrics.normalizeIdentifiers()
	got := metrics.toMap()

	if len(got) != 1 {
		t.Fatalf("expected one merged metric, got %d", len(got))
	}
	if got["sessions_a_b"] == nil {
		t.Fatalf("expected merged metric keyed by %q", "sessions_a_b")
	}
	if got["sessions_a_b"].Request != "second" {
		t.Fatalf("expected later metric to overwrite existing entry, got request %q", got["sessions_a_b"].Request)
	}
}

func TestDefaultMetricsAssignsIDs(t *testing.T) {
	exporter := &Exporter{
		MetricsConfiguration: &MetricsConfiguration{},
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	metrics := exporter.DefaultMetrics()

	if len(metrics) == 0 {
		t.Fatal("expected embedded default metrics to load")
	}

	for id, metric := range metrics {
		if id == "" {
			t.Fatal("expected default metric map key to be non-empty")
		}
		if metric == nil {
			t.Fatalf("expected metric for ID %q", id)
		}
		if metric.ID != id {
			t.Fatalf("expected metric ID %q to match map key, got %q", id, metric.ID)
		}
	}
}

func TestMetricGetLabels(t *testing.T) {
	tests := []struct {
		name     string
		metric   Metric
		expected []string
	}{
		{
			name: "returns all labels when field to append is empty",
			metric: Metric{
				Labels: []string{"database", "instance"},
			},
			expected: []string{"database", "instance"},
		},
		{
			name: "omits field to append from labels",
			metric: Metric{
				Labels:        []string{"database", "instance", "sql_id"},
				FieldToAppend: "sql_id",
			},
			expected: []string{"database", "instance"},
		},
		{
			name: "returns labels unchanged when field to append is not present",
			metric: Metric{
				Labels:        []string{"database", "instance"},
				FieldToAppend: "sql_id",
			},
			expected: []string{"database", "instance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metric.GetLabels()
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d labels, got %d (%v)", len(tt.expected), len(got), got)
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Fatalf("expected labels %v, got %v", tt.expected, got)
				}
			}
		})
	}
}

func TestMetricIsEnabledForDatabase(t *testing.T) {
	tests := []struct {
		name     string
		metric   Metric
		database Database
		expected bool
	}{
		{
			name: "enabled for all databases when databases is nil",
			metric: Metric{
				Databases: nil,
			},
			database: Database{Name: "prod"},
			expected: true,
		},
		{
			name: "enabled when database is listed",
			metric: Metric{
				Databases: []string{"prod", "staging"},
			},
			database: Database{Name: "prod"},
			expected: true,
		},
		{
			name: "disabled when database is not listed",
			metric: Metric{
				Databases: []string{"staging"},
			},
			database: Database{Name: "prod"},
			expected: false,
		},
		{
			name: "disabled for all databases when list is empty but non nil",
			metric: Metric{
				Databases: []string{},
			},
			database: Database{Name: "prod"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metric.IsEnabledForDatabase(&tt.database)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestParseFloat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name     string
		metric   string
		row      map[string]string
		expected float64
		ok       bool
	}{
		{
			name:     "parses valid float",
			metric:   "value",
			row:      map[string]string{"value": "42.5"},
			expected: 42.5,
			ok:       true,
		},
		{
			name:     "trims whitespace",
			metric:   "value",
			row:      map[string]string{"value": "  7.25  "},
			expected: 7.25,
			ok:       true,
		},
		{
			name:     "treats nil string as zero",
			metric:   "value",
			row:      map[string]string{"value": "<nil>"},
			expected: 0,
			ok:       true,
		},
		{
			name:     "returns zero and false when key is missing",
			metric:   "value",
			row:      map[string]string{},
			expected: 0,
			ok:       false,
		},
		{
			name:     "returns error sentinel for invalid float",
			metric:   "value",
			row:      map[string]string{"value": "not-a-number"},
			expected: -1,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseFloat(logger, tt.metric, "help", tt.row)
			if got != tt.expected || ok != tt.ok {
				t.Fatalf("expected (%v, %v), got (%v, %v)", tt.expected, tt.ok, got, ok)
			}
		})
	}
}
