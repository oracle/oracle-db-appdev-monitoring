// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"log/slog"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

const (
	tagKeyExporterEnabled = "oracledb-metrics-exporter-enabled"
)

func discoverDatabases(logger *slog.Logger, cfg *DatabasesFromConfig) ([]DatabaseConfig, error) {
	var databases []DatabaseConfig
	if cfg == nil { // no databases to discover
		return nil, nil
	}

	if cfg.OCI != nil {
		ociDatabases, err := discoverOCIDatabases(logger, cfg.OCI)
		if err != nil {
			return nil, err
		}
		databases = append(databases, ociDatabases...)
	}

	return databases, nil
}

func discoverOCIDatabases(logger *slog.Logger, cfg *OCIDatabasesFromConfig) ([]DatabaseConfig, error) {
	var databases []DatabaseConfig

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

	return databases, nil
}
