// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

// ErrConnectionNotReady means acquisition can be retried at the next poll.
var ErrConnectionNotReady = errors.New("database connection is not ready")

var (
	errDatabaseSessionNotInitialized = errors.New("database session is not initialized")
	errDatabaseClosed                = errors.New("database is shut down")
)

// Database owns an Oracle SQL session and its connection lifecycle.
type Database struct {
	Name          string
	Session       *sql.DB
	Config        config.DatabaseConfig
	DatabaseLabel string
	ClientInfo    config.ClientInfoConfig

	ctx        context.Context
	closed     bool
	connectErr error
	up         float64

	invalidUntil       *time.Time
	startupReady       atomic.Bool
	reconnectMU        sync.RWMutex
	reconnectAttemptMU sync.Mutex
	warmupLogger       *slog.Logger
	connectionBackoff  time.Duration
}

func NewDatabase(logger *slog.Logger, databaseLabel, name string, databaseConfig config.DatabaseConfig, clientInfo config.ClientInfoConfig) *Database {
	return NewDatabaseContext(context.Background(), logger, databaseLabel, name, databaseConfig, clientInfo)
}

func NewDatabaseContext(ctx context.Context, logger *slog.Logger, databaseLabel, name string, databaseConfig config.DatabaseConfig, clientInfo config.ClientInfoConfig) *Database {
	db, err := connectContext(ctx, logger, name, databaseConfig, clientInfo)
	if err != nil {
		logger.Error("Failed to initialize database session", "error", err, "database", name)
	}
	return &Database{
		ctx:           ctx,
		Name:          name,
		Session:       db,
		Config:        databaseConfig,
		connectErr:    err,
		DatabaseLabel: databaseLabel,
		ClientInfo:    clientInfo,
	}
}

// Close permanently shuts down this database handle. Native pools may be shared
// by multiple handles and are closed separately by CloseDatabases.
func (d *Database) Close() error {
	d.reconnectAttemptMU.Lock()
	defer d.reconnectAttemptMU.Unlock()
	d.reconnectMU.Lock()
	defer d.reconnectMU.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.startupReady.Store(false)
	d.up = 0
	d.connectErr = errDatabaseClosed
	session := d.Session
	d.Session = nil
	if session != nil {
		return session.Close()
	}
	return nil
}

func (d *Database) operationContext() context.Context {
	if d.ctx != nil {
		return d.ctx
	}
	return context.Background()
}

func (d *Database) StartupReady() bool {
	return d.startupReady.Load()
}

// WarmupConnectionPool serially acquires connections to "warm up" the connection pool.
func (d *Database) WarmupConnectionPool(logger *slog.Logger, backoff time.Duration) error {
	d.reconnectAttemptMU.Lock()
	defer d.reconnectAttemptMU.Unlock()
	if d.closed {
		return errDatabaseClosed
	}
	d.warmupLogger = logger
	d.connectionBackoff = backoff
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

	if err := d.warmupSession(d.operationContext(), logger, session); err != nil {
		d.setUp(0)
		d.invalidate(backoff)
		return err
	}

	d.setUp(1)
	d.clearInvalid()
	return nil
}

