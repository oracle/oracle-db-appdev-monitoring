// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora

package collector

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/sijms/go-ora/v2"
	"github.com/sijms/go-ora/v2/network"
	"log/slog"
)

func connect(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *sql.DB {
	logger.Debug("Launching connection to "+maskDsn(dbconfig.URL), "database", dbname)

	password := dbconfig.GetPassword()
	username := dbconfig.GetUsername()
	dbconfig.ExternalAuth = password == ""

	logger.Debug(fmt.Sprintf("external authentication set to %t", dbconfig.ExternalAuth), "database", dbname)

	msg := "Using Username/Password Authentication."
	if dbconfig.ExternalAuth {
		msg = "Database Password not specified; will attempt to use external authentication (ignoring user input)."
		dbconfig.Username = ""
	}
	logger.Info(msg, "database", dbname)

	// Build connection string for go-ora
	var dsn string
	if dbconfig.ExternalAuth {
		// go-ora doesn't directly support external authentication
		// So we rely on OS authentication (set Oracle wallet/env)
		dsn = fmt.Sprintf("oracle://@%s", dbconfig.URL)
	} else if username != "" {
		dsn = fmt.Sprintf("oracle://%s:%s@%s", username, password, dbconfig.URL)
	} else {
		dsn = fmt.Sprintf("oracle://%s", dbconfig.URL)
	}

	// open connection (lazy until first use)
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		logger.Error("Failed to create DB handle", "error", err, "database", dbname)
		return nil
	}

	// Configure connection pool (sql.DB handles pooling)
	setConnectionPool(logger, dbname, dbconfig, db)
	initdb(logger, dbname, dbconfig, db)
	return db
}

func setConnectionPool(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {
	if dbconfig.GetPoolMaxConnections() > 0 {
		logger.Debug(fmt.Sprintf("set pool max connections to %d", dbconfig.PoolMaxConnections), "database", dbname)
		db.SetMaxOpenConns(dbconfig.GetPoolMaxConnections())
	} else {
		db.SetMaxOpenConns(dbconfig.GetMaxOpenConns())
	}
	if dbconfig.GetPoolMinConnections() > 0 {
		logger.Debug(fmt.Sprintf("set pool min connections to %d", dbconfig.PoolMinConnections), "database", dbname)
		db.SetMaxIdleConns(dbconfig.GetPoolMinConnections())
	} else {
		db.SetMaxIdleConns(dbconfig.GetMaxIdleConns())
	}
}

func isInvalidCredentialsError(err error) bool {
	if err == nil {
		return false
	}
	var oraErr *network.OracleError
	ok := errors.As(err, &oraErr)
	if !ok {
		return false
	}
	return oraErr.ErrCode == ora01017code || oraErr.ErrCode == ora28000code
}
