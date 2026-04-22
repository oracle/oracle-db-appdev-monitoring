// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestWarmupConnectionPoolWithOCIVaultLookupErrorUsesBackoff(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	original := getOCIVaultSecret
	getOCIVaultSecret = func(vaultID, secretName string) (string, error) {
		return "", errors.New("vault unavailable")
	}
	t.Cleanup(func() {
		getOCIVaultSecret = original
	})

	db := NewDatabase(logger, "database", "db1", DatabaseConfig{
		URL: "dbhost/service",
		Vault: &VaultConfig{
			OCI: &OCIVault{
				ID:             "vault-1",
				PasswordSecret: "db-password",
			},
		},
	})

	if db.Session != nil {
		t.Fatal("expected session initialization to fail when OCI Vault lookup fails")
	}

	err := db.WarmupConnectionPool(logger, time.Minute)
	if err == nil {
		t.Fatal("expected warmup to fail after OCI Vault lookup error")
	}
	if err.Error() != "vault unavailable" {
		t.Fatalf("expected vault error to be preserved, got %v", err)
	}
	if db.invalidUntil == nil {
		t.Fatal("expected invalidUntil to be set after vault lookup failure")
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
				MountType:  hashiCorpMountTypeKVv2ForTest(),
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

func hashiCorpMountTypeKVv2ForTest() string {
	return "kvv2"
}
