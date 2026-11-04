// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora || oracledb

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

type credentialFailureConnector struct{ err error }

func (c credentialFailureConnector) Connect(context.Context) (driver.Conn, error) {
	return credentialFailureConnection{c.err}, nil
}
func (credentialFailureConnector) Driver() driver.Driver { return testQueryDriver{} }

type credentialFailureConnection struct{ err error }

func (c credentialFailureConnection) Ping(context.Context) error { return c.err }
func (credentialFailureConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (credentialFailureConnection) Close() error { return nil }
func (credentialFailureConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}

func TestCredentialFailureRetiresVaultPoolAndReresolvesAfterBackoff(t *testing.T) {
	oldSession := sql.OpenDB(credentialFailureConnector{credentialRefreshAuthError()})
	t.Cleanup(func() { _ = oldSession.Close() })
	// A missing password file acts as an observable, network-free credential
	// resolver failure after retirement. It must never be read during backoff.
	missing := filepath.Join(t.TempDir(), "rotated-password")
	d := &Database{Session: oldSession, up: 1, Config: config.DatabaseConfig{
		PasswordFile: missing,
		Vault:        &config.VaultConfig{OCI: &config.OCIVault{}},
	}}
	if err := d.Ping(testLogger(), time.Minute); !isInvalidCredentialsError(err) {
		t.Fatalf("expected driver auth error, got %v", err)
	}
	if d.Session != nil || d.Up() != 0 || d.IsValid() == nil {
		t.Fatal("Ping did not retire the stale Vault credential connector")
	}
	if err := oldSession.Ping(); err == nil {
		t.Fatal("retired SQL pool remains usable")
	}
	if err := d.Ping(testLogger(), time.Minute); err == nil || !strings.Contains(err.Error(), "reconnect delayed") {
		t.Fatalf("retry failed to honor backoff: %v", err)
	}
	d.clearInvalid()
	err := d.Ping(testLogger(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "failed to read password file") {
		t.Fatalf("retry did not resolve fresh credentials: %v", err)
	}
	if d.Session != nil || d.IsValid() == nil {
		t.Fatal("failed fresh credential resolution did not retain backoff")
	}
}

func TestCredentialFailureWithoutVaultKeepsConfiguredPool(t *testing.T) {
	session := sql.OpenDB(credentialFailureConnector{credentialRefreshAuthError()})
	t.Cleanup(func() { _ = session.Close() })
	d := &Database{Session: session, up: 1, Config: config.DatabaseConfig{Username: "exporter", Password: "inline-password"}}
	if err := d.Ping(testLogger(), time.Minute); !isInvalidCredentialsError(err) {
		t.Fatalf("expected auth failure, got %v", err)
	}
	if d.Session != session || d.Up() != 0 || d.IsValid() == nil {
		t.Fatal("inline credential handling changed unexpectedly")
	}
}

func TestCredentialRefreshObservesDatabaseCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &Database{ctx: ctx, Config: config.DatabaseConfig{Vault: &config.VaultConfig{OCI: &config.OCIVault{UsernamePasswordSecret: "credentials"}}}}
	err := d.Ping(testLogger(), time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("credential refresh ignored database lifecycle cancellation: %v", err)
	}
	if d.Session != nil {
		t.Fatal("cancelled database created a SQL session")
	}
}
