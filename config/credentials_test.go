// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"errors"
	"fmt"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDatabaseConfigGetsCredentialsFromOCIUsernamePasswordSecret(t *testing.T) {
	original := getOCIUsernamePasswordSecret
	var calls []string
	getOCIUsernamePasswordSecret = func(ctx context.Context, vaultID, secretName string, authMode oci.AuthMode) (string, string, error) {
		calls = append(calls, fmt.Sprintf("%s/%s/%s", vaultID, secretName, authMode))
		return "scott", "tiger", nil
	}
	t.Cleanup(func() {
		getOCIUsernamePasswordSecret = original
	})

	cfg := DatabaseConfig{
		Vault: &VaultConfig{
			OCI: &OCIVault{
				ID:                     "vault-1",
				Auth:                   "instance_principal",
				UsernamePasswordSecret: "db-credentials",
			},
		},
	}

	credentials, err := cfg.ResolveCredentials()
	if err != nil || credentials.Username != "scott" || credentials.Password != "tiger" {
		t.Fatalf("expected credentials from OCI Vault JSON secret, got %#v, %v", credentials, err)
	}

	want := []string{"vault-1/db-credentials/instance_principal"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected OCI Vault calls: got %#v want %#v", calls, want)
	}
}

func TestDatabaseConfigReturnsOCIUsernamePasswordSecretLookupError(t *testing.T) {
	original := getOCIUsernamePasswordSecret
	getOCIUsernamePasswordSecret = func(ctx context.Context, vaultID, secretName string, authMode oci.AuthMode) (string, string, error) {
		return "", "", errors.New("vault unavailable")
	}
	t.Cleanup(func() {
		getOCIUsernamePasswordSecret = original
	})

	cfg := DatabaseConfig{
		Username: "fallback-username",
		Password: "fallback-password",
		Vault: &VaultConfig{OCI: &OCIVault{
			ID:                     "vault-1",
			UsernamePasswordSecret: "db-credentials",
		}},
	}

	_, err := cfg.ResolveCredentials()
	if err == nil || err.Error() != "vault unavailable" {
		t.Fatalf("expected OCI Vault lookup error, got %v", err)
	}
}

func TestDatabaseConfigPasswordFileOverridesSeparateOCIVaultPasswordSecret(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(passwordFile, []byte("file-password"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}

	original := getOCIVaultSecret
	getOCIVaultSecret = func(ctx context.Context, vaultID, secretName string, authMode oci.AuthMode) (string, error) {
		t.Fatal("OCI Vault should not be called when passwordFile is configured")
		return "", nil
	}
	t.Cleanup(func() {
		getOCIVaultSecret = original
	})

	cfg := DatabaseConfig{
		Username:     "scott",
		PasswordFile: passwordFile,
		Vault: &VaultConfig{OCI: &OCIVault{
			ID:             "vault-1",
			PasswordSecret: "db-password",
		}},
	}

	credentials, err := cfg.ResolveCredentials()
	if err != nil || credentials.Username != "scott" || credentials.Password != "file-password" {
		t.Fatalf("expected password-file credentials, got %#v, %v", credentials, err)
	}
}

func TestDatabaseConfigResolvesSingleOCIVaultCredential(t *testing.T) {
	original := getOCIVaultSecret
	getOCIVaultSecret = func(ctx context.Context, vaultID, secretName string, authMode oci.AuthMode) (string, error) {
		return secretName + "-value", nil
	}
	t.Cleanup(func() {
		getOCIVaultSecret = original
	})

	tests := []struct {
		name     string
		vault    OCIVault
		username string
		password string
	}{
		{
			name:     "username",
			vault:    OCIVault{ID: "vault-1", UsernameSecret: "db-username"},
			username: "db-username-value",
			password: "configured-password",
		},
		{
			name:     "password",
			vault:    OCIVault{ID: "vault-1", PasswordSecret: "db-password"},
			username: "configured-username",
			password: "db-password-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DatabaseConfig{
				Username: "configured-username",
				Password: "configured-password",
				Vault:    &VaultConfig{OCI: &tt.vault},
			}

			credentials, err := cfg.ResolveCredentials()
			if err != nil || credentials.Username != tt.username || credentials.Password != tt.password {
				t.Fatalf("unexpected credentials: %#v, %v", credentials, err)
			}
		})
	}
}

func TestResolveCredentialsContextPropagatesCancellationAndDeadline(t *testing.T) {
	original := getOCIUsernamePasswordSecret
	t.Cleanup(func() { getOCIUsernamePasswordSecret = original })
	calls := 0
	getOCIUsernamePasswordSecret = func(ctx context.Context, _, _ string, _ oci.AuthMode) (string, string, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing secret lookup deadline")
		}
		<-ctx.Done()
		return "", "", ctx.Err()
	}
	cfg := DatabaseConfig{Vault: &VaultConfig{OCI: &OCIVault{UsernamePasswordSecret: "credentials"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cfg.ResolveCredentialsContext(ctx); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("canceled request was not rejected before lookup: calls=%d err=%v", calls, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := cfg.ResolveCredentialsContext(ctx); !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("lookup did not observe parent deadline: calls=%d err=%v", calls, err)
	}
}
