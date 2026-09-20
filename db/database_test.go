// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/v2/config"
)

func TestIsTemporaryConnectionErrorCode(t *testing.T) {
	tests := []struct {
		name string
		code int
		want bool
	}{
		{name: "database starting", code: ora01033code, want: true},
		{name: "end of file on communication channel", code: ora03113code, want: true},
		{name: "not connected to Oracle", code: ora03114code, want: true},
		{name: "connection closed", code: ora12537code, want: true},
		{name: "no listener", code: ora12541code, want: true},
		{name: "invalid credentials", code: ora01017code, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTemporaryConnectionErrorCode(tt.code); got != tt.want {
				t.Errorf("isTemporaryConnectionErrorCode(%d) = %t, want %t", tt.code, got, tt.want)
			}
		})
	}
}

type testQueryDriver struct{}

type testQueryConn struct {
	rows driver.Rows
}

type testQueryRows struct {
	read bool
}

type testQueryConnector struct {
	rows driver.Rows
}

var testQueryDriverID atomic.Uint64
var errWarmupConnectionFailed = errors.New("warmup connection failed")

func (testQueryDriver) Open(name string) (driver.Conn, error) {
	return testQueryConn{rows: &testQueryRows{}}, nil
}

func (c testQueryConnector) Connect(context.Context) (driver.Conn, error) {
	rows := c.rows
	if rows == nil {
		rows = &testQueryRows{}
	}
	return testQueryConn{rows: rows}, nil
}

func (testQueryConnector) Driver() driver.Driver {
	return testQueryDriver{}
}

func (testQueryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (testQueryConn) Close() error {
	return nil
}

func (testQueryConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (c testQueryConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return c.rows, nil
}

func (r *testQueryRows) Columns() []string {
	return []string{"value"}
}

func (r *testQueryRows) Close() error {
	return nil
}

func (r *testQueryRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = "1"
	return nil
}

func openTestQueryDB(t *testing.T) *sql.DB {
	return openTestQueryDBWithRows(t, nil)
}

func openTestQueryDBWithRows(t *testing.T, rows driver.Rows) *sql.DB {
	t.Helper()

	name := "collector-test-query-" + strconv.FormatUint(testQueryDriverID.Add(1), 10)
	sql.Register(name, testQueryDriver{})

	db := sql.OpenDB(testQueryConnector{rows: rows})
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

type partialWarmupFailureConnector struct {
	successfulConnects int64
	connectAttempts    atomic.Int64
}

type partialWarmupFailureConn struct{}

func (c *partialWarmupFailureConnector) Connect(context.Context) (driver.Conn, error) {
	if c.connectAttempts.Add(1) > c.successfulConnects {
		return nil, errWarmupConnectionFailed
	}
	return partialWarmupFailureConn{}, nil
}

func (c *partialWarmupFailureConnector) Driver() driver.Driver {
	return testQueryDriver{}
}

func (partialWarmupFailureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (partialWarmupFailureConn) Close() error {
	return nil
}

func (partialWarmupFailureConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (partialWarmupFailureConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func (partialWarmupFailureConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &testQueryRows{}, nil
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		name         string
		invalidUntil *time.Time
		wantNil      bool
	}{
		{
			name:         "Nil invalidUntil",
			invalidUntil: nil,
			wantNil:      true,
		},
		{
			name:         "Future invalidUntil",
			invalidUntil: func() *time.Time { t := time.Now().Add(time.Minute); return &t }(),
			wantNil:      false,
		},
		{
			name:         "Past invalidUntil",
			invalidUntil: func() *time.Time { t := time.Now().Add(-time.Minute); return &t }(),
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &Database{invalidUntil: tt.invalidUntil}
			result := db.IsValid()
			if tt.wantNil {
				if result != nil {
					t.Fatalf("expected nil retryAfter, got %v", *result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil retryAfter")
			}
			if *result <= 0 {
				t.Fatalf("expected positive retryAfter, got %v", *result)
			}
		})
	}
}

func TestInvalidate(t *testing.T) {
	db := &Database{}
	backoff := time.Minute
	db.invalidate(backoff)
	if db.invalidUntil == nil {
		t.Fatal("Expected non-nil invalidUntil")
	}
	if time.Now().After(*db.invalidUntil) {
		t.Error("Expected invalidUntil in the future")
	}
}

func TestClearInvalid(t *testing.T) {
	db := &Database{}
	db.invalidate(time.Minute)
	db.clearInvalid()
	if db.invalidUntil != nil {
		t.Fatal("Expected invalidUntil to be cleared")
	}
}

func TestIsClosedDatabaseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "sql err conn done",
			err:  sql.ErrConnDone,
			want: true,
		},
		{
			name: "closed database text",
			err:  errors.New("sql: database is closed"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("other"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClosedDatabaseError(tt.err); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestWarmupConnectionPoolWithNilSessionSetsStartupReadyAndBackoff(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &Database{}

	err := db.WarmupConnectionPool(logger, time.Minute)
	if err == nil {
		t.Fatal("expected warmup to fail for nil session")
	}
	if !db.StartupReady() {
		t.Fatal("expected startupReady to be true after warmup attempt")
	}
	if db.IsValid() == nil {
		t.Fatal("expected invalidUntil to be set after warmup failure")
	}
	if got := db.getUp(); got != 0 {
		t.Fatalf("expected database up metric to remain 0, got %v", got)
	}
}

func TestWarmupSessionClosesAcquiredConnectionsAfterPartialFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	connector := &partialWarmupFailureConnector{successfulConnects: 2}
	session := sql.OpenDB(connector)
	t.Cleanup(func() {
		_ = session.Close()
	})
	maxOpenConns := 3
	db := &Database{
		Name:          "db1",
		Config:        config.DatabaseConfig{ConnectConfig: config.ConnectConfig{MaxOpenConns: &maxOpenConns}},
		DatabaseLabel: "database",
	}

	err := db.warmupSession(logger, session)

	if !errors.Is(err, errWarmupConnectionFailed) {
		t.Fatalf("expected warmup connection failure, got %v", err)
	}
	if got := connector.connectAttempts.Load(); got != 3 {
		t.Fatalf("expected initdb plus partial warmup to make 3 connection attempts, got %d", got)
	}
	if got := session.Stats().InUse; got != 0 {
		t.Fatalf("expected acquired warmup connections to be returned to the pool, got %d in use", got)
	}
}

func TestDatabaseStateAccessIsRaceSafe(t *testing.T) {
	db := &Database{
		DatabaseLabel: "database",
	}
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if (i+j)%2 == 0 {
					db.invalidate(time.Millisecond)
				} else {
					db.clearInvalid()
				}
				db.setUp(float64((i + j) % 2))
				_ = db.IsValid()
				_ = db.Up()
			}
		}(i)
	}

	wg.Wait()
}

func TestQueryContextHoldsReadLockUntilUnlock(t *testing.T) {
	db := &Database{
		Session: openTestQueryDB(t),
	}

	rows, unlock, err := db.QueryContext(context.Background(), "select 1 from dual")
	if err != nil {
		t.Fatalf("expected query to succeed, got %v", err)
	}

	locked := make(chan struct{})
	go func() {
		db.reconnectMU.Lock()
		close(locked)
		db.reconnectMU.Unlock()
	}()

	select {
	case <-locked:
		t.Fatal("expected reconnect write lock to wait for active query reader")
	case <-time.After(100 * time.Millisecond):
	}

	if err := rows.Close(); err != nil {
		t.Fatalf("expected rows close to succeed, got %v", err)
	}
	unlock()

	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("expected reconnect write lock to proceed after query reader released lock")
	}
}
