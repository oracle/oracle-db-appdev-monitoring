// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build godror

package collector

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"log/slog"
	"time"
)

func connect(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *sql.DB {
	logger.Debug("Launching connection to "+maskDsn(dbconfig.URL), "database", dbname)

	var P godror.ConnectionParams
	password := dbconfig.GetPassword()
	username := dbconfig.GetUsername()
	// If password is not specified, externalAuth will be true, and we'll ignore user input
	dbconfig.ExternalAuth = password == ""
	logger.Debug(fmt.Sprintf("external authentication set to %t", dbconfig.ExternalAuth), "database", dbname)
	msg := "Using Username/Password Authentication."
	if dbconfig.ExternalAuth {
		msg = "Database Password not specified; will attempt to use external authentication (ignoring user input)."
		dbconfig.Username = ""
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

	// note that this just configures the connection, it does not actually connect until later
	// when we call db.Ping()
	db := sql.OpenDB(godror.NewConnector(P))
	initdb(logger, dbname, dbconfig, db)
	return db
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
