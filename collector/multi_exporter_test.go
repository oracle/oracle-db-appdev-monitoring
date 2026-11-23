// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

const customMetricFixture = `[[metric]]
context = "custom_instances"
metricsdesc = { value = "Custom test metric." }
request = "select 1 as value from dual"
`

func TestNewExporterFingerprintsInitialCustomMetrics(t *testing.T) {
	customMetricsPath := writeCustomMetricsFixture(t, customMetricFixture)

	first := newTestExporterWithCustomMetrics(customMetricsPath)
	if first.checkIfMetricsChanged() {
		t.Fatal("unexpected change after the first exporter loaded custom metrics")
	}
	assertMetricLoaded(t, first, "custom_instances_value")

	second := newTestExporterWithCustomMetrics(customMetricsPath)
	if second.checkIfMetricsChanged() {
		t.Fatal("unexpected change after the second exporter loaded custom metrics")
	}
	assertMetricLoaded(t, second, "custom_instances_value")
}

func TestCheckIfMetricsChangedReloadsEachExporterAfterFileUpdate(t *testing.T) {
	customMetricsPath := writeCustomMetricsFixture(t, customMetricFixture)

	first := newTestExporterWithCustomMetrics(customMetricsPath)
	second := newTestExporterWithCustomMetrics(customMetricsPath)

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

func TestReloadMetricsKeepsLastGoodMetricsOnParseError(t *testing.T) {
	customMetricsPath := writeCustomMetricsFixture(t, customMetricFixture)
	exporter := newTestExporterWithCustomMetrics(customMetricsPath)

	assertMetricRequest(t, exporter, "custom_instances_value", "select 1 as value from dual")

	invalidMetrics := `[[metric]]
context = "custom_instances"
metricsdesc = { value = "Broken custom test metric."
request = "select 2 as value from dual"
`
	if err := os.WriteFile(customMetricsPath, []byte(invalidMetrics), 0o600); err != nil {
		t.Fatalf("failed to write invalid custom metrics fixture: %v", err)
	}

	if !exporter.checkIfMetricsChanged() {
		t.Fatal("expected invalid custom metrics update to be detected")
	}
	if exporter.reloadMetrics() {
		t.Fatal("expected metrics reload to fail for invalid custom metrics")
	}
	assertMetricRequest(t, exporter, "custom_instances_value", "select 1 as value from dual")

	if !exporter.checkIfMetricsChanged() {
		t.Fatal("expected invalid custom metrics to continue appearing changed until a reload succeeds")
	}
}

func TestReloadMetricsDetectsFileReplacedAfterDecode(t *testing.T) {
	path := writeCustomMetricsFixture(t, customMetricFixture)
	exporter := newTestExporterWithCustomMetrics(path)
	secondVersion := strings.Replace(customMetricFixture, "select 1", "select 2", 1)
	thirdVersion := strings.Replace(customMetricFixture, "select 1", "select 3", 1)
	if err := os.WriteFile(path, []byte(secondVersion), 0o600); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(path), "replacement.toml")
	if err := os.WriteFile(replacement, []byte(thirdVersion), 0o600); err != nil {
		t.Fatal(err)
	}
	replaced := false
	exporter.logger = slog.New(metricLoadHandler{
		Handler: slog.NewTextHandler(io.Discard, nil),
		loaded: func() {
			if !replaced {
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
				replaced = true
			}
		},
	})

	if !exporter.checkIfMetricsChanged() || !exporter.reloadMetrics() || !replaced {
		t.Fatal("expected a reload followed by an atomic replacement")
	}
	assertMetricRequest(t, exporter, "custom_instances_value", "select 2 as value from dual")
	if !exporter.checkIfMetricsChanged() || !exporter.reloadMetrics() {
		t.Fatal("expected the next reload to pick up the replacement")
	}
	assertMetricRequest(t, exporter, "custom_instances_value", "select 3 as value from dual")
	if exporter.checkIfMetricsChanged() {
		t.Fatal("unexpected reload for the already loaded file")
	}
}

type metricLoadHandler struct {
	slog.Handler
	loaded func()
}

func (h metricLoadHandler) Handle(ctx context.Context, record slog.Record) error {
	if strings.HasPrefix(record.Message, "Successfully loaded custom metrics from ") {
		h.loaded()
	}
	return h.Handler.Handle(ctx, record)
}

func newTestExporterWithCustomMetrics(customMetricsPath string) *Exporter {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewExporter(logger, &config.MetricsConfiguration{
		Metrics: config.MetricsFilesConfig{
			Custom: []string{customMetricsPath},
		},
	}, nil)
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
