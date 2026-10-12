// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
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
		databases = append(databases, NewDatabase(logger, databaseLabel, name, databaseConfig, m.Metrics.ClientInfo))
	}
	return databases
}

// CloseDatabases closes all SQL handles before destroying shared native pools.
// Call only after all database workers and collectors have stopped.
func CloseDatabases(databases []*Database) error {
	drivers := make(map[driver.Driver]io.Closer)
	for _, database := range databases {
		database.reconnectMU.RLock()
		if database.Session != nil {
			driver := database.Session.Driver()
			if closer, ok := driver.(io.Closer); ok {
				drivers[driver] = closer
			}
		}
		database.reconnectMU.RUnlock()
	}
	var errs []error
	for _, database := range databases {
		if err := database.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close database %s: %w", database.Name, err))
		}
	}
	for _, closer := range drivers {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close native database pools: %w", err))
		}
	}
	return errors.Join(errs...)
}
