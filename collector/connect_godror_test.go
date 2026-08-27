// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora

package collector

import (
	"io"
	"log/slog"
	"testing"
)

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

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestConnectionParamsPoolKeysSelectNativePool(t *testing.T) {
	poolMin, poolMax, poolInc := 1, 5, 1
	config := DatabaseConfig{Username: "user", Password: "secret", URL: "db.example:1521/svc", ConnectConfig: ConnectConfig{
		PoolMinConnections: &poolMin,
		PoolMaxConnections: &poolMax,
		PoolIncrement:      &poolInc,
	}}

	P, err := connectionParams(discardLogger(), "db", config)
	if err != nil {
		t.Fatal(err)
	}
	if P.IsStandalone() {
		t.Fatalf("pool* keys are set but godror would open standalone connections (StandaloneConnection=%+v)", P.StandaloneConnection)
	}
	if P.PoolParams.MinSessions != poolMin || P.PoolParams.MaxSessions != poolMax || P.PoolParams.SessionIncrement != poolInc {
		t.Fatalf("pool params not propagated: %+v", P.PoolParams)
	}
}

func TestConnectionParamsWithoutPoolKeysKeepGodrorDefault(t *testing.T) {
	config := DatabaseConfig{Username: "user", Password: "secret", URL: "db.example:1521/svc"}

	P, err := connectionParams(discardLogger(), "db", config)
	if err != nil {
		t.Fatal(err)
	}
	if P.StandaloneConnection.Valid {
		t.Fatalf("no pool* keys: connection mode must stay at godror default, got %+v", P.StandaloneConnection)
	}
}

func TestConnectionParamsAdminRoleStaysStandalone(t *testing.T) {
	poolMax := 5
	config := DatabaseConfig{Username: "sys", Password: "secret", URL: "db.example:1521/svc", ConnectConfig: ConnectConfig{
		Role:               "SYSDBA",
		PoolMaxConnections: &poolMax,
	}}

	P, err := connectionParams(discardLogger(), "db", config)
	if err != nil {
		t.Fatal(err)
	}
	// godror forces standalone connections for administrative roles.
	if !P.IsStandalone() {
		t.Fatalf("SYSDBA must remain standalone in godror, got %+v", P)
	}
}

func TestConnectionParamsZeroValuedPoolKeySelectsPool(t *testing.T) {
	poolMin := 0 // present but zero: still an explicit request for the native pool
	config := DatabaseConfig{Username: "user", Password: "secret", URL: "db.example:1521/svc", ConnectConfig: ConnectConfig{
		PoolMinConnections: &poolMin,
	}}

	P, err := connectionParams(discardLogger(), "db", config)
	if err != nil {
		t.Fatal(err)
	}
	if P.IsStandalone() {
		t.Fatalf("poolMinConnections: 0 is present but the connection stays standalone: %+v", P.StandaloneConnection)
	}
}

func TestConnectionParamsExternalAuthClearsUsername(t *testing.T) {
	config := DatabaseConfig{Username: "ignored", Password: "", URL: "db.example:1521/svc"}

	P, err := connectionParams(discardLogger(), "db", config)
	if err != nil {
		t.Fatal(err)
	}
	if !P.ExternalAuth.Valid || !P.ExternalAuth.Bool {
		t.Fatalf("empty password must select external auth, got %+v", P.ExternalAuth)
	}
	if P.Username != "" {
		t.Fatalf("external auth must ignore the configured username, got %q", P.Username)
	}
}

func TestWarmupConnectionPoolSizeCappedByPoolMax(t *testing.T) {
	maxOpenConns := 10
	poolMax := 4
	config := DatabaseConfig{ConnectConfig: ConnectConfig{
		MaxOpenConns:       &maxOpenConns,
		PoolMaxConnections: &poolMax,
	}}

	if got := warmupConnectionPoolSize(config); got != poolMax {
		t.Fatalf("warmup must not exceed the native pool MaxSessions: got %d, want %d", got, poolMax)
	}
}
