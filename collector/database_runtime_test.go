// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"database/sql"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/internal/testdb"
)

func TestDatabaseConstLabelsAndUpMetric(t *testing.T) {
	db := &Database{
		Name:          "db1",
		Up:            1,
		DatabaseLabel: "database",
		Config: DatabaseConfig{
			Labels: map[string]string{"env": "prod"},
		},
	}

	labels := db.constLabels(map[string]string{"cluster": "east"})
	if labels["database"] != "db1" || labels["env"] != "prod" || labels["cluster"] != "east" {
		t.Fatalf("unexpected const labels %v", labels)
	}

	metric := db.UpMetric(map[string]string{"cluster": "east"})
	if metric == nil {
		t.Fatal("expected up metric")
	}
}

func TestDatabaseInitCache(t *testing.T) {
	db := &Database{}
	metric := &Metric{ID: "sessions_value"}

	db.initCache(map[string]*Metric{metric.ID: metric})

	if db.MetricsCache == nil || db.MetricsCache.cache[metric] == nil {
		t.Fatal("expected metrics cache to be initialized")
	}
}

func TestNewDatabaseUsesConnectHelper(t *testing.T) {
	originalConnect := connectDB
	defer func() { connectDB = originalConnect }()

	dbh, _ := testdb.New(testdb.Scenario{})
	defer dbh.Close()

	connectDB = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *sql.DB {
		return dbh
	}

	db := NewDatabase(testLogger(), "database", "db1", DatabaseConfig{})
	if db.Session != dbh || db.Name != "db1" || db.DatabaseLabel != "database" {
		t.Fatalf("unexpected new database %#v", db)
	}
}

func TestWarmupSessionSuccessUsesConfiguredPoolSize(t *testing.T) {
	originalInit := initDatabaseSession
	initDatabaseSession = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {}
	t.Cleanup(func() { initDatabaseSession = originalInit })

	dbh, state := testdb.New(testdb.Scenario{})
	defer dbh.Close()

	db := &Database{
		Name:   "db1",
		Config: DatabaseConfig{ConnectConfig: ConnectConfig{MaxOpenConns: ptr(3)}},
	}

	if err := db.warmupSession(testLogger(), time.Minute, dbh); err != nil {
		t.Fatalf("warmupSession: %v", err)
	}
	if db.Up != 1 {
		t.Fatalf("expected db up metric 1, got %v", db.Up)
	}
	if db.invalidUntil != nil {
		t.Fatalf("expected invalidUntil cleared, got %v", db.invalidUntil)
	}
	if got := state.ConnectCalls(); got != 3 {
		t.Fatalf("expected 3 connection acquisitions, got %d", got)
	}
}

func TestWarmupSessionCapsPoolSizeAtHundred(t *testing.T) {
	originalInit := initDatabaseSession
	initDatabaseSession = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {}
	t.Cleanup(func() { initDatabaseSession = originalInit })

	dbh, state := testdb.New(testdb.Scenario{})
	defer dbh.Close()

	db := &Database{
		Name:   "db1",
		Config: DatabaseConfig{ConnectConfig: ConnectConfig{MaxOpenConns: ptr(150)}},
	}

	if err := db.warmupSession(testLogger(), time.Minute, dbh); err != nil {
		t.Fatalf("warmupSession: %v", err)
	}
	if got := state.ConnectCalls(); got != 100 {
		t.Fatalf("expected warmup to cap at 100 connections, got %d", got)
	}
}

func TestWarmupSessionConnectionFailureInvalidatesDatabase(t *testing.T) {
	originalInit := initDatabaseSession
	initDatabaseSession = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {}
	t.Cleanup(func() { initDatabaseSession = originalInit })

	dbh, _ := testdb.New(testdb.Scenario{
		ConnectErrors: []error{nil, errors.New("connect failed")},
	})
	defer dbh.Close()

	db := &Database{
		Name:   "db1",
		Config: DatabaseConfig{ConnectConfig: ConnectConfig{MaxOpenConns: ptr(2)}},
	}

	err := db.warmupSession(testLogger(), time.Minute, dbh)
	if err == nil {
		t.Fatal("expected warmup failure")
	}
	if db.Up != 0 {
		t.Fatalf("expected db up metric 0, got %v", db.Up)
	}
	if db.invalidUntil == nil {
		t.Fatal("expected invalidUntil to be set")
	}
}

func TestReconnectUsesInjectedConnectFunction(t *testing.T) {
	originalConnect := connectDB
	originalInit := initDatabaseSession
	initDatabaseSession = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {}
	t.Cleanup(func() {
		connectDB = originalConnect
		initDatabaseSession = originalInit
	})

	oldDB, _ := testdb.New(testdb.Scenario{})
	defer oldDB.Close()
	newDB, _ := testdb.New(testdb.Scenario{})
	defer newDB.Close()

	var calls atomic.Int32
	connectDB = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *sql.DB {
		calls.Add(1)
		return newDB
	}

	db := &Database{
		Name:    "db1",
		Session: oldDB,
		Config:  DatabaseConfig{ConnectConfig: ConnectConfig{MaxOpenConns: ptr(1)}},
	}

	if err := db.reconnect(testLogger(), time.Minute); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected connect helper to be called once, got %d", calls.Load())
	}
	if db.Session != newDB {
		t.Fatal("expected database session to be replaced")
	}
}

func TestPingSuccessClearsInvalidState(t *testing.T) {
	dbh, state := testdb.New(testdb.Scenario{})
	defer dbh.Close()

	db := &Database{
		Name:    "db1",
		Session: dbh,
		Config:  DatabaseConfig{ConnectConfig: ConnectConfig{QueryTimeout: ptr(5)}},
	}
	db.invalidate(time.Minute)

	if err := db.ping(testLogger(), time.Minute); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if db.Up != 1 {
		t.Fatalf("expected db up metric 1, got %v", db.Up)
	}
	if db.invalidUntil != nil {
		t.Fatal("expected invalid state cleared")
	}
	if state.PingCalls() != 1 {
		t.Fatalf("expected 1 ping call, got %d", state.PingCalls())
	}
}

func TestPingClosedDatabaseReconnects(t *testing.T) {
	originalConnect := connectDB
	originalInit := initDatabaseSession
	initDatabaseSession = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig, db *sql.DB) {}
	t.Cleanup(func() {
		connectDB = originalConnect
		initDatabaseSession = originalInit
	})

	closedDB, _ := testdb.New(testdb.Scenario{PingErr: sql.ErrConnDone})
	defer closedDB.Close()
	replacementDB, _ := testdb.New(testdb.Scenario{})
	defer replacementDB.Close()

	connectDB = func(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *sql.DB {
		return replacementDB
	}

	db := &Database{
		Name:    "db1",
		Session: closedDB,
		Config:  DatabaseConfig{ConnectConfig: ConnectConfig{MaxOpenConns: ptr(1)}},
	}

	if err := db.ping(testLogger(), time.Minute); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if db.Session != replacementDB {
		t.Fatal("expected ping to reconnect with replacement session")
	}
}

func TestPingReturnsGenericError(t *testing.T) {
	dbh, _ := testdb.New(testdb.Scenario{PingErr: errors.New("boom")})
	defer dbh.Close()

	db := &Database{
		Name:    "db1",
		Session: dbh,
		Config:  DatabaseConfig{ConnectConfig: ConnectConfig{QueryTimeout: ptr(5)}},
	}

	err := db.ping(testLogger(), time.Minute)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
	if db.Up != 0 {
		t.Fatalf("expected db up metric 0, got %v", db.Up)
	}
}
