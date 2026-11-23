// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !goora && !oracledb

package db

import (
	"slices"
	"strings"
	"testing"

	"github.com/godror/godror"
	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func requireConnectionParams(t *testing.T, dbconfig config.DatabaseConfig, username, password string, clientInfo config.ClientInfoConfig) godror.ConnectionParams {
	t.Helper()
	params, err := connectionParams(dbconfig, username, password, clientInfo)
	if err != nil {
		t.Fatal(err)
	}
	return params
}

func TestConnectionParamsInitializeExporterSessions(t *testing.T) {
	poolMax := 4
	for _, role := range []string{"", "SYSDBA"} {
		for _, maximum := range []*int{nil, &poolMax} {
			params := requireConnectionParams(t, config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
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
	params := requireConnectionParams(t, config.DatabaseConfig{}, "scott", "tiger", config.ClientInfoConfig{Enabled: &disabled})
	if len(params.OnInitStmts) != 0 || params.OnInit != nil {
		t.Fatal("disabled client info must not initialize sessions")
	}
	params = requireConnectionParams(t, config.DatabaseConfig{}, "scott", "tiger", config.ClientInfoConfig{Label: "team's exporter"})
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
			params := requireConnectionParams(t, config.DatabaseConfig{ConnectConfig: tt.config}, "scott", "tiger", config.ClientInfoConfig{})
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
	params := requireConnectionParams(t, config.DatabaseConfig{}, "scott", "tiger", config.ClientInfoConfig{})
	if !params.IsStandalone() {
		t.Fatal("expected no pool settings to retain godror's standalone default")
	}
	if params.StandaloneConnection.Valid {
		t.Fatalf("expected standaloneConnection to be unset, got %+v", params.StandaloneConnection)
	}
}

func TestConnectionParamsKeepAdministrativeRolesStandalone(t *testing.T) {
	poolMaxConnections := 4
	params := requireConnectionParams(t, config.DatabaseConfig{ConnectConfig: config.ConnectConfig{
		Role:               "SYSDBA",
		PoolMaxConnections: &poolMaxConnections,
	}}, "sys", "tiger", config.ClientInfoConfig{})
	if !params.IsStandalone() {
		t.Fatal("expected SYSDBA connections to remain standalone")
	}
}

func TestConnectionParamsClearUsernameForExternalAuth(t *testing.T) {
	params := requireConnectionParams(t, config.DatabaseConfig{Username: "scott"}, "scott", "", config.ClientInfoConfig{})
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

func TestGodrorOCIProfileUsesPerConnectionWallet(t *testing.T) {
	const prefix = `(description = (ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=orders_low))`
	const security = `(security = (TLS_SERVER_DN_MATCH=ON)(SSL_SERVER_CERT_DN="CN=ADB (prod),O=Oracle"))`
	for _, tt := range []struct {
		name, url, directory, want string
	}{
		{name: "existing TLS settings", url: prefix + security + ")", directory: "/private/orders",
			want: prefix + strings.TrimSuffix(security, ")") + `(MY_WALLET_DIRECTORY="/private/orders")))`},
		{name: "default DN check", url: prefix + `(security=(SSL_SERVER_CERT_DN='CN=Other (prod)')))`, directory: "/private/another wallet (prod)",
			want: prefix + `(security=(SSL_SERVER_CERT_DN='CN=Other (prod)')(MY_WALLET_DIRECTORY="/private/another wallet (prod)")(SSL_SERVER_DN_MATCH=YES)))`},
		{name: "missing security section", url: prefix + ")", directory: "/private/example",
			want: prefix + `(SECURITY=(MY_WALLET_DIRECTORY="/private/example")(SSL_SERVER_DN_MATCH=YES)))`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			params := requireConnectionParams(t, config.DatabaseConfig{
				URL: tt.url, DiscoveryMTLSProfile: true, ConnectConfig: config.ConnectConfig{TNSAdmin: tt.directory},
			}, "exporter", "password", config.ClientInfoConfig{})
			if params.ConnectString != tt.want || params.ConfigDir != "" {
				t.Fatalf("got descriptor %q and ConfigDir %q, want descriptor %q and empty ConfigDir", params.ConnectString, params.ConfigDir, tt.want)
			}
		})
	}
}

func TestGodrorOCIProfileRejectsMissingOrAmbiguousWallet(t *testing.T) {
	for _, tt := range []struct{ url, directory string }{
		{`(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)))`, ""},
		{"adb.example.com:1522/orders_low", "/private/wallet"},
		{`(DESCRIPTION=(SECURITY=(SSL_SERVER_DN_MATCH=YES))`, "/private/wallet"},
		{`(DESCRIPTION=(SECURITY=(WALLET_LOCATION=/other)))`, "/private/wallet"},
		{`(DESCRIPTION=(SECURITY=(my_wallet_directory=/other)))`, "/private/wallet"},
		{`(DESCRIPTION=(SECURITY=(SSL_SERVER_DN_MATCH=YES))(SECURITY=(TLS_SERVER_DN_MATCH=ON)))`, "/private/wallet"},
	} {
		connector, err := newConnector(config.DatabaseConfig{
			URL: tt.url, DiscoveryMTLSProfile: true, ConnectConfig: config.ConnectConfig{TNSAdmin: tt.directory},
		}, config.DatabaseCredentials{Username: "exporter", Password: "password"}, config.ClientInfoConfig{})
		if err == nil || connector != nil {
			t.Fatalf("expected connector error for %q and directory %q", tt.url, tt.directory)
		}
	}
}

func TestGodrorExplicitURLPreservesWalletConfiguration(t *testing.T) {
	const descriptor = `(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=orders_low))(SECURITY=(SSL_SERVER_DN_MATCH=YES)))`
	for _, resourceID := range []string{"", "ocid1.autonomousdatabase.explicit-url"} {
		params := requireConnectionParams(t, config.DatabaseConfig{
			URL: descriptor, ResourceID: resourceID, ConnectConfig: config.ConnectConfig{TNSAdmin: "/private/wallet"},
		}, "exporter", "password", config.ClientInfoConfig{})
		if params.ConnectString != descriptor || params.ConfigDir != "/private/wallet" {
			t.Fatal("explicit URL and TNS Admin directory should be passed through")
		}
	}
}
