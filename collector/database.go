// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	ora01017code = 1017
	ora01033code = 1033
	ora28000code = 28000
	ora03113code = 3113
	ora03114code = 3114
	ora12537code = 12537
)

var errDatabaseSessionNotInitialized = errors.New("database session is not initialized")

func (d *Database) UpMetric(exporterLabels map[string]string) prometheus.Metric {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Whether the Oracle AI Database server is up.",
		nil,
		d.constLabels(exporterLabels),
	)
	return prometheus.MustNewConstMetric(desc,
		prometheus.GaugeValue,
		d.getUp(),
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
	db, err := connect(logger, dbname, dbconfig)
	if err != nil {
		logger.Error("Failed to initialize database session", "error", err, "database", dbname)
	}
	return &Database{
		Name:          dbname,
		Up:            0,
		Session:       db,
		Config:        dbconfig,
		connectErr:    err,
		DatabaseLabel: dblabel,
		reconnectMU:   sync.RWMutex{},
	}
}

func (d *Database) StartupReady() bool {
	return d.startupReady.Load()
}

// initCache resets the metrics cached. Used on startup and when metrics are reloaded.
func (d *Database) initCache(metrics map[string]*Metric) {
	d.MetricsCache = NewMetricsCache(metrics)
}

// WarmupConnectionPool serially acquires connections to "warm up" the connection pool.
// This is a workaround for a perceived bug in ODPI_C where rapid acquisition of connections
// results in a SIGABRT.
func (d *Database) WarmupConnectionPool(logger *slog.Logger, backoff time.Duration) error {
	defer d.startupReady.Store(true)

	d.reconnectMU.RLock()
	session := d.Session
	connectErr := d.connectErr
	d.reconnectMU.RUnlock()

	if session == nil {
		d.setUp(0)
		d.invalidate(backoff)
		if connectErr != nil {
			return connectErr
		}
		return errDatabaseSessionNotInitialized
	}

	if err := d.warmupSession(logger, session); err != nil {
		d.setUp(0)
		d.invalidate(backoff)
		return err
	}

	d.setUp(1)
	d.clearInvalid()
	return nil
}

func (d *Database) warmupSession(logger *slog.Logger, session *sql.DB) error {
	if session == nil {
		return errDatabaseSessionNotInitialized
	}

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

		conn, err := session.Conn(ctx)
		if err != nil {
			return err
		}
		connections = append(connections, conn)
		return nil
	}

	initdb(logger, d.Name, d.Config, session)

	for i := 0; i < poolSize; i++ {
		// short circuit warmup for inaccessible databases
		if err := warmup(i + 1); err != nil {
			logger.Debug("Failed warmup database connection pool", "conn", i, "error", err, "database", d.Name)
			return err
		}
	}

	logger.Debug("Warmed connection pool", "total", len(connections), "database", d.Name)
	for i, conn := range connections {
		if err := conn.Close(); err != nil {
			logger.Debug("Failed to return database connection to pool on warmup", "conn", i+1, "error", err, "database", d.Name)
		}
	}
	return nil
}

func (d *Database) reconnect(logger *slog.Logger, backoff time.Duration) error {
	d.reconnectAttemptMU.Lock()
	defer d.reconnectAttemptMU.Unlock()

	logger.Info("Reconnecting database session", "database", d.Name)

	session, err := connect(logger, d.Name, d.Config)
	if err != nil {
		d.reconnectMU.Lock()
		d.connectErr = err
		d.Up = 0
		d.invalidateLocked(backoff)
		d.reconnectMU.Unlock()
		return err
	}
	if err := d.warmupSession(logger, session); err != nil {
		if session != nil {
			_ = session.Close()
		}
		d.reconnectMU.Lock()
		d.connectErr = err
		d.Up = 0
		d.invalidateLocked(backoff)
		d.reconnectMU.Unlock()
		return err
	}

	d.reconnectMU.Lock()
	oldSession := d.Session
	d.Session = session
	d.connectErr = nil
	d.Up = 1
	d.clearInvalidLocked()
	if oldSession != nil && oldSession != session {
		_ = oldSession.Close()
	}
	d.reconnectMU.Unlock()
	return nil
}

