// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora && !oracledb

package db

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

const driverName = "godror"

func newConnector(dbconfig config.DatabaseConfig, credentials config.DatabaseCredentials, clientInfo config.ClientInfoConfig) (driver.Connector, error) {
	return godror.NewConnector(connectionParams(dbconfig, credentials.Username, credentials.Password, clientInfo)), nil
}

func connectionParams(dbconfig config.DatabaseConfig, username, password string, clientInfo config.ClientInfoConfig) godror.ConnectionParams {
	var P godror.ConnectionParams
	if clientInfo.IsEnabled() {
		P.OnInitStmts = []string{clientInfoSQL(clientInfo.GetLabel())}
	}
	externalAuth := password == ""
	if externalAuth {
		username = ""
	}
	P.Username, P.Password, P.ConnectString, P.ExternalAuth = username, godror.NewPassword(password), dbconfig.URL, sql.NullBool{
		Bool:  externalAuth,
		Valid: true,
	}

	if dbconfig.GetPoolIncrement() > 0 {
		P.PoolParams.SessionIncrement = dbconfig.GetPoolIncrement()
	}
	if dbconfig.GetPoolMaxConnections() > 0 {
		P.PoolParams.MaxSessions = dbconfig.GetPoolMaxConnections()
	}
	if dbconfig.GetPoolMinConnections() > 0 {
		P.PoolParams.MinSessions = dbconfig.GetPoolMinConnections()
	}
	// godror defaults to standalone connections, which ignore PoolParams. Any explicit
	// pool setting, including zero, opts into ODPI-C pooling. Administrative roles remain
	// standalone because godror's IsStandalone enforces that restriction.
	if dbconfig.PoolIncrement != nil || dbconfig.PoolMaxConnections != nil || dbconfig.PoolMinConnections != nil {
		P.StandaloneConnection = sql.NullBool{Bool: false, Valid: true}
	}
	P.PoolParams.WaitTimeout = time.Second * 5
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

	return P
}

func effectiveSQLPoolLimits(dbconfig config.DatabaseConfig) (int, int) {
	return dbconfig.GetMaxOpenConns(), dbconfig.GetMaxIdleConns()
}

func warmupConnectionPoolSize(dbconfig config.DatabaseConfig) int {
	poolSize := dbconfig.GetMaxOpenConns()
	if poolSize < 1 {
		poolSize = dbconfig.GetPoolMaxConnections()
	}
	if poolMax := dbconfig.GetPoolMaxConnections(); poolMax > 0 && poolSize > poolMax {
		poolSize = poolMax
	}
	return poolSize
}

func oracleErrorCode(err error) int {
	var oraErr *godror.OraErr
	if !errors.As(err, &oraErr) {
		return 0
	}
	return oraErr.Code()
}
