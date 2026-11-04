// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"log/slog"
	"testing"

	adb "github.com/oracle/oci-go-sdk/v65/database"
)

type discoveryStatsLister struct {
	items []adb.AutonomousDatabaseSummary
}

func (l discoveryStatsLister) ListAutonomousDatabases(context.Context, adb.ListAutonomousDatabasesRequest) (adb.ListAutonomousDatabasesResponse, error) {
	return adb.ListAutonomousDatabasesResponse{Items: l.items}, nil
}

func TestDiscoveryStatsCountOnlyMatchingResourcesAndConversionSkips(t *testing.T) {
	enabled := map[string]string{tagKeyExporterEnabled: "true"}
	invalid := map[string]string{tagKeyExporterEnabled: "true", tagKeyDatabaseName: " "}
	snapshot, err := discoverOCIWithClientStats(context.Background(), testLogger(), &OCIDatabasesFromConfig{
		Compartments: []string{"compartment"}, Filters: OCIDatabasesFromFiltersConfig{RequiredTags: enabled},
	}, discoveryStatsLister{items: []adb.AutonomousDatabaseSummary{
		testADBSummary("orders", enabled), testADBSummary("orders", enabled), testADBSummary("bad", invalid), testADBSummary("disabled", nil),
	}})
	if err != nil || snapshot.Matched != 3 || snapshot.Skipped != 2 || len(snapshot.Databases) != 1 {
		t.Fatalf("incorrect discovery stats: %#v, %v", snapshot, err)
	}
}

func TestDiscoveryStatsIncludeStaticCollisionsAtMaterialization(t *testing.T) {
	original := discoverOCIStats
	t.Cleanup(func() { discoverOCIStats = original })
	discoverOCIStats = func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (DiscoverySnapshot, error) {
		return DiscoverySnapshot{Databases: map[string]DatabaseConfig{"orders": {URL: "discovered"}, "inventory": {URL: "inventory"}}, Matched: 3, Skipped: 1}, nil
	}
	m := &MetricsConfiguration{Databases: map[string]DatabaseConfig{"orders": {URL: "static"}}, DatabasesFrom: &DatabasesFromConfig{OCI: &OCIDatabasesFromConfig{}}}
	snapshot, err := m.DiscoverDatabasesWithStats(context.Background(), testLogger())
	if err != nil || snapshot.Matched != 3 || snapshot.Skipped != 2 || len(snapshot.Databases) != 2 || snapshot.Databases["orders"].URL != "static" {
		t.Fatalf("incorrect materialized stats: %#v, %v", snapshot, err)
	}
}

func TestMutualOnlyProfilesInferWallet(t *testing.T) {
	resource := testADBSummary("orders", nil)
	resource.IsMtlsConnectionRequired = nil
	resource.ConnectionStrings.Profiles = []adb.DatabaseConnectionStringProfile{testADBProfile("LOW", true)}
	database, err := databaseConfigFromAutonomousDatabase(resource, "")
	if err != nil || database.DiscoveryWallet == nil {
		t.Fatalf("mutual-only capability not inferred: %#v, %v", database, err)
	}
	resource.FreeformTags = map[string]string{tagKeyMTLSConnectionRequired: "false"}
	if _, err = databaseConfigFromAutonomousDatabase(resource, ""); err == nil {
		t.Fatal("explicit walletless requirement ignored")
	}
}

func TestDiscoveryDuplicateResolutionIgnoresOCIListOrder(t *testing.T) {
	a := testADBSummary("a", map[string]string{tagKeyDatabaseName: "orders"})
	b := testADBSummary("b", map[string]string{tagKeyDatabaseName: "orders"})
	for _, items := range [][]adb.AutonomousDatabaseSummary{{a, b}, {b, a}} {
		snapshot, err := discoverOCIWithClientStats(context.Background(), testLogger(), &OCIDatabasesFromConfig{Compartments: []string{"compartment"}}, discoveryStatsLister{items: items})
		if err != nil || snapshot.Databases["orders"].ResourceID != *a.Id || snapshot.Matched != 2 || snapshot.Skipped != 1 {
			t.Fatalf("unstable duplicate resolution: %#v, %v", snapshot, err)
		}
	}
}
