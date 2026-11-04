// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

func testADBProfile(service string, mutual bool) adb.DatabaseConnectionStringProfile {
	auth := adb.DatabaseConnectionStringProfileTlsAuthenticationServer
	if mutual {
		auth = adb.DatabaseConnectionStringProfileTlsAuthenticationMutual
	}
	return adb.DatabaseConnectionStringProfile{
		Value:             common.String("(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orders_" + strings.ToLower(service) + "))(SECURITY=(SSL_SERVER_CERT_DN=\"CN=adb.example.com\")))"),
		ConsumerGroup:     adb.DatabaseConnectionStringProfileConsumerGroupEnum(service),
		Protocol:          adb.DatabaseConnectionStringProfileProtocolTcps,
		TlsAuthentication: auth,
		HostFormat:        adb.DatabaseConnectionStringProfileHostFormatFqdn,
		SessionMode:       adb.DatabaseConnectionStringProfileSessionModeDirect,
		SyntaxFormat:      adb.DatabaseConnectionStringProfileSyntaxFormatLong,
	}
}

func testADBSummary(name string, tags map[string]string) adb.AutonomousDatabaseSummary {
	return adb.AutonomousDatabaseSummary{
		Id: common.String("ocid1.autonomousdatabase." + name), DbName: common.String(name), FreeformTags: tags,
		IsMtlsConnectionRequired: common.Bool(false),
		ConnectionStrings:        &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{testADBProfile("LOW", false), testADBProfile("HIGH", false), testADBProfile("LOW", true)}},
	}
}

func TestDatabaseConfigFromAutonomousDatabase(t *testing.T) {
	database, err := databaseConfigFromAutonomousDatabase(testADBSummary("orders", map[string]string{
		tagKeyDatabaseName: "production-orders", tagKeyExporterEnabled: "true", tagKeyVaultID: "vault", tagKeyUsernamePasswordSecret: "credentials", tagKeyConnectService: "HIGH",
		tagPrefix + "role": "SYSDBA", tagPrefix + "connMaxLifetime": "5m", tagPrefix + "maxOpenConns": "20", tagPrefix + "labels": "{team: payments}",
	}), oci.AuthModeWorkloadIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(database.URL, "orders_high") || !strings.Contains(database.URL, "SSL_SERVER_DN_MATCH=YES") || database.Role != "SYSDBA" || database.MaxOpenConns == nil || *database.MaxOpenConns != 20 || database.ConnMaxLifetime == nil || *database.ConnMaxLifetime != 5*time.Minute || database.Labels["team"] != "payments" {
		t.Fatalf("unexpected config: %#v", database)
	}
	if database.ResourceID != "ocid1.autonomousdatabase.orders" || database.Vault == nil || database.Vault.OCI.Auth != oci.AuthModeWorkloadIdentity || database.Vault.OCI.UsernamePasswordSecret != "credentials" {
		t.Fatalf("missing OCI metadata/credentials: %#v", database)
	}
}

func TestDatabaseConfigDefaultsToLowTLS(t *testing.T) {
	database, err := databaseConfigFromAutonomousDatabase(testADBSummary("orders", nil), oci.AuthModeConfigFile)
	if err != nil || !strings.Contains(database.URL, "orders_low") || database.DiscoveryWallet != nil {
		t.Fatalf("unexpected default config: %#v, %v", database, err)
	}
}

func TestDatabaseConfigRejectsInvalidTags(t *testing.T) {
	for name, tags := range map[string]map[string]string{
		"vault only":                {tagKeyVaultID: "vault"},
		"credentials without vault": {tagKeyUsernamePasswordSecret: "credentials"},
		"wallet without vault":      {tagKeyWalletSecret: "wallet"},
		"unknown field":             {tagPrefix + "notAConfigField": "value"},
		"invalid type":              {tagPrefix + "maxOpenConns": "not-a-number"},
		"invalid mtls flag":         {tagKeyMTLSConnectionRequired: "invalid"},
		"wallet secret with TLS":    {tagKeyMTLSConnectionRequired: "false", tagKeyVaultID: "vault", tagKeyWalletSecret: "wallet"},
		"mounted wallet with TLS":   {tagKeyMTLSConnectionRequired: "false", tagPrefix + "tnsAdmin": "/wallet"},
		"insecure URL":              {tagPrefix + "url": "adb.example.com:1521/orders_low"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := databaseConfigFromAutonomousDatabase(testADBSummary("orders", tags), oci.AuthModeConfigFile); err == nil {
				t.Fatal("expected invalid tag error")
			}
		})
	}
}

