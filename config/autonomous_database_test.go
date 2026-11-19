// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

func TestConnectionStringForAllServices(t *testing.T) {
	for _, service := range []string{"HIGH", "MEDIUM", "LOW", "TP", "TPURGENT"} {
		t.Run(service, func(t *testing.T) {
			profiles := &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{testADBProfile(service, false), testADBProfile(service, true)}}
			got, err := connectionProfileForService(profiles, " "+strings.ToLower(service)+" ", false)
			if err != nil || !strings.Contains(got, "orders_"+strings.ToLower(service)) {
				t.Fatalf("unexpected service: %q, %v", got, err)
			}
		})
	}
}

func TestConnectionProfileRequiresMatchingMetadata(t *testing.T) {
	wrongProtocol := testADBProfile("LOW", false)
	wrongProtocol.Protocol = adb.DatabaseConnectionStringProfileProtocolTcp
	for _, tt := range []struct {
		name, service string
		profiles      *adb.AutonomousDatabaseConnectionStrings
	}{
		{name: "nil", service: "LOW"},
		{name: "legacy only", service: "LOW", profiles: &adb.AutonomousDatabaseConnectionStrings{Low: common.String("adb.example.com:1522/orders_low")}},
		{name: "unsupported service", service: "DEDICATED", profiles: &adb.AutonomousDatabaseConnectionStrings{}},
		{name: "wrong group", service: "TP", profiles: &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{testADBProfile("LOW", false)}}},
		{name: "wrong auth", service: "LOW", profiles: &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{testADBProfile("LOW", true)}}},
		{name: "wrong protocol", service: "LOW", profiles: &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{wrongProtocol}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := connectionProfileForService(tt.profiles, tt.service, false); err == nil {
				t.Fatal("expected missing matching profile error")
			}
		})
	}
}

func TestConnectionProfileSelectionIsDeterministic(t *testing.T) {
	first := testADBProfile("LOW", false)
	second := first
	second.Value = common.String(strings.ReplaceAll(*first.Value, "adb.example.com", "z.example.com"))
	for _, profiles := range [][]adb.DatabaseConnectionStringProfile{{first, second}, {second, first}} {
		got, err := connectionProfileForService(&adb.AutonomousDatabaseConnectionStrings{Profiles: profiles}, "LOW", false)
		if err != nil || got != *first.Value {
			t.Fatalf("got %q, %v; want unchanged profile %q", got, err, *first.Value)
		}
	}
}

func TestDatabaseConfigPreservesExplicitURL(t *testing.T) {
	for _, value := range []string{"adb.example.com:1521/orders_low", "invalid connection string"} {
		summary := testADBSummary("orders", map[string]string{tagPrefix + "url": value})
		database, err := databaseConfigFromAutonomousDatabase(summary, oci.AuthModeConfigFile)
		if err != nil || database.URL != value {
			t.Fatalf("got URL %q, %v; want unchanged override %q", database.URL, err, value)
		}
		summary.FreeformTags[tagKeyConnectService] = "HIGH"
		database, err = databaseConfigFromAutonomousDatabase(summary, oci.AuthModeConfigFile)
		if err != nil || database.URL != *summary.ConnectionStrings.Profiles[1].Value {
			t.Fatalf("explicit service did not override URL: %q, %v", database.URL, err)
		}
	}
}
