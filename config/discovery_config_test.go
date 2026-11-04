// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

func TestDiscoveryConfigurationDefaultsAndValidation(t *testing.T) {
	for _, tt := range []struct {
		name, yaml string
		wantError  bool
	}{
		{"defaults", "databasesFrom:\n  oci:\n    compartments: [compartment]\n", false},
		{"auth", "databasesFrom:\n  oci:\n    compartments: [compartment]\n    auth: invalid\n", true},
		{"compartments", "databasesFrom:\n  oci: {}\n", true},
		{"blank compartment", "databasesFrom:\n  oci:\n    compartments: [' ']\n", true},
		{"lifecycle", "databasesFrom:\n  oci:\n    compartments: [compartment]\n    filters:\n      lifecycleState: INVALID\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: writeExporterConfig(t, tt.yaml)})
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if !tt.wantError && (m.DatabasesFrom.OCI.Filters.LifecycleState != adb.AutonomousDatabaseLifecycleStateAvailable || m.DatabasesFrom.OCI.Filters.RequiredTags[tagKeyExporterEnabled] != "true") {
				t.Fatalf("missing defaults: %#v", m.DatabasesFrom.OCI.Filters)
			}
		})
	}
}

func TestDiscoverDatabasesMergesStaticTargets(t *testing.T) {
	original := discoverOCIStats
	t.Cleanup(func() { discoverOCIStats = original })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discoverOCIStats = func(got context.Context, _ *slog.Logger, cfg *OCIDatabasesFromConfig) (DiscoverySnapshot, error) {
		if got != ctx || cfg.Auth != oci.AuthModeWorkloadIdentity {
			t.Fatal("discovery configuration or context not passed through")
		}
		return DiscoverySnapshot{Databases: map[string]DatabaseConfig{"orders": {URL: "discovered"}, "inventory": {URL: "inventory"}}, Matched: 2}, nil
	}
	m := &MetricsConfiguration{Databases: map[string]DatabaseConfig{"orders": {URL: "static"}}, DatabasesFrom: &DatabasesFromConfig{OCI: &OCIDatabasesFromConfig{Auth: oci.AuthModeWorkloadIdentity}}}
	if err := m.DiscoverDatabases(ctx, testLogger()); err != nil {
		t.Fatal(err)
	}
	if len(m.Databases) != 2 || m.Databases["orders"].URL != "static" || m.Databases["inventory"].URL != "inventory" {
		t.Fatalf("unexpected targets: %#v", m.Databases)
	}
}

func TestDiscoveryFailurePreservesStaticTargets(t *testing.T) {
	original := discoverOCIStats
	t.Cleanup(func() { discoverOCIStats = original })
	want := errors.New("OCI unavailable")
	discoverOCIStats = func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (DiscoverySnapshot, error) {
		return DiscoverySnapshot{}, want
	}
	m := &MetricsConfiguration{Databases: map[string]DatabaseConfig{"orders": {URL: "static"}}, DatabasesFrom: &DatabasesFromConfig{OCI: &OCIDatabasesFromConfig{}}}
	if err := m.DiscoverDatabases(context.Background(), testLogger()); !errors.Is(err, want) {
		t.Fatalf("unexpected discovery error: %v", err)
	}
	if len(m.Databases) != 1 || m.Databases["orders"].URL != "static" {
		t.Fatalf("static targets changed: %#v", m.Databases)
	}
}

func TestDiscoverySupportsNoStaticTargets(t *testing.T) {
	original := discoverOCIStats
	t.Cleanup(func() { discoverOCIStats = original })
	discoverOCIStats = func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (DiscoverySnapshot, error) {
		return DiscoverySnapshot{Databases: map[string]DatabaseConfig{"orders": {URL: "orders"}}, Matched: 1}, nil
	}
	m := &MetricsConfiguration{DatabasesFrom: &DatabasesFromConfig{OCI: &OCIDatabasesFromConfig{}}}
	if err := m.DiscoverDatabases(context.Background(), testLogger()); err != nil || m.Databases["orders"].URL != "orders" {
		t.Fatalf("unexpected targets: %#v, %v", m.Databases, err)
	}
}

func TestDiscoveryTimeoutAndRefreshConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name, yaml string
		wantError  bool
	}{
		{"defaults", "databasesFrom:\n  oci:\n    compartments: [compartment]\n", false},
		{"zero timeout", "databasesFrom:\n  timeout: 0s\n  oci:\n    compartments: [compartment]\n", true},
		{"negative timeout", "databasesFrom:\n  timeout: -1s\n  oci:\n    compartments: [compartment]\n", true},
		{"negative refresh", "databasesFrom:\n  refreshInterval: -1s\n  oci:\n    compartments: [compartment]\n", true},
		{"disabled refresh", "databasesFrom:\n  refreshInterval: 0s\n  oci:\n    compartments: [compartment]\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: writeExporterConfig(t, tt.yaml)})
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected validation result: %v", err)
			}
			if tt.name == "defaults" && (cfg.DatabasesFrom.DiscoveryTimeout() != 30*time.Second || cfg.DatabasesFrom.DiscoveryRefreshInterval() != 5*time.Minute) {
				t.Fatalf("unexpected discovery defaults: %#v", cfg.DatabasesFrom)
			}
		})
	}
}

func TestDiscoveryLifecycleStateIsCanonicalized(t *testing.T) {
	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: writeExporterConfig(t, "databasesFrom:\n  oci:\n    compartments: [compartment]\n    filters:\n      lifecycleState: available\n")})
	if err != nil || cfg.DatabasesFrom.OCI.Filters.LifecycleState != adb.AutonomousDatabaseLifecycleStateAvailable {
		t.Fatalf("expected canonical lifecycle enum, got %#v, %v", cfg, err)
	}
}

func TestDiscoveryWalletRefreshValidation(t *testing.T) {
	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: writeExporterConfig(t, "databasesFrom:\n  oci:\n    compartments: [compartment]\n")})
	if err != nil || cfg.DatabasesFrom.DiscoveryWalletRefreshInterval() != 24*time.Hour {
		t.Fatalf("unexpected default wallet interval: %#v, %v", cfg, err)
	}
	for _, value := range []time.Duration{-time.Second, 0, time.Hour} {
		cfg.DatabasesFrom.WalletRefreshInterval = &value
		if err := cfg.validateDatabasesFromConfig(); (err != nil) != (value < 0) {
			t.Fatalf("unexpected validation for %v: %v", value, err)
		}
	}
}
