// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

const (
	tagKeyExporterEnabled = "oracledb-metrics-exporter-enabled"
)

func discoverDatabases(databasesFrom *DatabasesFromConfig) ([]DatabaseConfig, error) {
	var databases []DatabaseConfig
	if databasesFrom == nil { // no databases to discover
		return nil, nil
	}

	if databasesFrom.OCI != nil {
		ociDatabases, err := discoverOCIDatabases(databasesFrom.OCI)
		if err != nil {
			return nil, err
		}
		databases = append(databases, ociDatabases...)
	}

	return databases, nil
}

func discoverOCIDatabases(databasesFrom *OCIDatabasesFromConfig) ([]DatabaseConfig, error) {
	var databases []DatabaseConfig

	configProvider, err := oci.ConfigurationProviderForAuthMode(databasesFrom.Auth)
	if err != nil {
		return nil, err
	}
	adbClient, err := adb.NewDatabaseClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, err
	}

	return databases, nil
}
