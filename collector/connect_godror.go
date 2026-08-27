// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora

package collector

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
)

func connect(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) (*sql.DB, error) {
	logger.Debug("Launching connection to "+maskDsn(dbconfig.URL), "database", dbname)

	P, err := connectionParams(logger, dbname, dbconfig)
	if err != nil {
		return nil, err
	}
	// note that this just configures the connection, it does not actually connect until later
	// when we call db.Ping()
	db := sql.OpenDB(godror.NewConnector(P))
	return db, nil
}

// connectionParams translates the exporter database config into godror connection parameters.
func connectionParams(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) (godror.ConnectionParams, error) {
	var P godror.ConnectionParams
	password, err := dbconfig.GetPassword()
	if err != nil {
		return P, err
	}
	username, err := dbconfig.GetUsername()
	if err != nil {
		return P, err
	}
	// If password is not specified, externalAuth will be true, and we'll ignore user input
	dbconfig.ExternalAuth = password == ""
	logger.Debug(fmt.Sprintf("external authentication set to %t", dbconfig.ExternalAuth), "database", dbname)
	msg := "Using Username/Password Authentication."
	if dbconfig.ExternalAuth {
		msg = "Database Password not specified; will attempt to use external authentication (ignoring user input)."
		dbconfig.Username = ""
		username = "" // the local copy was fetched before this branch; clear it too
	}
	logger.Info(msg, "database", dbname)
	externalAuth := sql.NullBool{
		Bool:  dbconfig.ExternalAuth,
		Valid: true,
	}
	P.Username, P.Password, P.ConnectString, P.ExternalAuth = username, godror.NewPassword(password), dbconfig.URL, externalAuth

	if dbconfig.GetPoolIncrement() > 0 {
		logger.Debug(fmt.Sprintf("set pool increment to %d", dbconfig.PoolIncrement), "database", dbname)
		P.PoolParams.SessionIncrement = dbconfig.GetPoolIncrement()
	}
	if dbconfig.GetPoolMaxConnections() > 0 {
		logger.Debug(fmt.Sprintf("set pool max connections to %d", dbconfig.PoolMaxConnections), "database", dbname)
		P.PoolParams.MaxSessions = dbconfig.GetPoolMaxConnections()
	}
	if dbconfig.GetPoolMinConnections() > 0 {
		logger.Debug(fmt.Sprintf("set pool min connections to %d", dbconfig.PoolMinConnections), "database", dbname)
		P.PoolParams.MinSessions = dbconfig.GetPoolMinConnections()
	}

	P.PoolParams.WaitTimeout = time.Second * 5

	// godror defaults to standalone connections and then ignores PoolParams; opt into the
	// ODPI-C session pool whenever any pool* key is present in the config, including
	// explicit zero values. (Administrative roles such as SYSDBA are always standalone
	// in godror.)
	if dbconfig.PoolIncrement != nil || dbconfig.PoolMaxConnections != nil || dbconfig.PoolMinConnections != nil {
		P.StandaloneConnection = sql.NullBool{Bool: false, Valid: true}
	}

	// if TNS_ADMIN env var is set, set ConfigDir to that location
	P.ConfigDir = dbconfig.TNSAdmin

	switch dbconfig.Role {
	case "SYSDBA":
		P.AdminRole = dsn.SysDBA
	case "SYSOPER":
		P.AdminRole = dsn.SysOPER
	case "SYSBACKUP":
		P.AdminRole = dsn.SysBACKUP
	case "SYSDG":
		P.AdminRole = dsn.SysDG
	case "SYSKM":
		P.AdminRole = dsn.SysKM
	case "SYSRAC":
		P.AdminRole = dsn.SysRAC
	case "SYSASM":
		P.AdminRole = dsn.SysASM
	default:
		P.AdminRole = dsn.NoRole
	}

	return P, nil
}

func effectiveSQLPoolLimits(dbconfig DatabaseConfig) (int, int) {
	return dbconfig.GetMaxOpenConns(), dbconfig.GetMaxIdleConns()
}

func warmupConnectionPoolSize(dbconfig DatabaseConfig) int {
	poolSize := dbconfig.GetMaxOpenConns()
	if poolSize < 1 {
		poolSize = dbconfig.GetPoolMaxConnections()
	}
	// Warmup holds every acquired connection until the loop ends; the native pool
	// cannot hand out more than MaxSessions, so cap to avoid WaitTimeout errors.
	if poolMax := dbconfig.GetPoolMaxConnections(); poolMax > 0 && poolSize > poolMax {
		poolSize = poolMax
	}
	return poolSize
}

func isInvalidCredentialsError(err error) bool {
	err = errors.Unwrap(err)
	if err == nil {
		return false
	}
	oraErr, ok := err.(*godror.OraErr)
	if !ok {
		return false
	}
	return oraErr.Code() == ora01017code || oraErr.Code() == ora28000code
}

func isTemporaryConnectionError(err error) bool {
	err = errors.Unwrap(err)
	if err == nil {
		return false
	}
	oraErr, ok := err.(*godror.OraErr)
	if !ok {
		return false
	}
	switch oraErr.Code() {
	case ora01033code, ora03113code, ora03114code, ora12537code:
		return true
	default:
		return false
	}
}
