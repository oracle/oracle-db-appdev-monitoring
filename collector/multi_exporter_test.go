// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

const customMetricFixture = `[[metric]]
context = "custom_instances"
metricsdesc = { value = "Custom test metric." }
request = "select 1 as value from dual"
`

func TestCheckIfMetricsChangedIsTrackedPerExporter(t *testing.T) {
	customMetricsPath := writeCustomMetricsFixture(t, customMetricFixture)

	first := newTestExporterWithCustomMetrics(customMetricsPath)
	if !first.checkIfMetricsChanged() {
		t.Fatal("expected first exporter to detect initial custom metrics load")
	}
	first.reloadMetrics()
	assertMetricLoaded(t, first, "custom_instances_value")

	second := newTestExporterWithCustomMetrics(customMetricsPath)
	if !second.checkIfMetricsChanged() {
		t.Fatal("expected second exporter to detect initial custom metrics load independently")
	}
	second.reloadMetrics()
	assertMetricLoaded(t, second, "custom_instances_value")
}

func TestCheckIfMetricsChangedReloadsEachExporterAfterFileUpdate(t *testing.T) {
	customMetricsPath := writeCustomMetricsFixture(t, customMetricFixture)

	first := newTestExporterWithCustomMetrics(customMetricsPath)
	second := newTestExporterWithCustomMetrics(customMetricsPath)

	if !first.checkIfMetricsChanged() {
		t.Fatal("expected first exporter to detect initial custom metrics load")
	}
	first.reloadMetrics()
	if !second.checkIfMetricsChanged() {
		t.Fatal("expected second exporter to detect initial custom metrics load")
	}
	second.reloadMetrics()

	updatedMetrics := `[[metric]]
context = "custom_instances"
metricsdesc = { value = "Updated custom test metric." }
request = "select 2 as value from dual"
`
	if err := os.WriteFile(customMetricsPath, []byte(updatedMetrics), 0o600); err != nil {
		t.Fatalf("failed to update custom metrics fixture: %v", err)
	}

	if !first.checkIfMetricsChanged() {
		t.Fatal("expected first exporter to detect updated custom metrics")
	}
	first.reloadMetrics()
	assertMetricRequest(t, first, "custom_instances_value", "select 2 as value from dual")

	if !second.checkIfMetricsChanged() {
		t.Fatal("expected second exporter to detect updated custom metrics independently")
	}
	second.reloadMetrics()
	assertMetricRequest(t, second, "custom_instances_value", "select 2 as value from dual")
}

func newTestExporterWithCustomMetrics(customMetricsPath string) *Exporter {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewExporter(logger, &MetricsConfiguration{
		Metrics: MetricsFilesConfig{
			Custom: []string{customMetricsPath},
		},
	})
}

func writeCustomMetricsFixture(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "custom-metrics.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write custom metrics fixture: %v", err)
	}
	return path
}

func assertMetricLoaded(t *testing.T, exporter *Exporter, metricID string) {
	t.Helper()

	metric := exporter.metricsToScrape[metricID]
	if metric == nil {
		t.Fatalf("expected metric %q to be loaded", metricID)
	}
}

func assertMetricRequest(t *testing.T, exporter *Exporter, metricID, want string) {
	t.Helper()

	metric := exporter.metricsToScrape[metricID]
	if metric == nil {
		t.Fatalf("expected metric %q to be loaded", metricID)
	}
	if metric.Request != want {
		t.Fatalf("expected metric %q request %q, got %q", metricID, want, metric.Request)
	}
}
