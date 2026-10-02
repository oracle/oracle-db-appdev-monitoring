// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora

package db

import (
	"slices"
	"testing"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func TestConnectionParamsInitializeExporterSessions(t *testing.T) {
	poolMax := 4
	for _, role := range []string{"", "SYSDBA"} {
		for _, maximum := range []*int{nil, &poolMax} {
			params := connectionParams(config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
				Role: role, PoolMaxConnections: maximum,
			}}, "scott", "tiger", config.ClientInfoConfig{})
			if !slices.Equal(params.OnInitStmts, []string{clientInfoSQL(config.DefaultClientInfoLabel)}) {
				t.Fatalf("role=%q pool=%v: missing session marker: %v", role, maximum, params.OnInitStmts)
			}
			if params.OnInit != nil || params.InitOnNewConn {
				t.Fatal("expected session statements to run for both new and acquired Oracle sessions")
			}
		}
	}
}

func TestConnectionParamsClientInfoConfiguration(t *testing.T) {
	disabled := false
	params := connectionParams(config.DatabaseConfig{}, "scott", "tiger", config.ClientInfoConfig{Enabled: &disabled})
	if len(params.OnInitStmts) != 0 || params.OnInit != nil {
		t.Fatal("disabled client info must not initialize sessions")
	}
	params = connectionParams(config.DatabaseConfig{}, "scott", "tiger", config.ClientInfoConfig{Label: "team's exporter"})
	if !slices.Equal(params.OnInitStmts, []string{"BEGIN DBMS_APPLICATION_INFO.SET_CLIENT_INFO('team''s exporter'); END;"}) {
		t.Fatalf("expected safely quoted custom label, got %v", params.OnInitStmts)
	}
}

func TestConnectionParamsUsePoolWhenConfigured(t *testing.T) {
	zero := 0
	tests := []struct {
		name   string
		config config.ConnectConfig
	}{
		{name: "pool increment", config: config.ConnectConfig{PoolIncrement: &zero}},
		{name: "pool maximum", config: config.ConnectConfig{PoolMaxConnections: &zero}},
		{name: "pool minimum", config: config.ConnectConfig{PoolMinConnections: &zero}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := connectionParams(config.DatabaseConfig{ConnectConfig: tt.config}, "scott", "tiger", config.ClientInfoConfig{})
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
	params := connectionParams(config.DatabaseConfig{}, "scott", "tiger", config.ClientInfoConfig{})
	if !params.IsStandalone() {
		t.Fatal("expected no pool settings to retain godror's standalone default")
	}
	if params.StandaloneConnection.Valid {
		t.Fatalf("expected standaloneConnection to be unset, got %+v", params.StandaloneConnection)
	}
}

func TestConnectionParamsKeepAdministrativeRolesStandalone(t *testing.T) {
	poolMaxConnections := 4
	params := connectionParams(config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		Role:               "SYSDBA",
		PoolMaxConnections: &poolMaxConnections,
	}}, "sys", "tiger", config.ClientInfoConfig{})
	if !params.IsStandalone() {
		t.Fatal("expected SYSDBA connections to remain standalone")
	}
}

func TestConnectionParamsClearUsernameForExternalAuth(t *testing.T) {
	params := connectionParams(config.DatabaseConfig{Username: "scott"}, "scott", "", config.ClientInfoConfig{})
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
	dbconfig := config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		MaxIdleConns:       &maxIdleConns,
		PoolMaxConnections: &poolMaxConnections,
		PoolMinConnections: &poolMinConnections,
	}}

	gotMaxOpenConns, gotMaxIdleConns := effectiveSQLPoolLimits(dbconfig)
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
	dbconfig := config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		PoolMaxConnections: &poolMaxConnections,
	}}

	if got := warmupConnectionPoolSize(dbconfig); got != poolMaxConnections {
		t.Fatalf("expected warmup to keep existing poolMaxConnections fallback, got %d", got)
	}
}

func TestWarmupConnectionPoolSizeCapsAtGodrorPoolMaximum(t *testing.T) {
	maxOpenConns := 10
	poolMaxConnections := 4
	dbconfig := config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		PoolMaxConnections: &poolMaxConnections,
	}}

	if got := warmupConnectionPoolSize(dbconfig); got != poolMaxConnections {
		t.Fatalf("expected warmup to be capped at poolMaxConnections, got %d", got)
	}
}
