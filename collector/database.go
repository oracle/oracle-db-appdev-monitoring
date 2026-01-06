// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql"
	"fmt"
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
		"Whether the Oracle AI Database server is up.",
		nil,
		d.constLabels(exporterLabels),
	)
	return prometheus.MustNewConstMetric(desc,
		prometheus.GaugeValue,
		d.Up,
	)
}

func (d *Database) constLabels(labels map[string]string) map[string]string {
	labels[d.DatabaseLabel] = d.Name

	// configured per-database labels added to constLabels
	for label, value := range d.Config.Labels {
		labels[label] = value
	}
	return labels
}

func NewDatabase(logger *slog.Logger, dblabel, dbname string, dbconfig DatabaseConfig) *Database {
	db := connect(logger, dbname, dbconfig)
	return &Database{
		Name:          dbname,
		Up:            0,
		Session:       db,
		Config:        dbconfig,
		DatabaseLabel: dblabel,
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
	warmup := func(i int) error {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := d.Session.Conn(ctx)
		if err != nil {
			return err
		}
		connections = append(connections, conn)
		return nil
	}

	func() {
		for i := 0; i < poolSize; i++ {
			// short circuit warmup for inaccessible databases
			if err := warmup(i + 1); err != nil {
				d.Up = 0
				logger.Error("Failed warmup database connection pool", "conn", i, "error", err, "database", d.Name)
				return
			}
		}
	}()

	logger.Debug("Warmed connection pool", "total", len(connections), "database", d.Name)
	for i, conn := range connections {
		if err := conn.Close(); err != nil {
			logger.Debug("Failed to return database connection to pool on warmup", "conn", i+1, "error", err, "database", d.Name)
		}
	}
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

func (d *Database) IsValid() bool {
	if d.invalidUntil == nil {
		return true
	}
	return time.Now().After(*d.invalidUntil)
}

func (d *Database) invalidate() {
	invalidDuration := 5 * time.Minute
	until := time.Now().Add(invalidDuration)
	d.invalidUntil = &until
}

func initdb(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {
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
}
