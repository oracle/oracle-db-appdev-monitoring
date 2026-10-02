// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	go_ora "github.com/sijms/go-ora/v2"
)

var _ driver.ExecerContext = (*go_ora.Connection)(nil)

func TestGooraClientInfoConfiguration(t *testing.T) {
	disabled := false
	for _, tt := range []struct {
		name   string
		config config.ClientInfoConfig
		want   string
	}{
		{name: "default", want: "BEGIN DBMS_APPLICATION_INFO.SET_CLIENT_INFO('oracledb_exporter'); END;"},
		{name: "custom", config: config.ClientInfoConfig{Label: "team's exporter"}, want: "BEGIN DBMS_APPLICATION_INFO.SET_CLIENT_INFO('team''s exporter'); END;"},
		{name: "disabled", config: config.ClientInfoConfig{Enabled: &disabled}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			connector := &sessionTestConnector{}
			conn, err := withClientInfo(connector, tt.config).Connect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			statements := connector.connections[0].statements
			if tt.want == "" {
				if len(statements) != 0 {
					t.Fatalf("disabled client info executed SQL: %v", statements)
				}
			} else if len(statements) != 1 || statements[0] != tt.want {
				t.Fatalf("unexpected initialization statements: %v", statements)
			}
		})
	}
}

type sessionTestConnector struct {
	driver.Connector
	connections []*sessionTestConn
	connectErr  error
	initErr     error
}

func (c *sessionTestConnector) Connect(context.Context) (driver.Conn, error) {
	if c.connectErr != nil {
		return nil, c.connectErr
	}
	conn := &sessionTestConn{initErr: c.initErr}
	c.connections = append(c.connections, conn)
	return conn, nil
}

func (*sessionTestConnector) Driver() driver.Driver { return testQueryDriver{} }

type sessionTestConn struct {
	driver.Conn
	initErr    error
	statements []string
	closed     bool
}

func (c *sessionTestConn) Close() error { c.closed = true; return nil }

func (c *sessionTestConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.statements = append(c.statements, query)
	return driver.RowsAffected(0), c.initErr
}

func TestGooraInitializesEveryPoolConnection(t *testing.T) {
	connector := &sessionTestConnector{}
	pool := sql.OpenDB(initializingConnector{Connector: connector, label: config.DefaultClientInfoLabel})
	t.Cleanup(func() { pool.Close() })
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(2)
	ctx := context.Background()
	first, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	second.Close()
	reused, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reused.Close()
	if len(connector.connections) != 2 {
		t.Fatalf("expected two sessions and reuse, got %d", len(connector.connections))
	}
	// Discard idle sessions so the next acquisition must create a replacement.
	pool.SetMaxIdleConns(0)
	replacement, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Close()
	if len(connector.connections) != 3 {
		t.Fatalf("expected a replacement session, got %d", len(connector.connections))
	}
	for i, conn := range connector.connections {
		if len(conn.statements) != 1 || conn.statements[0] != clientInfoSQL(config.DefaultClientInfoLabel) {
			t.Fatalf("session %d was not initialized exactly once: %v", i, conn.statements)
		}
	}
}

func TestGooraInitializationFailureClosesConnection(t *testing.T) {
	wantErr := errors.New("cannot set client info")
	connector := &sessionTestConnector{initErr: wantErr}
	conn, err := (initializingConnector{Connector: connector, label: config.DefaultClientInfoLabel}).Connect(context.Background())
	if conn != nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected initialization failure, got %v, %v", conn, err)
	}
	if !connector.connections[0].closed {
		t.Fatal("failed session was not closed")
	}
}

func TestGooraConnectFailureSkipsInitialization(t *testing.T) {
	wantErr := errors.New("cannot connect")
	connector := &sessionTestConnector{connectErr: wantErr}
	conn, err := (initializingConnector{Connector: connector, label: config.DefaultClientInfoLabel}).Connect(context.Background())
	if conn != nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected connection failure, got %v, %v", conn, err)
	}
	if len(connector.connections) != 0 {
		t.Fatal("unexpected session after connection failure")
	}
}

func TestGooraInitializationHonorsContext(t *testing.T) {
	connector := &sessionTestConnector{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := (initializingConnector{Connector: connector, label: config.DefaultClientInfoLabel}).Connect(ctx)
	if conn != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v, %v", conn, err)
	}
	if !connector.connections[0].closed {
		t.Fatal("canceled session was not closed")
	}
}

func TestEffectiveSQLPoolLimitsPreferGooraPoolSettings(t *testing.T) {
	maxOpenConns := 10
	maxIdleConns := 6
	poolMaxConnections := 4
	poolMinConnections := 2
	dbconfig := config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		MaxIdleConns:       &maxIdleConns,
		PoolMaxConnections: &poolMaxConnections,
		PoolMinConnections: &poolMinConnections,
	}}

	gotMaxOpenConns, gotMaxIdleConns := effectiveSQLPoolLimits(dbconfig)
	if gotMaxOpenConns != poolMaxConnections {
		t.Fatalf("expected poolMaxConnections to set max open connections, got %d", gotMaxOpenConns)
	}
	if gotMaxIdleConns != poolMinConnections {
		t.Fatalf("expected poolMinConnections to set max idle connections, got %d", gotMaxIdleConns)
	}
	if got := warmupConnectionPoolSize(dbconfig); got != poolMaxConnections {
		t.Fatalf("expected warmup to use poolMaxConnections, got %d", got)
	}
}

func TestEffectiveSQLPoolLimitsFallbackToSQLSettingsForGoora(t *testing.T) {
	maxOpenConns := 8
	maxIdleConns := 3
	dbconfig := config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		MaxOpenConns: &maxOpenConns,
		MaxIdleConns: &maxIdleConns,
	}}

	gotMaxOpenConns, gotMaxIdleConns := effectiveSQLPoolLimits(dbconfig)
	if gotMaxOpenConns != maxOpenConns {
		t.Fatalf("expected maxOpenConns fallback, got %d", gotMaxOpenConns)
	}
	if gotMaxIdleConns != maxIdleConns {
		t.Fatalf("expected maxIdleConns fallback, got %d", gotMaxIdleConns)
	}
}

func TestInitDBKeepsGooraPoolMaxConnections(t *testing.T) {
	maxOpenConns := 10
	poolMaxConnections := 4
	dbconfig := config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		PoolMaxConnections: &poolMaxConnections,
	}}
	db := openTestQueryDB(t)

	initdb(testLogger(), "db1", dbconfig, db)

	if got := db.Stats().MaxOpenConnections; got != poolMaxConnections {
		t.Fatalf("expected initdb to keep poolMaxConnections as max open connections, got %d", got)
	}
}
