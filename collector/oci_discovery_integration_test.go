// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
	"github.com/prometheus/client_golang/prometheus"
)

// OCI_DISCOVERY_TEST_CONFIG opts into real OCI API calls, Vault reads, and SQL
// queries. For mTLS targets without wallet-secret, discovery generates a wallet.
// Use a dedicated test compartment and tagged database; ordinary runs skip this.
func TestOCIDiscoveryLiveScrape(t *testing.T) {
	filename := os.Getenv("OCI_DISCOVERY_TEST_CONFIG")
	if filename == "" {
		t.Skip("OCI_DISCOVERY_TEST_CONFIG is not set")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m, err := config.LoadMetricsConfiguration(logger, &config.Config{ConfigFile: filename})
	if err != nil {
		t.Fatal(err)
	}
	if m.DatabasesFrom == nil || m.DatabasesFrom.OCI == nil || len(m.Databases) != 0 {
		t.Fatal("live discovery test requires databasesFrom.oci and no static databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	defer func() {
		if err := oci.CleanupWallets(); err != nil {
			t.Error(err)
		}
	}()
	if err := m.DiscoverDatabases(ctx, logger); err != nil {
		t.Fatal(err)
	}
	if len(m.Databases) == 0 {
		t.Fatal("no usable tagged databases discovered")
	}
	for name, cfg := range m.Databases {
		if cfg.ResourceID == "" || cfg.Vault == nil || cfg.Vault.OCI == nil || cfg.Vault.OCI.UsernamePasswordSecret == "" {
			t.Fatalf("target %q must have OCI resource identity and a combined Vault credential secret", name)
		}
		// Keep the live probe small and bounded regardless of ordinary metric settings.
		one, timeout := 1, 5
		cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.QueryTimeout = &one, &one, &timeout
		m.Databases[name] = cfg
	}
	metricFile := filepath.Join(t.TempDir(), "probe.toml")
	if err := os.WriteFile(metricFile, []byte(`[[metric]]
context = "oci_discovery_probe"
metricsdesc = { value = "OCI discovery integration probe." }
request = "select 1 as value from dual"
`), 0600); err != nil {
		t.Fatal(err)
	}
	zero := time.Duration(0)
	m.Metrics.Default, m.Metrics.Custom, m.Metrics.ScrapeInterval = metricFile, nil, &zero
	databases := db.NewDatabasesContext(ctx, logger, m)
	defer func() {
		if err := db.CloseDatabases(databases); err != nil {
			t.Error(err)
		}
	}()
	exporter := NewExporter(logger, m, databases)
	exporter.InitializeDatabasesContext(ctx)
	registry := prometheus.NewRegistry()
	if err := registry.Register(exporter); err != nil {
		t.Fatal(err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, family := range families {
		if family.GetName() != "oracledb_oci_discovery_probe_value" {
			continue
		}
		for _, metric := range family.Metric {
			if metric.GetGauge().GetValue() != 1 {
				t.Fatal("discovered database probe did not return 1")
			}
			found++
		}
	}
	if found != len(databases) {
		t.Fatalf("expected a successful probe for every discovered database: got %d of %d", found, len(databases))
	}
}
