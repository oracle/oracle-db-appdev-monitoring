// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"database/sql"
	"fmt"
	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
	"strings"
	"time"
)

func (d *Database) UpMetric() prometheus.Metric {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Whether the Oracle database server is up.",
		nil,
		d.constLabels(),
	)
	return prometheus.MustNewConstMetric(desc,
		prometheus.GaugeValue,
		d.Up,
	)
}

func (d *Database) DBTypeMetric() prometheus.Metric {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "dbtype"),
		"Type of database the exporter is connected to (0=non-CDB, 1=CDB, >1=PDB).",
		nil,
		d.constLabels(),
	)
	return prometheus.MustNewConstMetric(desc,
		prometheus.GaugeValue,
		d.Type,
	)
}

func (d *Database) ping(logger *slog.Logger) error {
	err := d.Session.Ping()
	if err != nil {

		d.Up = 0
		if strings.Contains(err.Error(), "sql: database is closed") {
			db, dbtype := connect(logger, d.Name, d.Config)
			d.Session = db
			d.Type = dbtype
		}
	} else {
		d.Up = 1
	}
	return err
}

func (d *Database) constLabels() map[string]string {
	return map[string]string{
		"database": d.Name,
	}
}

func NewDatabase(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *Database {
	db, dbtype := connect(logger, dbname, dbconfig)

	return &Database{
		Name:    dbname,
		Up:      0,
		Session: db,
		Type:    dbtype,
		Config:  dbconfig,
	}
}

func connect(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) (*sql.DB, float64) {
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

	if _, err := db.Exec(`
			begin
	       		dbms_application_info.set_client_info('oracledb_exporter');
			end;`); err != nil {
		logger.Info("Could not set CLIENT_INFO.", "database", dbname)
	}

	var result int
	if err := db.QueryRow("select sys_context('USERENV', 'CON_ID') from dual").Scan(&result); err != nil {
		logger.Info("dbtype err", "error", err, "database", dbname)
	}

	var sysdba string
	if err := db.QueryRow("select sys_context('USERENV', 'ISDBA') from dual").Scan(&sysdba); err != nil {
		logger.Error("error checking my database role", "error", err, "database", dbname)
	}
	logger.Info("Connected as SYSDBA? "+sysdba, "database", dbname)

	return db, float64(result)
}
