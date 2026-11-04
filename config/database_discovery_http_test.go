// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

// Exercise the actual SDK request and response path, including pagination across
// compartments, freeform filtering, structured profiles and target conversion.
func TestDiscoverOCIThroughSDKHTTP(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/20160918/autonomousDatabases" {
			t.Errorf("unexpected API path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("lifecycleState") != "AVAILABLE" {
			t.Error("missing lifecycle filter")
		}
		key := r.URL.Query().Get("compartmentId") + "/" + r.URL.Query().Get("page")
		requests = append(requests, key)
		var results []adb.AutonomousDatabaseSummary
		switch key {
		case "compartment-1/":
			w.Header().Set("opc-next-page", "next")
			results = []adb.AutonomousDatabaseSummary{testADBSummary("orders", map[string]string{tagKeyExporterEnabled: "true", tagKeyDatabaseName: "production-orders"})}
		case "compartment-1/next":
			results = []adb.AutonomousDatabaseSummary{testADBSummary("disabled", map[string]string{tagKeyExporterEnabled: "false"})}
		case "compartment-2/":
			item := testADBSummary("inventory", map[string]string{tagKeyExporterEnabled: "true", tagKeyConnectService: "TPURGENT"})
			item.ConnectionStrings.Profiles = []adb.DatabaseConnectionStringProfile{testADBProfile("TPURGENT", false)}
			results = []adb.AutonomousDatabaseSummary{item}
		default:
			t.Errorf("unexpected request %s", key)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(results); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	provider := common.NewRawConfigurationProvider("tenancy", "user", "us-ashburn-1", "fingerprint", string(privatePEM), nil)
	client, err := adb.NewDatabaseClientWithConfigurationProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	client.Host = server.URL
	snapshot, err := discoverOCIWithClientStats(context.Background(), testLogger(), &OCIDatabasesFromConfig{
		Auth: oci.AuthModeConfigFile, Compartments: []string{"compartment-1", "compartment-2"},
		Filters: OCIDatabasesFromFiltersConfig{LifecycleState: adb.AutonomousDatabaseLifecycleStateAvailable, RequiredTags: map[string]string{tagKeyExporterEnabled: "true"}},
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(requests, ",") != "compartment-1/,compartment-1/next,compartment-2/" {
		t.Fatalf("unexpected request sequence: %v", requests)
	}
	targets := snapshot.Databases
	if len(targets) != 2 || targets["production-orders"].ResourceID != "ocid1.autonomousdatabase.orders" || !strings.Contains(targets["inventory"].URL, "orders_tpurgent") {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}
