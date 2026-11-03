// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build oracledb && !goora

package db

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func TestOracleDBConnectorConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name string
		url  string
		role string
	}{
		{name: "easy connect", url: "localhost:1521/FREEPDB1"},
		{name: "easy connect options", url: "tcp://localhost:1521/FREEPDB1?transport_connect_timeout=5"},
		{name: "descriptor", url: "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=FREEPDB1)))"},
		{name: "administrative role", url: "localhost:1521/FREEPDB1", role: "SYSDBA"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DatabaseConfig{URL: tt.url, ConnectConfig: config.ConnectConfig{Role: tt.role}}
			// Credentials containing DSN delimiters must not be interpolated into a DSN.
			connector, err := newOracleDBConnector(cfg, "user/name@domain", "p@ss/?#%word")
			if err != nil {
				t.Fatal(err)
			}
			if connector == nil {
				t.Fatal("expected connector")
			}
		})
	}
}

func TestOracleDBRejectsUnsupportedConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name     string
		username string
		password string
		options  config.ConnectConfig
		url      string
		want     string
	}{
		{name: "no password", username: "user", want: "username/password"},
		{name: "no username", password: "secret", want: "username/password"},
		{name: "external auth", username: "user", password: "secret", options: config.ConnectConfig{ExternalAuth: true}, want: "external authentication"},
		{name: "TNS directory", username: "user", password: "secret", options: config.ConnectConfig{TNSAdmin: "/wallet"}, want: "tnsAdmin"},
		{name: "invalid role", username: "user", password: "secret", options: config.ConnectConfig{Role: "UNKNOWN"}, want: "connection configuration"},
		{name: "invalid DSN", username: "user", password: "secret", url: "user/secret@", want: "connection configuration"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dsn := tt.url
			if dsn == "" {
				dsn = "localhost:1521/FREEPDB1"
			}
			connector, err := newOracleDBConnector(config.DatabaseConfig{URL: dsn, ConnectConfig: tt.options}, tt.username, tt.password)
			if connector != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error and no connector, got %v, %v", tt.want, connector, err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error exposed password: %v", err)
			}
		})
	}
}

type oracleDBTestError string

func (e oracleDBTestError) Error() string     { return string(e) }
func (e oracleDBTestError) ErrorCode() string { return string(e) }

func TestOracleDBErrorClassification(t *testing.T) {
	for _, tt := range []struct {
		code      string
		invalid   bool
		temporary bool
	}{
		{code: "ORA-01017", invalid: true},
		{code: "ORA-28000", invalid: true},
		{code: "ORA-01033", temporary: true},
		{code: "ORA-03113", temporary: true},
		{code: "ORA-03114", temporary: true},
		{code: "ORA-12537", temporary: true},
		{code: "ORA-12541", temporary: true},
		{code: "ORA-00942"},
		{code: "OGD-01017"},
		{code: "ORA-invalid"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			original := oracleDBTestError(tt.code)
			for _, err := range []error{original, fmt.Errorf("wrapped: %w", fmt.Errorf("again: %w", original))} {
				if got := isInvalidCredentialsError(err); got != tt.invalid {
					t.Fatalf("invalid credentials: got %v, want %v", got, tt.invalid)
				}
				if got := isTemporaryConnectionError(err); got != tt.temporary {
					t.Fatalf("temporary connection: got %v, want %v", got, tt.temporary)
				}
			}
		})
	}
	for _, err := range []error{nil, errors.New("ORA-01017")} {
		if isInvalidCredentialsError(err) || isTemporaryConnectionError(err) {
			t.Fatalf("unexpected classification: %v", err)
		}
	}
}

func TestOracleDBDoesNotReparseExporterFlags(t *testing.T) {
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	flag.CommandLine = flag.NewFlagSet("driver", flag.ContinueOnError)
	os.Args = []string{"exporter", "--config.file", "config.yaml"}
	prepareOracleDBFlags()
	if !flag.Parsed() {
		t.Fatal("driver could still parse exporter arguments")
	}
	if flag.NArg() != 0 {
		t.Fatalf("unexpected driver arguments: %v", flag.Args())
	}
}
