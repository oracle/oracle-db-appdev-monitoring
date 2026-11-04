// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"fmt"
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

func databaseConfigFromAutonomousDatabase(autonomousDatabase adb.AutonomousDatabaseSummary, auth oci.AuthMode) (DatabaseConfig, error) {
	tags := autonomousDatabase.FreeformTags
	configTags := make(map[string]interface{})
	for key, value := range tags {
		_, discoveryTag := discoveryTagKeys[key]
		if !strings.HasPrefix(key, tagPrefix) || discoveryTag {
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
	value, explicitMutual := tags[tagKeyMTLSConnectionRequired]
	if explicitMutual {
		requested, err := strconv.ParseBool(value)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("invalid %s tag: %w", tagKeyMTLSConnectionRequired, err)
		}
		if mutual && !requested {
			return DatabaseConfig{}, fmt.Errorf("%s=false conflicts with the database's mTLS requirement", tagKeyMTLSConnectionRequired)
		}
		mutual = requested
	}
	// An existing wallet or wallet secret explicitly requests a mutual profile.
	if walletSecret != "" || database.TNSAdmin != "" {
		if explicitMutual && !mutual {
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
