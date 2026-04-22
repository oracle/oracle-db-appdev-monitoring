// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadYamlMetricsConfigNormalizesIdentifiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.yaml")
	content := `
metrics:
  - context: Sessions
    labels: ["Inst_ID"]
    metricsdesc:
      Value: help
    request: select 1 as value from dual
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write yaml metrics: %v", err)
	}

	metrics := &Metrics{}
	if err := loadMetricsConfig(path, metrics); err != nil {
		t.Fatalf("load yaml metrics: %v", err)
	}
	if len(metrics.Metric) != 1 {
		t.Fatalf("expected one metric, got %d", len(metrics.Metric))
	}
	if metrics.Metric[0].Labels[0] != "inst_id" {
		t.Fatalf("expected normalized labels, got %v", metrics.Metric[0].Labels)
	}
	if _, ok := metrics.Metric[0].MetricsDesc["value"]; !ok {
		t.Fatalf("expected normalized metricsdesc key, got %v", metrics.Metric[0].MetricsDesc)
	}
}

func TestLoadYamlMetricsConfigReturnsReadError(t *testing.T) {
	err := loadYamlMetricsConfig(filepath.Join(t.TempDir(), "missing.yaml"), &Metrics{})
	if err == nil {
		t.Fatal("expected read error for missing yaml file")
	}
}

func TestDefaultMetricsLoadsConfiguredFileAndFallsBackToEmbedded(t *testing.T) {
	customDefault := filepath.Join(t.TempDir(), "default-metrics.toml")
	content := `[[metric]]
context = "custom"
metricsdesc = { value = "custom help" }
request = "select 1 as value from dual"
`
	if err := os.WriteFile(customDefault, []byte(content), 0o600); err != nil {
		t.Fatalf("write custom default metrics: %v", err)
	}

	exporter := &Exporter{
		logger:               testLogger(),
		MetricsConfiguration: &MetricsConfiguration{Metrics: MetricsFilesConfig{Default: customDefault}},
	}
	metrics := exporter.DefaultMetrics()
	if metrics["custom_value"] == nil {
		t.Fatalf("expected configured default metrics file to be loaded, got %v", metrics)
	}

	exporter.Metrics.Default = filepath.Join(t.TempDir(), "missing.toml")
	metrics = exporter.DefaultMetrics()
	if len(metrics) == 0 {
		t.Fatal("expected embedded default metrics fallback")
	}
}
