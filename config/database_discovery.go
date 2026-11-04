// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
	"go.yaml.in/yaml/v2"
)

const (
	tagPrefix                    = "oracledb-metrics-exporter-"
	tagKeyExporterEnabled        = tagPrefix + "enabled"
	tagKeyDatabaseName           = tagPrefix + "dbname"
	tagKeyVaultID                = tagPrefix + "vault-id"
	tagKeyUsernamePasswordSecret = tagPrefix + "usernamePasswordSecret"
	tagKeyConnectService         = tagPrefix + "connect-service"
	tagKeyWalletSecret           = tagPrefix + "wallet-secret"
	tagKeyMTLSConnectionRequired = tagPrefix + "is-mtls-connection-required"
)

var discoveryTagKeys = map[string]struct{}{
	tagKeyExporterEnabled:        {},
	tagKeyDatabaseName:           {},
	tagKeyVaultID:                {},
	tagKeyUsernamePasswordSecret: {},
	tagKeyConnectService:         {},
	tagKeyWalletSecret:           {},
	tagKeyMTLSConnectionRequired: {},
}

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

func discoverOCIWithClient(ctx context.Context, logger *slog.Logger, cfg *OCIDatabasesFromConfig, adbClient oci.AutonomousDatabaseLister) (map[string]DatabaseConfig, error) {
	snapshot, err := discoverOCIWithClientStats(ctx, logger, cfg, adbClient)
	return snapshot.Databases, err
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
	if strings.TrimSpace(database.URL) == "" {
		logger.Warn("skipping discovered Autonomous Database without a connection URL; set connect-service or url tag", "database", name)
		return
	}
	databases[name] = database
}

func databaseConfigFromAutonomousDatabase(autonomousDatabase adb.AutonomousDatabaseSummary, auth oci.AuthMode) (DatabaseConfig, error) {
	tags := autonomousDatabase.FreeformTags
	configTags := make(map[string]interface{})
	for key, value := range tags {
		if !strings.HasPrefix(key, tagPrefix) || isDiscoveryTag(key) {
			continue
		}

		var tagValue interface{}
		if err := yaml.Unmarshal([]byte(value), &tagValue); err != nil {
			return DatabaseConfig{}, fmt.Errorf("decode value for tag %q: %w", key, err)
		}
		configTags[strings.TrimPrefix(key, tagPrefix)] = tagValue
	}

	database := DatabaseConfig{ResourceID: strings.TrimSpace(stringValue(autonomousDatabase.Id))}
	if len(configTags) > 0 {
		configYAML, err := yaml.Marshal(configTags)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("marshal database configuration tags: %w", err)
		}
		if err := yaml.UnmarshalStrict(configYAML, &database); err != nil {
			return DatabaseConfig{}, fmt.Errorf("decode database configuration tags: %w", err)
		}
	}

	vaultID, usernamePasswordSecret, walletSecret := tags[tagKeyVaultID], tags[tagKeyUsernamePasswordSecret], tags[tagKeyWalletSecret]
	if vaultID == "" && (usernamePasswordSecret != "" || walletSecret != "") {
		return DatabaseConfig{}, fmt.Errorf("%q is required when %q or %q is configured", tagKeyVaultID, tagKeyUsernamePasswordSecret, tagKeyWalletSecret)
	}
	if vaultID != "" && usernamePasswordSecret == "" && walletSecret == "" {
		return DatabaseConfig{}, fmt.Errorf("%q or %q is required when %q is configured", tagKeyUsernamePasswordSecret, tagKeyWalletSecret, tagKeyVaultID)
	}
	if usernamePasswordSecret != "" {
		database.Vault = &VaultConfig{OCI: &OCIVault{
			ID:                     vaultID,
			Auth:                   auth,
			UsernamePasswordSecret: usernamePasswordSecret,
		}}
	}

	mutual := autonomousDatabase.IsMtlsConnectionRequired != nil && *autonomousDatabase.IsMtlsConnectionRequired
	var explicitMutual *bool
	if value, present := tags[tagKeyMTLSConnectionRequired]; present {
		requested, err := strconv.ParseBool(value)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("invalid %s tag: %w", tagKeyMTLSConnectionRequired, err)
		}
		if mutual && !requested {
			return DatabaseConfig{}, fmt.Errorf("%s=false conflicts with the database's mTLS requirement", tagKeyMTLSConnectionRequired)
		}
		mutual = requested
		explicitMutual = &requested
	}
	// An existing wallet or wallet secret explicitly requests a mutual profile.
	if walletSecret != "" || database.TNSAdmin != "" {
		if explicitMutual != nil && !*explicitMutual {
			return DatabaseConfig{}, fmt.Errorf("a wallet conflicts with %s=false", tagKeyMTLSConnectionRequired)
		}
		mutual = true
	}
	service := strings.ToUpper(strings.TrimSpace(tags[tagKeyConnectService]))
	if service == "" {
		service = "LOW"
	}
	if database.URL == "" || tags[tagKeyConnectService] != "" {
		connectionString, err := connectionProfileForService(autonomousDatabase.ConnectionStrings, service, mutual)
		// If OCI only advertises mutual profiles, prefer a working wallet-based
		// connection unless the user explicitly requested walletless TLS.
		if err != nil && !mutual {
			if _, explicit := tags[tagKeyMTLSConnectionRequired]; !explicit {
				if fallback, fallbackErr := connectionProfileForService(autonomousDatabase.ConnectionStrings, service, true); fallbackErr == nil {
					connectionString, err, mutual = fallback, nil, true
				}
			}
		}
		if err != nil {
			return DatabaseConfig{}, err
		}
		database.URL = connectionString
	} else {
		// Explicit overrides must retain encrypted transport and certificate checks.
		secured, err := verifiedTCPSDescriptor(database.URL)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("invalid discovered database URL: %w", err)
		}
		database.URL = secured
	}
	if mutual && database.TNSAdmin == "" {
		if database.ResourceID == "" {
			return DatabaseConfig{}, fmt.Errorf("database OCID is required to prepare an mTLS wallet")
		}
		database.DiscoveryWallet = &DiscoveryWalletConfig{Auth: auth, DatabaseID: database.ResourceID, VaultID: vaultID, SecretName: walletSecret}
	}

	return database, nil
}

