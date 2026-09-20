// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/oracle/oracle-db-appdev-monitoring/v2/oci"
)

func TestDatabaseConfigReturnsOCIVaultLookupError(t *testing.T) {
	original := getOCIVaultSecret
	getOCIVaultSecret = func(string, string, oci.AuthMode) (string, error) {
		return "", errors.New("vault unavailable")
	}
	t.Cleanup(func() {
		getOCIVaultSecret = original
	})

	cfg := DatabaseConfig{Vault: &VaultConfig{OCI: &OCIVault{
		ID:             "vault-1",
		PasswordSecret: "db-password",
	}}}
	if _, err := cfg.GetPassword(); err == nil || err.Error() != "vault unavailable" {
		t.Fatalf("expected OCI Vault lookup error to be preserved, got %v", err)
	}
}

func TestHashiCorpVaultLookupErrorIsReturned(t *testing.T) {
	original := getHashiCorpVaultSecret
	getHashiCorpVaultSecret = func(logger *slog.Logger, cfg *HashiCorpVault, requiredKeys []string) (map[string]string, error) {
		return nil, errors.New("hashicorp vault unavailable")
	}
	t.Cleanup(func() {
		getHashiCorpVaultSecret = original
	})

	cfg := DatabaseConfig{
		Vault: &VaultConfig{
			HashiCorp: &HashiCorpVault{
				MountType:  "kvv2",
				MountName:  "secret",
				SecretPath: "db/prod",
			},
		},
	}

	_, err := cfg.GetPassword()
	if err == nil {
		t.Fatal("expected HashiCorp Vault lookup error")
	}
	if err.Error() != "hashicorp vault unavailable" {
		t.Fatalf("expected HashiCorp Vault error to be preserved, got %v", err)
	}
}

func TestAzureVaultLookupErrorIsReturned(t *testing.T) {
	original := getAZVaultSecret
	getAZVaultSecret = func(string, string) (string, error) {
		return "", errors.New("azure vault unavailable")
	}
	t.Cleanup(func() {
		getAZVaultSecret = original
	})

	cfg := DatabaseConfig{
		Vault: &VaultConfig{
			Azure: &AZVault{
				ID:             "vault-1",
				PasswordSecret: "db-password",
			},
		},
	}

	_, err := cfg.GetPassword()
	if err == nil {
		t.Fatal("expected Azure Vault lookup error")
	}
	if err.Error() != "azure vault unavailable" {
		t.Fatalf("expected Azure Vault error to be preserved, got %v", err)
	}
}
