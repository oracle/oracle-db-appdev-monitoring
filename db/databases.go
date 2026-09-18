// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"log/slog"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

// NewDatabases creates one database instance for every configured database.
// Connection failures are retained by each Database for the normal warmup and
// reconnect paths; they do not prevent the remaining databases from starting.
func NewDatabases(logger *slog.Logger, m *config.MetricsConfiguration) []*Database {
	databases := make([]*Database, 0, len(m.Databases))
	databaseLabel := m.DatabaseLabel()
	for name, databaseConfig := range m.Databases {
		logger.Info("Registering database", "database", name)
		databases = append(databases, NewDatabase(logger, databaseLabel, name, databaseConfig))
	}
	return databases
}