func isDiscoveryTag(key string) bool {
	_, ok := discoveryTagKeys[key]
	return ok
}

// connectionStringForService selects the walletless TLS profile for a service.
func connectionStringForService(connectionStrings *adb.AutonomousDatabaseConnectionStrings, service string) (string, error) {
	return connectionProfileForService(connectionStrings, service, false)
}

func connectionProfileForService(connectionStrings *adb.AutonomousDatabaseConnectionStrings, service string, mutual bool) (string, error) {
	service = strings.ToUpper(strings.TrimSpace(service))
	switch service {
	case "LOW", "MEDIUM", "HIGH", "TP", "TPURGENT":
	default:
		return "", fmt.Errorf("unsupported connection service %q; use LOW, MEDIUM, HIGH, TP, or TPURGENT", service)
	}
	if connectionStrings == nil {
		return "", fmt.Errorf("Autonomous Database has no connection profiles")
	}
	tlsAuth := adb.DatabaseConnectionStringProfileTlsAuthenticationServer
	if mutual {
		tlsAuth = adb.DatabaseConnectionStringProfileTlsAuthenticationMutual
	}
	type candidate struct {
		value string
		rank  int
	}
	var candidates []candidate
	for _, profile := range connectionStrings.Profiles {
		if !strings.EqualFold(string(profile.ConsumerGroup), service) || profile.Protocol != adb.DatabaseConnectionStringProfileProtocolTcps || profile.TlsAuthentication != tlsAuth {
			continue
		}
		// A database's FQDN profile is preferred over IP addresses, and direct
		// profiles over redirects. Deterministic sorting avoids polling changes
		// merely because OCI reordered its profiles.
		rank := 0
		if profile.HostFormat != adb.DatabaseConnectionStringProfileHostFormatFqdn {
			rank += 2
		}
		if profile.SessionMode != adb.DatabaseConnectionStringProfileSessionModeDirect {
			rank++
		}
		value, err := verifiedTCPSDescriptor(stringValue(profile.Value))
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{value: value, rank: rank})
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("Autonomous Database has no verified TCPS %s profile for service %s", tlsAuth, service)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		return candidates[i].value < candidates[j].value
	})
	return candidates[0].value, nil
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
