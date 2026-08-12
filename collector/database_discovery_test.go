// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"io"
	"log/slog"
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

func TestConnectionStringForService(t *testing.T) {
	connectionStrings := &adb.AutonomousDatabaseConnectionStrings{Dedicated: common.String("dedicated")}
	got, err := connectionStringForService(connectionStrings, "dedicated")
	if err != nil || got != "dedicated" {
		t.Fatalf("expected dedicated connection string, got %q, %v", got, err)
	}

	if _, err := connectionStringForService(connectionStrings, "TPURGENT"); err == nil {
		t.Fatal("expected unsupported service error")
	}
}
