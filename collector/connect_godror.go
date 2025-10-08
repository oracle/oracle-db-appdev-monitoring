// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora

package collector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"log/slog"
	"strings"
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
	logger.Debug(fmt.Sprintf("set max idle connections to %d", dbconfig.MaxIdleConns), "database", dbname)
	db.SetMaxIdleConns(dbconfig.GetMaxIdleConns())
	logger.Debug(fmt.Sprintf("set max open connections to %d", dbconfig.MaxOpenConns), "database", dbname)
	db.SetMaxOpenConns(dbconfig.GetMaxOpenConns())
	db.SetConnMaxLifetime(0)
	logger.Debug(fmt.Sprintf("Successfully configured connection to %s", maskDsn(dbconfig.URL)), "database", dbname)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `
			begin
	       		dbms_application_info.set_client_info('oracledb_exporter');
			end;`); err != nil {
		logger.Info("Could not set CLIENT_INFO.", "database", dbname)
	}

	var sysdba string
	if err := db.QueryRowContext(ctx, "select sys_context('USERENV', 'ISDBA') from dual").Scan(&sysdba); err != nil {
		logger.Error("error checking my database role", "error", err, "database", dbname)
	}
	logger.Info("Connected as SYSDBA? "+sysdba, "database", dbname)

	return db
}

// ping the database. If the database is disconnected, try to reconnect.
// If the database type is unknown, try to reload it.
func (d *Database) ping(logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := d.Session.PingContext(ctx)
	if err != nil {
		d.Up = 0
		if isInvalidCredentialsError(err) {
			d.invalidate()
			return err
		}
		// If database is closed, try to reconnect
		if strings.Contains(err.Error(), "sql: database is closed") {
			d.Session = connect(logger, d.Name, d.Config)
		}
		return err
	}
	d.Up = 1
	return nil
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
