// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/ocivault"
)

func TestConnectConfigGetConnMaxLifetime(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := ConnectConfig{}

		if got := cfg.GetConnMaxLifetime(); got != 30*time.Minute {
			t.Fatalf("expected default connection max lifetime of 30m, got %s", got)
		}
	})

	t.Run("configured", func(t *testing.T) {
		lifetime := 10 * time.Minute
		cfg := ConnectConfig{ConnMaxLifetime: &lifetime}

		if got := cfg.GetConnMaxLifetime(); got != lifetime {
			t.Fatalf("expected configured connection max lifetime of %s, got %s", lifetime, got)
		}
	})
}

func TestDatabaseConfigGetPasswordReturnsPasswordFileError(t *testing.T) {
	cfg := DatabaseConfig{
		PasswordFile: filepath.Join(t.TempDir(), "missing-password"),
	}

	_, err := cfg.GetPassword()
	if err == nil {
		t.Fatal("expected missing password file to return an error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing file error, got %v", err)
	}
}

func TestDatabaseConfigPassesOCIVaultAuthMode(t *testing.T) {
	original := getOCIVaultSecret
	var calls []string
	getOCIVaultSecret = func(vaultID, secretName string, authMode ocivault.AuthMode) (string, error) {
		calls = append(calls, fmt.Sprintf("%s/%s/%s", vaultID, secretName, authMode))
		return "secret-value", nil
	}
	t.Cleanup(func() {
		getOCIVaultSecret = original
	})

	cfg := DatabaseConfig{
		Vault: &VaultConfig{
			OCI: &OCIVault{
				ID:             "vault-1",
				Auth:           "instance_principal",
				UsernameSecret: "db-username",
				PasswordSecret: "db-password",
			},
		},
	}

	if got, err := cfg.GetUsername(); err != nil || got != "secret-value" {
		t.Fatalf("expected username from OCI Vault, got %q, %v", got, err)
	}
	if got, err := cfg.GetPassword(); err != nil || got != "secret-value" {
		t.Fatalf("expected password from OCI Vault, got %q, %v", got, err)
	}

	want := []string{
		"vault-1/db-username/instance_principal",
		"vault-1/db-password/instance_principal",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected OCI Vault calls: got %#v want %#v", calls, want)
	}
}

func TestMetricsConfigurationValidateOCIVaultAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authModes := []ocivault.AuthMode{"", "config_file", "instance_principal", "resource_principal", "workload_identity"}

	for _, authMode := range authModes {
		t.Run("valid "+string(authMode), func(t *testing.T) {
			cfg := &MetricsConfiguration{
				Databases: map[string]DatabaseConfig{
					"db1": {
						Vault: &VaultConfig{
							OCI: &OCIVault{
								ID:             "vault-1",
								Auth:           authMode,
								PasswordSecret: "db-password",
							},
						},
					},
				},
			}

			if err := cfg.validate(logger); err != nil {
				t.Fatalf("expected auth mode %q to validate, got %v", authMode, err)
			}
		})
	}
}

func TestMetricsConfigurationValidateRejectsInvalidOCIVaultAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &MetricsConfiguration{
		Databases: map[string]DatabaseConfig{
			"db1": {
				Vault: &VaultConfig{
					OCI: &OCIVault{
						ID:             "vault-1",
						Auth:           "api_key",
						PasswordSecret: "db-password",
					},
				},
			},
		},
	}

	err := cfg.validate(logger)
	if err == nil {
		t.Fatal("expected invalid OCI Vault auth mode to fail validation")
	}
	if !strings.Contains(err.Error(), "database \"db1\"") || !strings.Contains(err.Error(), "accepted values") {
		t.Fatalf("expected validation error to include database and accepted values, got %v", err)
	}
}
