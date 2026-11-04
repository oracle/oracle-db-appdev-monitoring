// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

// DiscoveryWalletConfig identifies wallet material without downloading it during
// metadata polling. The directory is materialized only for active targets.
type DiscoveryWalletConfig struct {
	Auth       oci.AuthMode
	DatabaseID string
	VaultID    string
	SecretName string
}

// DiscoverySnapshot reports resource matches and conversion outcomes. Matched is
// counted after lifecycle/tag filtering; Skipped includes invalid/duplicate names.
type DiscoverySnapshot struct {
	Databases map[string]DatabaseConfig
	Matched   int
	Skipped   int
}

var discoverOCIStats = discoverOCIDatabasesWithStats

func DiscoverDatabaseSnapshotWithStats(ctx context.Context, logger *slog.Logger, cfg *DatabasesFromConfig) (DiscoverySnapshot, error) {
	if cfg == nil || cfg.OCI == nil {
		return DiscoverySnapshot{Databases: make(map[string]DatabaseConfig)}, nil
	}
	return discoverOCIStats(ctx, logger, cfg.OCI)
}

// DiscoverDatabaseSnapshot resolves metadata without materializing wallets.
func DiscoverDatabaseSnapshot(ctx context.Context, logger *slog.Logger, cfg *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
	snapshot, err := DiscoverDatabaseSnapshotWithStats(ctx, logger, cfg)
	return snapshot.Databases, err
}

func discoverOCIDatabasesWithStats(ctx context.Context, logger *slog.Logger, cfg *OCIDatabasesFromConfig) (DiscoverySnapshot, error) {
	configProvider, err := oci.ConfigurationProviderForAuthMode(cfg.Auth)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	adbClient, err := adb.NewDatabaseClientWithConfigurationProvider(configProvider)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	return discoverOCIWithClientStats(ctx, logger, cfg, adbClient)
}

func discoverOCIWithClientStats(ctx context.Context, logger *slog.Logger, cfg *OCIDatabasesFromConfig, adbClient oci.AutonomousDatabaseLister) (DiscoverySnapshot, error) {
	databases := make(map[string]DatabaseConfig)
	var autonomousDatabases []adb.AutonomousDatabaseSummary
	for _, compartment := range cfg.Compartments {
		results, err := oci.ListAllDatabases(ctx, adbClient, compartment, cfg.Filters.RequiredTags, cfg.Filters.LifecycleState)
		if err != nil {
			return DiscoverySnapshot{}, fmt.Errorf("list Autonomous Databases in compartment %q: %w", compartment, err)
		}
		autonomousDatabases = append(autonomousDatabases, results...)
	}
	// OCI response order is not an identity contract. Resolve name collisions
	// deterministically so list reordering does not change the active resource.
	sort.SliceStable(autonomousDatabases, func(i, j int) bool {
		return stringValue(autonomousDatabases[i].Id) < stringValue(autonomousDatabases[j].Id)
	})
	for _, database := range autonomousDatabases {
		addAutonomousDatabase(databases, database, cfg.Auth, logger)
	}
	return DiscoverySnapshot{Databases: databases, Matched: len(autonomousDatabases), Skipped: len(autonomousDatabases) - len(databases)}, nil
}

func addAutonomousDatabase(databases map[string]DatabaseConfig, autonomousDatabase adb.AutonomousDatabaseSummary, auth oci.AuthMode, logger *slog.Logger) {
	name := strings.TrimSpace(stringValue(autonomousDatabase.DbName))
	if override, exists := autonomousDatabase.FreeformTags[tagKeyDatabaseName]; exists {
		name = strings.TrimSpace(override)
		if name == "" {
			logger.Warn("skipping discovered Autonomous Database with an empty database name override", "tag", tagKeyDatabaseName, "database OCID", stringValue(autonomousDatabase.Id))
			return
		}
	}
	if name == "" {
		logger.Warn("skipping discovered Autonomous Database without a database name")
		return
	}
	if _, exists := databases[name]; exists {
		logger.Warn("skipping duplicate discovered Autonomous Database", "database", name, "resourceID", stringValue(autonomousDatabase.Id), "existingResourceID", databases[name].ResourceID)
		return
	}

	database, err := databaseConfigFromAutonomousDatabase(autonomousDatabase, auth)
	if err != nil {
		logger.Warn("skipping discovered Autonomous Database", "database", name, "resourceID", stringValue(autonomousDatabase.Id), "err", err)
		return
	}
	databases[name] = database
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// DiscoverDatabases resolves configured discovery sources and merges them into
// the static targets. Call once at startup, after configuration validation.
// Config-file reloads restart the process and run discovery again.
func (m *MetricsConfiguration) DiscoverDatabases(ctx context.Context, logger *slog.Logger) error {
	_, err := m.DiscoverDatabasesWithStats(ctx, logger)
	return err
}

// DiscoverDatabasesWithStats materializes the accepted targets atomically and
// reports conversion skips plus collisions with static target names.
func (m *MetricsConfiguration) DiscoverDatabasesWithStats(ctx context.Context, logger *slog.Logger) (DiscoverySnapshot, error) {
	snapshot, err := DiscoverDatabaseSnapshotWithStats(ctx, logger, m.DatabasesFrom)
	if err != nil {
		return DiscoverySnapshot{}, fmt.Errorf("discover databases: %w", err)
	}
	collisions, err := m.mergeDiscoveredDatabases(ctx, logger, snapshot.Databases)
	if err != nil {
		return DiscoverySnapshot{}, err
	}
	snapshot.Skipped += collisions
	snapshot.Databases = m.Databases
	return snapshot, nil
}

func (m *MetricsConfiguration) mergeDiscoveredDatabases(ctx context.Context, logger *slog.Logger, discovered map[string]DatabaseConfig) (int, error) {
	next := make(map[string]DatabaseConfig, len(m.Databases)+len(discovered))
	for name, database := range m.Databases {
		next[name] = database
	}
	collisions := 0
	for name, database := range discovered {
		if _, exists := next[name]; exists {
			logger.Warn("skipping discovered database that conflicts with static configuration", "database", name, "resourceID", database.ResourceID)
			collisions++
			continue
		}
		if wallet := database.DiscoveryWallet; wallet != nil {
			directory, err := oci.PrepareWallet(ctx, wallet.Auth, wallet.DatabaseID, wallet.VaultID, wallet.SecretName)
			if err != nil {
				return collisions, fmt.Errorf("prepare wallet for database %q: %w", name, err)
			}
			database.TNSAdmin = directory
		}
		next[name] = database
	}
	candidate := *m
	candidate.Databases = next
	if err := candidate.validateOCIVaultAuth(); err != nil {
		return collisions, err
	}
	m.Databases = next
	return collisions, nil
}
