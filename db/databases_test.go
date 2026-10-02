// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"path/filepath"
	"testing"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func TestNewDatabasesRetainsClientInfoForReconnect(t *testing.T) {
	disabled := false
	clientInfo := config.ClientInfoConfig{Label: "custom_exporter", Enabled: &disabled}
	databases := NewDatabases(testLogger(), &config.MetricsConfiguration{
		Metrics: config.MetricsFilesConfig{ClientInfo: clientInfo},
		Databases: map[string]config.DatabaseConfig{
			"db1": {PasswordFile: filepath.Join(t.TempDir(), "missing-password")},
			"db2": {PasswordFile: filepath.Join(t.TempDir(), "missing-password")},
		},
	})
	if len(databases) != 2 {
		t.Fatalf("expected both databases, got %d", len(databases))
	}
	for _, database := range databases {
		if database.ClientInfo.GetLabel() != "custom_exporter" || database.ClientInfo.IsEnabled() {
			t.Fatalf("database %s did not retain client info settings: %+v", database.Name, database.ClientInfo)
		}
	}
}
