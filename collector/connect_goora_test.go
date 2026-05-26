// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora

package collector

import "testing"

func TestEffectiveSQLPoolLimitsPreferGooraPoolSettings(t *testing.T) {
	maxOpenConns := 10
	maxIdleConns := 6
	poolMaxConnections := 4
	poolMinConnections := 2
	config := DatabaseConfig{ConnectConfig: ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		MaxIdleConns:       &maxIdleConns,
		PoolMaxConnections: &poolMaxConnections,
		PoolMinConnections: &poolMinConnections,
	}}

	gotMaxOpenConns, gotMaxIdleConns := effectiveSQLPoolLimits(config)
	if gotMaxOpenConns != poolMaxConnections {
		t.Fatalf("expected poolMaxConnections to set max open connections, got %d", gotMaxOpenConns)
	}
	if gotMaxIdleConns != poolMinConnections {
		t.Fatalf("expected poolMinConnections to set max idle connections, got %d", gotMaxIdleConns)
	}
	if got := warmupConnectionPoolSize(config); got != poolMaxConnections {
		t.Fatalf("expected warmup to use poolMaxConnections, got %d", got)
	}
}

func TestEffectiveSQLPoolLimitsFallbackToSQLSettingsForGoora(t *testing.T) {
	maxOpenConns := 8
	maxIdleConns := 3
	config := DatabaseConfig{ConnectConfig: ConnectConfig{
		MaxOpenConns: &maxOpenConns,
		MaxIdleConns: &maxIdleConns,
	}}

	gotMaxOpenConns, gotMaxIdleConns := effectiveSQLPoolLimits(config)
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
	config := DatabaseConfig{ConnectConfig: ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		PoolMaxConnections: &poolMaxConnections,
	}}
	db := openTestQueryDB(t)

	initdb(testLogger(), "db1", config, db)

	if got := db.Stats().MaxOpenConnections; got != poolMaxConnections {
		t.Fatalf("expected initdb to keep poolMaxConnections as max open connections, got %d", got)
	}
}
