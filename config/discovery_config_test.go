// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
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
	cfg := monitorConfiguration()
	cfg.Databases = map[string]DatabaseConfig{"orders": {URL: "static"}}
	cfg.DatabasesFrom.OCI.Auth = oci.AuthModeWorkloadIdentity
	m := NewDiscoveryMonitor(cfg)
	m.snapshot = func(ctx context.Context, _ *slog.Logger, source *OCIDatabasesFromConfig) (discoverySnapshot, error) {
		if _, ok := ctx.Deadline(); !ok || source.Auth != oci.AuthModeWorkloadIdentity {
			t.Fatal("discovery timeout or configuration not passed through")
		}
		return discoverySnapshot{Databases: map[string]DatabaseConfig{"orders": {URL: "discovered"}, "inventory": {URL: "inventory"}}, Matched: 2}, nil
	}
	if err := m.Initialize(context.Background(), testLogger()); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Databases) != 2 || cfg.Databases["orders"].URL != "static" || cfg.Databases["inventory"].URL != "inventory" {
		t.Fatalf("unexpected targets: %#v", cfg.Databases)
	}
}

func TestDiscoverySupportsNoStaticTargets(t *testing.T) {
	cfg := monitorConfiguration()
	m := NewDiscoveryMonitor(cfg)
	m.snapshot = func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (discoverySnapshot, error) {
		return discoverySnapshot{Databases: map[string]DatabaseConfig{"orders": {URL: "orders"}}, Matched: 1}, nil
	}
	if err := m.Initialize(context.Background(), testLogger()); err != nil || cfg.Databases["orders"].URL != "orders" {
		t.Fatalf("unexpected targets: %#v, %v", cfg.Databases, err)
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
