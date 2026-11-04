// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

// connectContext owns credential resolution and the lazy SQL handle. Each build
// selects a driver-specific newConnector for transport and session setup.
func connectContext(ctx context.Context, logger *slog.Logger, dbname string, dbconfig config.DatabaseConfig, clientInfo config.ClientInfoConfig) (*sql.DB, error) {
	logger.Debug("Launching connection to "+MaskDSN(dbconfig.URL), "database", dbname)
	credentials, err := dbconfig.ResolveCredentialsContext(ctx)
	if err != nil {
		return nil, err
	}
	connector, err := newConnector(dbconfig, credentials, clientInfo)
	if err != nil {
		return nil, err
	}
	logger.Debug("Configured database connection", "driver", driverName, "external authentication", credentials.Password == "", "database", dbname)
	msg := "Using Username/Password Authentication."
	if credentials.Password == "" {
		msg = "Database Password not specified; will attempt to use external authentication (ignoring user input)."
	}
	logger.Info(msg, "database", dbname)
	return sql.OpenDB(connector), nil
}

const (
	ora01017code = 1017
	ora01033code = 1033
	ora28000code = 28000
	ora03113code = 3113
	ora03114code = 3114
	ora12537code = 12537
	ora12541code = 12541
)

func isTemporaryConnectionErrorCode(code int) bool {
	switch code {
	case ora01033code, ora03113code, ora03114code, ora12537code, ora12541code:
		return true
	default:
		return false
	}
}

func isInvalidCredentialsError(err error) bool {
	code := oracleErrorCode(err)
	return code == ora01017code || code == ora28000code
}

func isTemporaryConnectionError(err error) bool {
	return isTemporaryConnectionErrorCode(oracleErrorCode(err))
}
