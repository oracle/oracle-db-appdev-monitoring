// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora && !oracledb

package db

import (
	"net/url"
	"testing"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/sijms/go-ora/v3/configurations"
)

func TestGooraDiscoveryDescriptorPreservesTLSAndCredentials(t *testing.T) {
	descriptor := `(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orders_low.adb.oraclecloud.com))(SECURITY=(SSL_SERVER_DN_MATCH=YES)(SSL_SERVER_CERT_DN="CN=adb.example.com,OU=Oracle,O=Oracle Corporation")))`
	dsn := gooraDSN(config.DatabaseConfig{URL: descriptor}, "user/name@domain", "p@ss/?#%word")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("connStr") != descriptor {
		t.Fatal("descriptor or comma-containing certificate DN was modified")
	}
	parsed, err := configurations.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Protocol != "tcps" || !parsed.SSL || !parsed.SSLVerify || len(parsed.Servers) != 1 || parsed.Servers[0].Addr != "adb.example.com" || parsed.ServiceName != "orders_low.adb.oraclecloud.com" || parsed.UserID != "user/name@domain" || parsed.Password != "p@ss/?#%word" {
		t.Fatalf("incorrect secure connection translation: protocol=%s SSL=%v verify=%v service=%s", parsed.Protocol, parsed.SSL, parsed.SSLVerify, parsed.ServiceName)
	}
}

func TestGooraDiscoveryWalletIsPerConnection(t *testing.T) {
	descriptor := `(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=adb.example.com)(PORT=1522))(CONNECT_DATA=(SERVICE_NAME=orders_low)))`
	dsn := gooraDSN(config.DatabaseConfig{URL: descriptor, ConnectConfig: config.ConnectConfig{TNSAdmin: "/private/wallet with spaces"}}, "exporter", "password")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("WALLET") != "/private/wallet with spaces" || u.Query().Get("SSL") != "true" || u.Query().Get("SSL VERIFY") != "true" {
		t.Fatal("wallet path or TLS verification not supplied to connector")
	}
}