func TestDatabaseConfigIgnoresUnrelatedTags(t *testing.T) {
	database, err := databaseConfigFromAutonomousDatabase(testADBSummary("orders", map[string]string{"Department": "Finance"}), oci.AuthModeConfigFile)
	if err != nil || database.Vault != nil || database.Labels != nil || database.ExternalAuth {
		t.Fatalf("unexpected config: %#v, %v", database, err)
	}
}

func TestDatabaseConfigMTLSMetadataAndPreference(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		actual                *bool
		tag                   string
		wantWallet, wantError bool
	}{
		{name: "required metadata", actual: common.Bool(true), wantWallet: true},
		{name: "required contradiction", actual: common.Bool(true), tag: "false", wantError: true},
		{name: "explicit mutual", actual: common.Bool(false), tag: "true", wantWallet: true},
		{name: "explicit server", actual: common.Bool(false), tag: "false"},
		{name: "unknown capability", actual: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			adb := testADBSummary("orders", map[string]string{})
			adb.IsMtlsConnectionRequired = tt.actual
			if tt.tag != "" {
				adb.FreeformTags[tagKeyMTLSConnectionRequired] = tt.tag
			}
			database, err := databaseConfigFromAutonomousDatabase(adb, oci.AuthModeConfigFile)
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantError && (database.DiscoveryWallet != nil) != tt.wantWallet {
				t.Fatalf("unexpected wallet config: %#v", database.DiscoveryWallet)
			}
		})
	}
}

func TestDatabaseConfigWalletReference(t *testing.T) {
	adb := testADBSummary("orders", map[string]string{tagKeyVaultID: "vault", tagKeyWalletSecret: "wallet"})
	database, err := databaseConfigFromAutonomousDatabase(adb, oci.AuthModeInstancePrincipal)
	if err != nil || database.DiscoveryWallet == nil || database.DiscoveryWallet.VaultID != "vault" || database.DiscoveryWallet.SecretName != "wallet" || database.DiscoveryWallet.DatabaseID != *adb.Id {
		t.Fatalf("unexpected wallet reference: %#v, %v", database, err)
	}
	// A mounted wallet avoids downloads but still selects a mutual profile.
	adb.FreeformTags = map[string]string{tagPrefix + "tnsAdmin": "/wallet"}
	database, err = databaseConfigFromAutonomousDatabase(adb, oci.AuthModeConfigFile)
	if err != nil || database.DiscoveryWallet != nil || database.TNSAdmin != "/wallet" {
		t.Fatalf("unexpected mounted wallet: %#v, %v", database, err)
	}
}

func TestAddAutonomousDatabaseNameOverride(t *testing.T) {
	for _, tt := range []struct {
		name, original string
		tags           map[string]string
		want           string
	}{
		{name: "default", original: "orders", want: "orders"},
		{name: "override", original: "orders", tags: map[string]string{tagKeyDatabaseName: "production-orders"}, want: "production-orders"},
		{name: "trim", original: "orders", tags: map[string]string{tagKeyDatabaseName: "  production-orders  "}, want: "production-orders"},
		{name: "missing original", tags: map[string]string{tagKeyDatabaseName: "production-orders"}, want: "production-orders"},
		{name: "empty override", original: "orders", tags: map[string]string{tagKeyDatabaseName: ""}},
		{name: "blank override", original: "orders", tags: map[string]string{tagKeyDatabaseName: "  "}},
		{name: "missing both"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			databases := make(map[string]DatabaseConfig)
			addAutonomousDatabase(databases, testADBSummary(tt.original, tt.tags), oci.AuthModeConfigFile, testLogger())
			if tt.want == "" {
				if len(databases) != 0 {
					t.Fatalf("expected skip, got %#v", databases)
				}
				return
			}
			if len(databases) != 1 || !strings.Contains(databases[tt.want].URL, "orders_low") {
				t.Fatalf("unexpected targets: %#v", databases)
			}
		})
	}
}

func TestAddAutonomousDatabaseOverrideCollisions(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	databases := make(map[string]DatabaseConfig)
	add := func(original, name string) {
		addAutonomousDatabase(databases, testADBSummary(original, map[string]string{tagKeyDatabaseName: name}), oci.AuthModeConfigFile, logger)
	}
	add("orders", "production-orders")
	add("orders", "staging-orders")
	add("another", "production-orders")
	if len(databases) != 2 || databases["production-orders"].ResourceID != "ocid1.autonomousdatabase.orders" || !strings.Contains(logs.String(), "existingResourceID") {
		t.Fatalf("unexpected targets/logs: %#v %s", databases, logs.String())
	}
}
