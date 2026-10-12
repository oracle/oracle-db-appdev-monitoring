// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type shutdownDriver struct {
	connectionsClosed atomic.Int32
	poolsClosed       atomic.Int32
	wantConnections   int32
	closeErr          error
}

func (d *shutdownDriver) Open(string) (driver.Conn, error) { return &shutdownConn{driver: d}, nil }
func (d *shutdownDriver) Close() error {
	if got := d.connectionsClosed.Load(); got != d.wantConnections {
		return errors.New("native pools closed before SQL connections")
	}
	d.poolsClosed.Add(1)
	return d.closeErr
}

type shutdownConnector struct{ driver *shutdownDriver }

func (c shutdownConnector) Driver() driver.Driver                        { return c.driver }
func (c shutdownConnector) Connect(context.Context) (driver.Conn, error) { return c.driver.Open("") }

type shutdownConn struct {
	testQueryConn
	driver *shutdownDriver
}

func (c *shutdownConn) Close() error { c.driver.connectionsClosed.Add(1); return nil }

func TestCloseDatabasesClosesSharedNativePoolsOnce(t *testing.T) {
	driver := &shutdownDriver{wantConnections: 2}
	var databases []*Database
	for _, name := range []string{"first", "second"} {
		session := sql.OpenDB(shutdownConnector{driver: driver})
		t.Cleanup(func() { _ = session.Close() })
		if err := session.Ping(); err != nil {
			t.Fatal(err)
		}
		databases = append(databases, &Database{Name: name, Session: session})
	}
	if err := CloseDatabases(databases); err != nil {
		t.Fatal(err)
	}
	if got := driver.poolsClosed.Load(); got != 1 {
		t.Fatalf("pool closes = %d, want 1", got)
	}
	if err := CloseDatabases(databases); err != nil {
		t.Fatal(err)
	}
	if got := driver.poolsClosed.Load(); got != 1 {
		t.Fatalf("repeat shutdown closed native pools again: %d", got)
	}
	for _, database := range databases {
		if err := database.Ping(testLogger(), time.Second); !errors.Is(err, errDatabaseClosed) {
			t.Fatalf("Ping after shutdown: %v", err)
		}
		if err := database.reconnect(testLogger(), time.Second); !errors.Is(err, errDatabaseClosed) {
			t.Fatalf("reconnect after shutdown: %v", err)
		}
		if err := database.WarmupConnectionPool(testLogger(), time.Second); !errors.Is(err, errDatabaseClosed) {
			t.Fatalf("warmup after shutdown: %v", err)
		}
		if database.StartupReady() {
			t.Fatal("closed database is startup ready")
		}
		if _, _, err := database.Query("select 1"); !errors.Is(err, errDatabaseClosed) {
			t.Fatalf("query after shutdown: %v", err)
		}
	}
}

func TestCloseDatabasesReportsNativePoolCloseError(t *testing.T) {
	closeErr := errors.New("native cleanup failed")
	driver := &shutdownDriver{closeErr: closeErr}
	database := &Database{Session: sql.OpenDB(shutdownConnector{driver: driver})}
	if err := CloseDatabases([]*Database{database}); !errors.Is(err, closeErr) {
		t.Fatalf("close error = %v", err)
	}
}

func TestDatabaseCloseWaitsForActiveQuery(t *testing.T) {
	database := &Database{Session: openTestQueryDB(t)}
	rows, unlock, err := database.Query("select 1")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- database.Close() }()
	select {
	case err := <-done:
		t.Errorf("Close returned with active rows: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	_ = rows.Close()
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after query released its lifecycle lock")
	}
}
