// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
	"go.yaml.in/yaml/v2"
)

const (
	tagPrefix                    = "oracledb-metrics-exporter-"
	tagKeyExporterEnabled        = tagPrefix + "enabled"
	tagKeyVaultID                = tagPrefix + "vault-id"
	tagKeyUsernamePasswordSecret = tagPrefix + "usernamePasswordSecret"
	tagKeyConnectService         = tagPrefix + "connect-service"
	tagKeyWalletSecret           = tagPrefix + "wallet-secret"
	tagKeyMTLSConnectionRequired = tagPrefix + "is-mtls-connection-required"
)

var discoveryTagKeys = map[string]struct{}{
	tagKeyExporterEnabled:        {},
	tagKeyVaultID:                {},
	tagKeyUsernamePasswordSecret: {},
	tagKeyConnectService:         {},
	tagKeyWalletSecret:           {},
	tagKeyMTLSConnectionRequired: {},
}

func discoverDatabases(logger *slog.Logger, cfg *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
	databases := make(map[string]DatabaseConfig)
	if cfg == nil { // no databases to discover
		return databases, nil
	}

	if cfg.OCI != nil {
		ociDatabases, err := discoverOCIDatabases(logger, cfg.OCI)
		if err != nil {
			return nil, err
		}
		for name, database := range ociDatabases {
			databases[name] = database
		}
	}

	return databases, nil
}

func discoverOCIDatabases(logger *slog.Logger, cfg *OCIDatabasesFromConfig) (map[string]DatabaseConfig, error) {
	var databases = make(map[string]DatabaseConfig)

	configProvider, err := oci.ConfigurationProviderForAuthMode(cfg.Auth)
	if err != nil {
		return nil, err
	}
	adbClient, err := adb.NewDatabaseClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, err
	}

	var autonomousDatabases []adb.AutonomousDatabaseSummary

	for _, compartment := range cfg.Compartments {
		results, listErr := oci.ListAllDatabases(context.Background(), adbClient, compartment, cfg.Filters.RequiredTags, cfg.Filters.LifecycleState)
		if listErr != nil {
			return nil, listErr
		}
		autonomousDatabases = append(autonomousDatabases, results...)
	}

	for _, autonomousDatabase := range autonomousDatabases {
		addAutonomousDatabase(databases, autonomousDatabase, cfg.Auth, logger)
	}

	return databases, nil
}

func addAutonomousDatabase(databases map[string]DatabaseConfig, autonomousDatabase adb.AutonomousDatabaseSummary, auth oci.AuthMode, logger *slog.Logger) {
	name := strings.TrimSpace(stringValue(autonomousDatabase.DbName))
	if name == "" {
		logger.Warn("skipping discovered Autonomous Database without a database name")
		return
	}
	if _, exists := databases[name]; exists {
		logger.Warn("skipping duplicate discovered Autonomous Database", "database", name)
		return
	}

	database, err := databaseConfigFromAutonomousDatabase(autonomousDatabase, auth, logger)
	if err != nil {
		logger.Warn("skipping discovered Autonomous Database", "database", name, "err", err)
		return
	}
	databases[name] = database
}

func databaseConfigFromAutonomousDatabase(autonomousDatabase adb.AutonomousDatabaseSummary, auth oci.AuthMode, logger *slog.Logger) (DatabaseConfig, error) {
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

	var database DatabaseConfig
	if len(configTags) > 0 {
		configYAML, err := yaml.Marshal(configTags)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("marshal database configuration tags: %w", err)
		}
		if err := yaml.UnmarshalStrict(configYAML, &database); err != nil {
			return DatabaseConfig{}, fmt.Errorf("decode database configuration tags: %w", err)
		}
	}

	vaultID, usernamePasswordSecret := tags[tagKeyVaultID], tags[tagKeyUsernamePasswordSecret]
	if (vaultID == "") != (usernamePasswordSecret == "") {
		return DatabaseConfig{}, fmt.Errorf("both %q and %q are required for OCI Vault credentials", tagKeyVaultID, tagKeyUsernamePasswordSecret)
	}
	if vaultID != "" {
		database.Vault = &VaultConfig{OCI: &OCIVault{
			ID:                     vaultID,
			Auth:                   auth,
			UsernamePasswordSecret: usernamePasswordSecret,
		}}
	}

	if walletSecret := tags[tagKeyWalletSecret]; walletSecret != "" {
		// TODO: resolve wallet-secret after mTLS wallet setup is implemented.
		logger.Debug("ignoring discovered wallet secret until mTLS wallet setup is supported", "database", stringValue(autonomousDatabase.DbName))
	}

	if connectService := tags[tagKeyConnectService]; connectService != "" {
		connectionString, err := connectionStringForService(autonomousDatabase.ConnectionStrings, connectService)
		if err != nil {
			return DatabaseConfig{}, err
		}
		database.URL = connectionString
	}

	return database, nil
}

func isDiscoveryTag(key string) bool {
	_, ok := discoveryTagKeys[key]
	return ok
}

func connectionStringForService(connectionStrings *adb.AutonomousDatabaseConnectionStrings, service string) (string, error) {
	if connectionStrings == nil {
		return "", fmt.Errorf("Autonomous Database has no connection strings")
	}

	service = strings.ToUpper(strings.TrimSpace(service))
	var connectionString *string
	switch service {
	case "LOW":
		connectionString = connectionStrings.Low
	case "MEDIUM":
		connectionString = connectionStrings.Medium
	case "HIGH":
		connectionString = connectionStrings.High
	case "DEDICATED":
		connectionString = connectionStrings.Dedicated
	default:
		return "", fmt.Errorf("unsupported connection service %q", service)
	}

	if strings.TrimSpace(stringValue(connectionString)) == "" {
		return "", fmt.Errorf("Autonomous Database has no %s connection string", service)
	}
	return stringValue(connectionString), nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