func (d *Database) warmupSession(ctx context.Context, logger *slog.Logger, session *sql.DB) error {
	if session == nil {
		return errDatabaseSessionNotInitialized
	}

	var connections []*sql.Conn
	defer func() {
		for i, conn := range connections {
			if err := conn.Close(); err != nil {
				logger.Debug("Failed to return database connection to pool on warmup", "conn", i+1, "error", err, "database", d.Name)
			}
		}
	}()

	poolSize := warmupConnectionPoolSize(d.Config)
	if poolSize > 100 { // defensively cap poolsize
		poolSize = 100
	}
	warmup := func() error {
		if warmupConnectionPause > 0 {
			pause := time.NewTimer(warmupConnectionPause)
			defer pause.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pause.C:
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		conn, err := session.Conn(ctx)
		if err != nil {
			return err
		}
		connections = append(connections, conn)
		return nil
	}

	initdb(ctx, logger, d.Name, d.Config, session)

	for i := 0; i < poolSize; i++ {
		// short circuit warmup for inaccessible databases
		if err := warmup(); err != nil {
			logger.Debug("Failed warmup database connection pool", "conn", i, "error", err, "database", d.Name)
			return err
		}
	}

	logger.Debug("Warmed connection pool", "total", len(connections), "database", d.Name)
	return ctx.Err()
}

func (d *Database) reconnect(logger *slog.Logger, backoff time.Duration, failedSession *sql.DB) error {
	d.reconnectAttemptMU.Lock()
	defer d.reconnectAttemptMU.Unlock()
	return d.reconnectLocked(d.operationContext(), logger, backoff, failedSession)
}

func (d *Database) reconnectLocked(ctx context.Context, logger *slog.Logger, backoff time.Duration, failedSession *sql.DB) error {
	if d.closed {
		return errDatabaseClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d.reconnectMU.RLock()
	current := d.Session
	d.reconnectMU.RUnlock()
	if current != failedSession {
		return nil // Another caller already replaced the failed handle.
	}
	if retryAfter := d.IsValid(); retryAfter != nil {
		return fmt.Errorf("database reconnect delayed for %s: %w", retryAfter.Round(time.Millisecond), ErrConnectionNotReady)
	}
	logger.Info("Reconnecting database session", "database", d.Name)

	session, err := connectContext(ctx, logger, d.Name, d.Config, d.ClientInfo)
	if err == nil {
		err = d.warmupSession(ctx, logger, session)
		if err != nil && session != nil {
			_ = session.Close()
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		d.reconnectMU.Lock()
		d.connectErr = err
		d.up = 0
		d.invalidateLocked(backoff)
		d.reconnectMU.Unlock()
		return err
	}

	d.reconnectMU.Lock()
	oldSession := d.Session
	d.Session = session
	d.connectErr = nil
	d.up = 1
	d.clearInvalidLocked()
	if oldSession != nil && oldSession != session {
		_ = oldSession.Close()
	}
	d.reconnectMU.Unlock()
	return nil
}

// Ping the database. If the database is disconnected, try to reconnect.
// If the database type is unknown, try to reload it.
func (d *Database) Ping(logger *slog.Logger, backoff time.Duration) error {
	ctx, cancel := context.WithTimeout(d.operationContext(), 5*time.Second)
	defer cancel()

	session, err := d.pingSession(ctx)
	if errors.Is(err, errDatabaseClosed) {
		return err
	}
	if errors.Is(err, errDatabaseSessionNotInitialized) {
		return d.reconnect(logger, backoff, session)
	}
	if err != nil {
		if isInvalidCredentialsError(err) && d.Config.Vault != nil && d.Config.Vault.OCI != nil {
			// Retire the connector holding old credentials. The next attempt, after
			// backoff, fetches the current Vault version while rebuilding the SQL handle.
			d.retireCredentials(session, backoff)
			return err
		}
		d.setUp(0)
		if isInvalidCredentialsError(err) || isTemporaryConnectionError(err) {
			d.invalidate(backoff)
			return err
		}
		// If database is closed, rebuild the handle and rerun init/warmup.
		if isClosedDatabaseError(err) {
			return d.reconnect(logger, backoff, session)
		}
		return err
	}
	// A concurrent authentication failure may already have retired this pool.
	d.reconnectMU.Lock()
	if !d.closed && d.Session == session {
		d.up = 1
		d.clearInvalidLocked()
	}
	d.reconnectMU.Unlock()
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
	return d.up
}

func (d *Database) setUp(up float64) {
	d.reconnectMU.Lock()
	defer d.reconnectMU.Unlock()
	d.up = up
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

// EnsureSession recovers an absent pool after startup and connection backoff.
// Call this with the worker context before starting a query timeout.
func (d *Database) EnsureSession(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.StartupReady() {
		return ErrConnectionNotReady
	}
	d.reconnectMU.RLock()
	available := d.Session != nil
	d.reconnectMU.RUnlock()
	if available {
		return nil
	}
	// Another caller may already be recovering this database. Let scheduled
	// log polls retry on their normal cadence without waiting for that caller.
	if !d.reconnectAttemptMU.TryLock() {
		return ErrConnectionNotReady
	}
	defer d.reconnectAttemptMU.Unlock()
	return d.reconnectLocked(ctx, d.warmupLogger, d.connectionBackoff, nil)
}

// ConnectionContext pins a session for the caller. Release must run before
// delivering output so log delivery does not hold a database connection.
func (d *Database) ConnectionContext(ctx context.Context) (*sql.Conn, func() error, error) {
	d.reconnectMU.RLock()
	if d.Session == nil {
		d.reconnectMU.RUnlock()
		return nil, nil, errDatabaseSessionNotInitialized
	}
	conn, err := d.Session.Conn(ctx)
	if err != nil {
		d.reconnectMU.RUnlock()
		return nil, nil, err
	}
	return conn, func() error {
		defer d.reconnectMU.RUnlock()
		return conn.Close()
	}, nil
}

func (d *Database) PingContext(ctx context.Context) error {
	_, err := d.pingSession(ctx)
	return err
}

func (d *Database) pingSession(ctx context.Context) (*sql.DB, error) {
	d.reconnectMU.RLock()
	defer d.reconnectMU.RUnlock()
	if d.closed {
		return nil, errDatabaseClosed
	}
	if d.Session == nil {
		return nil, errDatabaseSessionNotInitialized
	}
	return d.Session, d.Session.PingContext(ctx)
}

func (d *Database) retireCredentials(failedSession *sql.DB, backoff time.Duration) {
	d.reconnectMU.Lock()
	if d.closed || d.Session != failedSession {
		d.reconnectMU.Unlock()
		return
	}
	d.Session = nil
	d.up = 0
	d.invalidateLocked(backoff)
	d.reconnectMU.Unlock()
	if failedSession != nil {
		_ = failedSession.Close()
	}
}

// Up reports whether the most recent connection check succeeded.
func (d *Database) Up() float64 {
	return d.getUp()
}

func isClosedDatabaseError(err error) bool {
	return errors.Is(err, sql.ErrConnDone) || strings.Contains(err.Error(), "sql: database is closed")
}

func initdb(ctx context.Context, logger *slog.Logger, dbname string, dbconfig config.DatabaseConfig, db *sql.DB) {
	configureSQLConnectionPool(logger, dbname, dbconfig, db)
	logger.Debug(fmt.Sprintf("set connection max lifetime to %s", dbconfig.GetConnMaxLifetime()), "database", dbname)
	db.SetConnMaxLifetime(dbconfig.GetConnMaxLifetime())
	logger.Debug(fmt.Sprintf("Successfully configured connection to %s", MaskDSN(dbconfig.URL)), "database", dbname)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var sysdba string
	if err := db.QueryRowContext(ctx, "select sys_context('USERENV', 'ISDBA') from dual").Scan(&sysdba); err != nil {
		logger.Error("error checking my database role", "error", err, "database", dbname)
	}
	logger.Info("Connected as SYSDBA? "+sysdba, "database", dbname)
}

func configureSQLConnectionPool(logger *slog.Logger, dbname string, dbconfig config.DatabaseConfig, db *sql.DB) {
	maxOpenConns, maxIdleConns := effectiveSQLPoolLimits(dbconfig)
	logger.Debug(fmt.Sprintf("set max idle connections to %d", maxIdleConns), "database", dbname)
	db.SetMaxIdleConns(maxIdleConns)
	logger.Debug(fmt.Sprintf("set max open connections to %d", maxOpenConns), "database", dbname)
	db.SetMaxOpenConns(maxOpenConns)
}

// MaskDSN removes credentials from a connection string before it is logged.
func MaskDSN(dsn string) string {
	parts := strings.Split(dsn, "@")
	if len(parts) > 1 {
		return "***@" + parts[1]
	}
	return dsn
}
