// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package main

import (
	"testing"

	"github.com/prometheus/common/version"
)

func TestSyncBuildVersionUsesExporterVersionWhenPrometheusVersionEmpty(t *testing.T) {
	originalMainVersion := Version
	originalPromVersion := version.Version
	t.Cleanup(func() {
		Version = originalMainVersion
		version.Version = originalPromVersion
	})

	Version = "2.3.1-test"
	version.Version = ""

	syncBuildVersion()

	if version.Version != Version {
		t.Fatalf("expected prometheus version %q, got %q", Version, version.Version)
	}
}

func TestSyncBuildVersionPreservesExplicitPrometheusVersion(t *testing.T) {
	originalMainVersion := Version
	originalPromVersion := version.Version
	t.Cleanup(func() {
		Version = originalMainVersion
		version.Version = originalPromVersion
	})

	Version = "2.3.1-test"
	version.Version = "2.3.1-explicit"

	syncBuildVersion()

	if version.Version != "2.3.1-explicit" {
		t.Fatalf("expected explicit prometheus version to be preserved, got %q", version.Version)
	}
}

func TestGetEnvUsesFallbackWhenMissing(t *testing.T) {
	t.Setenv("METRICS_EXPORTER_TEST_ENV", "")
	if got := getEnv("METRICS_EXPORTER_TEST_ENV_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback value, got %q", got)
	}
}

func TestGetEnvReturnsConfiguredValue(t *testing.T) {
	t.Setenv("METRICS_EXPORTER_TEST_ENV", "configured")
	if got := getEnv("METRICS_EXPORTER_TEST_ENV", "fallback"); got != "configured" {
		t.Fatalf("expected configured value, got %q", got)
	}
}
