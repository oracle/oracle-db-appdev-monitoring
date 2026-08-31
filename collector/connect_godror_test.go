// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora

package collector

import "testing"

func TestConnectionParamsUsePoolWhenConfigured(t *testing.T) {
	zero := 0
	tests := []struct {
		name   string
		config ConnectConfig
	}{
		{name: "pool increment", config: ConnectConfig{PoolIncrement: &zero}},
		{name: "pool maximum", config: ConnectConfig{PoolMaxConnections: &zero}},
		{name: "pool minimum", config: ConnectConfig{PoolMinConnections: &zero}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := connectionParams(DatabaseConfig{ConnectConfig: tt.config}, "scott", "tiger")
			if params.IsStandalone() {
				t.Fatal("expected an explicit pool setting to enable ODPI-C pooling")
			}
			if !params.StandaloneConnection.Valid || params.StandaloneConnection.Bool {
				t.Fatalf("expected standaloneConnection=false, got %+v", params.StandaloneConnection)
			}
		})
	}
}

func TestConnectionParamsDefaultsToStandalone(t *testing.T) {
	params := connectionParams(DatabaseConfig{}, "scott", "tiger")
	if !params.IsStandalone() {
		t.Fatal("expected no pool settings to retain godror's standalone default")
	}
	if params.StandaloneConnection.Valid {
		t.Fatalf("expected standaloneConnection to be unset, got %+v", params.StandaloneConnection)
	}
}

func TestConnectionParamsKeepAdministrativeRolesStandalone(t *testing.T) {
	poolMaxConnections := 4
	params := connectionParams(DatabaseConfig{ConnectConfig: ConnectConfig{
		Role:               "SYSDBA",
		PoolMaxConnections: &poolMaxConnections,
	}}, "sys", "tiger")
	if !params.IsStandalone() {
		t.Fatal("expected SYSDBA connections to remain standalone")
	}
}

func TestConnectionParamsClearUsernameForExternalAuth(t *testing.T) {
	params := connectionParams(DatabaseConfig{Username: "scott"}, "scott", "")
	if params.Username != "" {
		t.Fatalf("expected external authentication to clear username, got %q", params.Username)
	}
	if !params.ExternalAuth.Valid || !params.ExternalAuth.Bool {
		t.Fatalf("expected external authentication, got %+v", params.ExternalAuth)
	}
}

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

func TestWarmupConnectionPoolSizeCapsAtGodrorPoolMaximum(t *testing.T) {
	maxOpenConns := 10
	poolMaxConnections := 4
	config := DatabaseConfig{ConnectConfig: ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		PoolMaxConnections: &poolMaxConnections,
	}}

	if got := warmupConnectionPoolSize(config); got != poolMaxConnections {
		t.Fatalf("expected warmup to be capped at poolMaxConnections, got %d", got)
	}
}
