// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

func TestDatabaseConfigFromAutonomousDatabase(t *testing.T) {
	maxOpenConns := 20
	database, err := databaseConfigFromAutonomousDatabase(adb.AutonomousDatabaseSummary{
		DbName: common.String("orders"),
		FreeformTags: map[string]string{
			tagKeyExporterEnabled:         "true",
			tagKeyVaultID:                 "ocid1.vault.oc1..example",
			tagKeyUsernamePasswordSecret:  "orders-credentials",
			tagKeyConnectService:          "HIGH",
			tagPrefix + "role":            "SYSDBA",
			tagPrefix + "connMaxLifetime": "5m",
			tagPrefix + "maxOpenConns":    "20",
			tagKeyMTLSConnectionRequired:  "true",
		},
		ConnectionStrings: &adb.AutonomousDatabaseConnectionStrings{High: common.String("orders_high")},
	}, oci.AuthModeWorkloadIdentity, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("expected database configuration, got %v", err)
	}

	if database.URL != "orders_high" || database.Role != "SYSDBA" || database.MaxOpenConns == nil || *database.MaxOpenConns != maxOpenConns {
		t.Fatalf("unexpected database configuration: %#v", database)
	}
	if database.ConnMaxLifetime == nil || *database.ConnMaxLifetime != 5*time.Minute {
		t.Fatalf("expected connection max lifetime from tag, got %#v", database.ConnMaxLifetime)
	}
	if database.Vault == nil || database.Vault.OCI == nil || database.Vault.OCI.ID != "ocid1.vault.oc1..example" || database.Vault.OCI.UsernamePasswordSecret != "orders-credentials" || database.Vault.OCI.Auth != oci.AuthModeWorkloadIdentity {
		t.Fatalf("unexpected OCI Vault configuration: %#v", database.Vault)
	}
}

func TestDatabaseConfigFromAutonomousDatabaseRequiresBothOCIVaultTags(t *testing.T) {
	for name, tags := range map[string]map[string]string{
		"vault ID only": {tagKeyVaultID: "ocid1.vault.oc1..example"},
		"secret only":   {tagKeyUsernamePasswordSecret: "orders-credentials"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := databaseConfigFromAutonomousDatabase(adb.AutonomousDatabaseSummary{FreeformTags: tags}, oci.AuthModeConfigFile, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err == nil || !strings.Contains(err.Error(), tagKeyVaultID) || !strings.Contains(err.Error(), tagKeyUsernamePasswordSecret) {
				t.Fatalf("expected incomplete OCI Vault tag error, got %v", err)
			}
		})
	}
}

func TestDatabaseConfigFromAutonomousDatabaseDecodesConnectionConfigTags(t *testing.T) {
	database, err := databaseConfigFromAutonomousDatabase(adb.AutonomousDatabaseSummary{
		FreeformTags: map[string]string{
			tagPrefix + "externalAuth": "true",
			tagPrefix + "labels":       "team: payments",
		},
	}, oci.AuthModeConfigFile, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("expected connection config tags to decode, got %v", err)
	}
	if !database.ExternalAuth || database.Labels["team"] != "payments" {
		t.Fatalf("unexpected decoded connection config: %#v", database)
	}
}

func TestDatabaseConfigFromAutonomousDatabaseRejectsInvalidConfigTags(t *testing.T) {
	for name, tags := range map[string]map[string]string{
		"unknown config field": {tagPrefix + "notAConfigField": "value"},
		"wrong config type":    {tagPrefix + "maxOpenConns": "not-a-number"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := databaseConfigFromAutonomousDatabase(adb.AutonomousDatabaseSummary{FreeformTags: tags}, oci.AuthModeConfigFile, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err == nil {
				t.Fatal("expected invalid configuration tag error")
			}
		})
	}
}

func TestDatabaseConfigFromAutonomousDatabaseIgnoresUnrelatedTags(t *testing.T) {
	database, err := databaseConfigFromAutonomousDatabase(adb.AutonomousDatabaseSummary{
		FreeformTags: map[string]string{
			"Department": "Finance",
		},
	}, oci.AuthModeConfigFile, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("expected unrelated tag to be ignored, got %v", err)
	}
	if database.URL != "" || database.Vault != nil || database.Labels != nil || database.ExternalAuth {
		t.Fatalf("expected empty database config, got %#v", database)
	}
}

func TestAddAutonomousDatabaseSkipsDuplicateAndMalformedDatabases(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	databases := make(map[string]DatabaseConfig)
	first := adb.AutonomousDatabaseSummary{
		DbName:            common.String("orders"),
		FreeformTags:      map[string]string{tagKeyConnectService: "HIGH"},
		ConnectionStrings: &adb.AutonomousDatabaseConnectionStrings{High: common.String("first")},
	}
	duplicate := adb.AutonomousDatabaseSummary{
		DbName:            common.String("orders"),
		FreeformTags:      map[string]string{tagKeyConnectService: "HIGH"},
		ConnectionStrings: &adb.AutonomousDatabaseConnectionStrings{High: common.String("second")},
	}
	malformed := adb.AutonomousDatabaseSummary{
		DbName:       common.String("bad"),
		FreeformTags: map[string]string{tagKeyConnectService: "TPURGENT"},
	}
	missingName := adb.AutonomousDatabaseSummary{
		FreeformTags: map[string]string{tagKeyConnectService: "HIGH"},
	}

	addAutonomousDatabase(databases, first, oci.AuthModeConfigFile, logger)
	addAutonomousDatabase(databases, duplicate, oci.AuthModeConfigFile, logger)
	addAutonomousDatabase(databases, malformed, oci.AuthModeConfigFile, logger)
	addAutonomousDatabase(databases, missingName, oci.AuthModeConfigFile, logger)

	if len(databases) != 1 || databases["orders"].URL != "first" {
		t.Fatalf("expected first duplicate to be preserved, got %#v", databases)
	}
	if got := logs.String(); !strings.Contains(got, "skipping duplicate discovered Autonomous Database") || !strings.Contains(got, "skipping discovered Autonomous Database") || !strings.Contains(got, "without a database name") {
		t.Fatalf("expected warnings for skipped databases, got %q", got)
	}
}

func TestConnectionStringForService(t *testing.T) {
	connectionStrings := &adb.AutonomousDatabaseConnectionStrings{
		Low:       common.String("low"),
		Medium:    common.String("medium"),
		High:      common.String("high"),
		Dedicated: common.String("dedicated"),
	}
	for name, test := range map[string]struct {
		connectionStrings *adb.AutonomousDatabaseConnectionStrings
		service           string
		want              string
		wantErr           bool
	}{
		"LOW":                 {connectionStrings, "LOW", "low", false},
		"MEDIUM":              {connectionStrings, "MEDIUM", "medium", false},
		"HIGH":                {connectionStrings, " high ", "high", false},
		"DEDICATED":           {connectionStrings, "dedicated", "dedicated", false},
		"missing connection":  {&adb.AutonomousDatabaseConnectionStrings{}, "LOW", "", true},
		"missing all strings": {nil, "LOW", "", true},
		"unsupported service": {connectionStrings, "TPURGENT", "", true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := connectionStringForService(test.connectionStrings, test.service)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected connection service error")
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("expected %q, got %q, %v", test.want, got, err)
			}
		})
	}
}
