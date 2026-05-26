// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora

package collector

import "testing"

func TestEffectiveSQLPoolLimitsUseSQLSettingsForGodror(t *testing.T) {
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
	if gotMaxOpenConns != maxOpenConns {
		t.Fatalf("expected maxOpenConns to set max open connections, got %d", gotMaxOpenConns)
	}
	if gotMaxIdleConns != maxIdleConns {
		t.Fatalf("expected maxIdleConns to set max idle connections, got %d", gotMaxIdleConns)
	}
}

func TestWarmupConnectionPoolSizePreservesGodrorPoolFallback(t *testing.T) {
	maxOpenConns := 0
	poolMaxConnections := 4
	config := DatabaseConfig{ConnectConfig: ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		PoolMaxConnections: &poolMaxConnections,
	}}

	if got := warmupConnectionPoolSize(config); got != poolMaxConnections {
		t.Fatalf("expected warmup to keep existing poolMaxConnections fallback, got %d", got)
	}
}