// ping the database. If the database is disconnected, try to reconnect.
// If the database type is unknown, try to reload it.
func (d *Database) ping(logger *slog.Logger, backoff time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := d.PingContext(ctx)
	if errors.Is(err, errDatabaseSessionNotInitialized) {
		return d.reconnect(logger, backoff)
	}
	if err != nil {
		d.setUp(0)
		if isInvalidCredentialsError(err) || isTemporaryConnectionError(err) {
			d.invalidate(backoff)
			return err
		}
		// If database is closed, rebuild the handle and rerun init/warmup.
		if isClosedDatabaseError(err) {
			return d.reconnect(logger, backoff)
		}
		return err
	}
	d.setUp(1)
	d.clearInvalid()
	return nil
}

func (d *Database) IsValid() *time.Duration {
	d.reconnectMU.RLock()
	defer d.reconnectMU.RUnlock()

	if d.invalidUntil == nil {
		return nil
	}
	retryAfter := time.Until(*d.invalidUntil)
	if retryAfter <= 0 {
		return nil
	}
	return &retryAfter
}

func (d *Database) invalidate(backoff time.Duration) {
	d.reconnectMU.Lock()
	defer d.reconnectMU.Unlock()
	d.invalidateLocked(backoff)
}

func (d *Database) invalidateLocked(backoff time.Duration) {
	until := time.Now().Add(backoff)
	d.invalidUntil = &until
}

func (d *Database) clearInvalid() {
	d.reconnectMU.Lock()
	defer d.reconnectMU.Unlock()
	d.clearInvalidLocked()
}

func (d *Database) clearInvalidLocked() {
	d.invalidUntil = nil
}

func (d *Database) getUp() float64 {
	d.reconnectMU.RLock()
	defer d.reconnectMU.RUnlock()
	return d.Up
}

func (d *Database) setUp(up float64) {
	d.reconnectMU.Lock()
	defer d.reconnectMU.Unlock()
	d.Up = up
}

func (d *Database) Query(query string, args ...interface{}) (*sql.Rows, func(), error) {
	d.reconnectMU.RLock()
	if d.Session == nil {
		err := d.connectErr
		d.reconnectMU.RUnlock()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errDatabaseSessionNotInitialized
	}

	rows, err := d.Session.Query(query, args...)
	if err != nil {
		d.reconnectMU.RUnlock()
		return nil, nil, err
	}
	return rows, d.reconnectMU.RUnlock, nil
}

func (d *Database) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, func(), error) {
	d.reconnectMU.RLock()
	if d.Session == nil {
		err := d.connectErr
		d.reconnectMU.RUnlock()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errDatabaseSessionNotInitialized
	}

	rows, err := d.Session.QueryContext(ctx, query, args...)
	if err != nil {
		d.reconnectMU.RUnlock()
		return nil, nil, err
	}
	return rows, d.reconnectMU.RUnlock, nil
}

func (d *Database) PingContext(ctx context.Context) error {
	d.reconnectMU.RLock()
	defer d.reconnectMU.RUnlock()
	if d.Session == nil {
		return errDatabaseSessionNotInitialized
	}
	return d.Session.PingContext(ctx)
}

func isClosedDatabaseError(err error) bool {
	return errors.Is(err, sql.ErrConnDone) || strings.Contains(err.Error(), "sql: database is closed")
}

func initdb(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {
	logger.Debug(fmt.Sprintf("set max idle connections to %d", dbconfig.MaxIdleConns), "database", dbname)
	db.SetMaxIdleConns(dbconfig.GetMaxIdleConns())
	logger.Debug(fmt.Sprintf("set max open connections to %d", dbconfig.MaxOpenConns), "database", dbname)
	db.SetMaxOpenConns(dbconfig.GetMaxOpenConns())
	logger.Debug(fmt.Sprintf("set connection max lifetime to %s", dbconfig.GetConnMaxLifetime()), "database", dbname)
	db.SetConnMaxLifetime(dbconfig.GetConnMaxLifetime())
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
