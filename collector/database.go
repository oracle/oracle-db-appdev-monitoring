// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
	"strings"
	"time"
)

const (
	ora01017code = 1017
	ora28000code = 28000
)

func (d *Database) UpMetric(exporterLabels map[string]string) prometheus.Metric {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Whether the Oracle database server is up.",
		nil,
		d.constLabels(exporterLabels),
	)
	return prometheus.MustNewConstMetric(desc,
		prometheus.GaugeValue,
		d.Up,
	)
}

func (d *Database) DBTypeMetric(exporterLabels map[string]string) prometheus.Metric {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "dbtype"),
		"Type of database the exporter is connected to (0=non-CDB, 1=CDB, >1=PDB).",
		nil,
		d.constLabels(exporterLabels),
	)
	return prometheus.MustNewConstMetric(desc,
		prometheus.GaugeValue,
		d.Type,
	)
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
			db, dbtype := connect(logger, d.Name, d.Config)
			d.Session = db
			d.Type = dbtype
		}
		return err
	}
	// if connected but database type is unknown, try to reload it
	if d.Type == -1 {
		d.Type = getDBtype(ctx, d.Session, logger, d.Name)
	}
	d.Up = 1
	return nil
}

func (d *Database) constLabels(labels map[string]string) map[string]string {
	labels["database"] = d.Name

	// configured per-database labels added to constLabels
	for label, value := range d.Config.Labels {
		labels[label] = value
	}
	return labels
}

func NewDatabase(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *Database {
	db, dbtype := connect(logger, dbname, dbconfig)
	return &Database{
		Name:    dbname,
		Up:      0,
		Session: db,
		Type:    dbtype,
		Config:  dbconfig,
		Valid:   true,
	}
}

// initCache resets the metrics cached. Used on startup and when metrics are reloaded.
func (d *Database) initCache(metrics map[string]*Metric) {
	d.MetricsCache = NewMetricsCache(metrics)
}

// WarmupConnectionPool serially acquires connections to "warm up" the connection pool.
// This is a workaround for a perceived bug in ODPI_C where rapid acquisition of connections
// results in a SIGABRT.
func (d *Database) WarmupConnectionPool(logger *slog.Logger) {
	var connections []*sql.Conn
	poolSize := d.Config.GetMaxOpenConns()
	if poolSize < 1 {
		poolSize = d.Config.GetPoolMaxConnections()
	}
	if poolSize > 100 { // defensively cap poolsize
		poolSize = 100
	}
	warmup := func(i int) {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := d.Session.Conn(ctx)
		if err != nil {
			logger.Debug("Failed to open database connection on warmup", "conn", i, "error", err, "database", d.Name)
			return
		}
		connections = append(connections, conn)
	}
	for i := 0; i < poolSize; i++ {
		warmup(i + 1)
	}

	logger.Debug("Warmed connection pool", "total", len(connections), "database", d.Name)
	for i, conn := range connections {
		if err := conn.Close(); err != nil {
			logger.Debug("Failed to return database connection to pool on warmup", "conn", i+1, "error", err, "database", d.Name)
		}
	}
}

func (d *Database) IsValid() bool {
	return d.Valid
}

func (d *Database) invalidate() {
	d.Valid = false
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

	dbtype := getDBtype(ctx, db, logger, dbname)
	return db, dbtype
}

func getDBtype(ctx context.Context, db *sql.DB, logger *slog.Logger, dbname string) float64 {
	var dbtype int
	if err := db.QueryRowContext(ctx, "select sys_context('USERENV', 'CON_ID') from dual").Scan(&dbtype); err != nil {
		logger.Info("dbtype err", "error", err, "database", dbname)
		return -1
	}
	return float64(dbtype)
}
