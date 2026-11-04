// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	adb "github.com/oracle/oci-go-sdk/v65/database"
)

func TestConnectionStringForAllServices(t *testing.T) {
	for _, service := range []string{"HIGH", "MEDIUM", "LOW", "TP", "TPURGENT"} {
		t.Run(service, func(t *testing.T) {
			profiles := &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{testADBProfile(service, false), testADBProfile(service, true)}}
			got, err := connectionProfileForService(profiles, " "+strings.ToLower(service)+" ", false)
			if err != nil || !strings.Contains(got, "orders_"+strings.ToLower(service)) || !strings.Contains(got, "SSL_SERVER_DN_MATCH=YES") {
				t.Fatalf("unexpected service: %q, %v", got, err)
			}
		})
	}
}

func TestConnectionProfileRejectsMissingOrInsecureProfiles(t *testing.T) {
	insecure := testADBProfile("LOW", false)
	insecure.Value = common.String(strings.Replace(*insecure.Value, "PROTOCOL=TCPS", "PROTOCOL=TCP", 1))
	noVerify := testADBProfile("LOW", false)
	noVerify.Value = common.String(strings.Replace(*noVerify.Value, "(SECURITY=", "(SECURITY=(SSL_SERVER_DN_MATCH=NO)", 1))
	for _, tt := range []struct {
		name, service string
		profiles      *adb.AutonomousDatabaseConnectionStrings
	}{
		{name: "nil", service: "LOW"},
		{name: "legacy only", service: "LOW", profiles: &adb.AutonomousDatabaseConnectionStrings{Low: common.String("adb.example.com:1522/orders_low")}},
		{name: "unsupported service", service: "DEDICATED", profiles: &adb.AutonomousDatabaseConnectionStrings{}},
		{name: "wrong group", service: "TP", profiles: &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{testADBProfile("LOW", false)}}},
		{name: "wrong auth", service: "LOW", profiles: &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{testADBProfile("LOW", true)}}},
		{name: "insecure protocol", service: "LOW", profiles: &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{insecure}}},
		{name: "disabled verification", service: "LOW", profiles: &adb.AutonomousDatabaseConnectionStrings{Profiles: []adb.DatabaseConnectionStringProfile{noVerify}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := connectionProfileForService(tt.profiles, tt.service, false); err == nil {
				t.Fatal("expected missing verified profile error")
			}
		})
	}
}

func TestConnectionProfileSelectionIsDeterministic(t *testing.T) {
	preferred := testADBProfile("LOW", false)
	redirected := preferred
	redirected.SessionMode = adb.DatabaseConnectionStringProfileSessionModeRedirect
	redirected.Value = common.String(strings.ReplaceAll(*redirected.Value, "adb.example.com", "a.example.com"))
	ip := preferred
	ip.HostFormat = adb.DatabaseConnectionStringProfileHostFormatIp
	ip.Value = common.String(strings.ReplaceAll(*ip.Value, "adb.example.com", "192.0.2.1"))
	later := preferred
	later.Value = common.String(strings.ReplaceAll(*later.Value, "adb.example.com", "z.example.com"))
	insecure := preferred
	insecure.Value = common.String(strings.ReplaceAll(*insecure.Value, "PROTOCOL=TCPS", "PROTOCOL=TCP"))

	for _, tt := range []struct {
		name          string
		first, second adb.DatabaseConnectionStringProfile
	}{
		{"direct over redirect", preferred, redirected},
		{"FQDN over IP", redirected, ip},
		{"lexical tie-breaker", preferred, later},
		{"skip insecure profile", preferred, insecure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want, err := verifiedTCPSDescriptor(*tt.first.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, profiles := range [][]adb.DatabaseConnectionStringProfile{{tt.first, tt.second}, {tt.second, tt.first}} {
				got, err := connectionProfileForService(&adb.AutonomousDatabaseConnectionStrings{Profiles: profiles}, "LOW", false)
				if err != nil || got != want {
					t.Fatalf("got %q, %v; want %q", got, err, want)
				}
			}
		})
	}
}
