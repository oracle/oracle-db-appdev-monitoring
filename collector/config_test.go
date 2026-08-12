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

	"github.com/oracle/oracle-db-appdev-monitoring/oci"
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

	_, err := cfg.ResolveCredentials()
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
	getOCIVaultSecret = func(vaultID, secretName string, authMode oci.AuthMode) (string, error) {
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

	credentials, err := cfg.ResolveCredentials()
	if err != nil || credentials.Username != "secret-value" || credentials.Password != "secret-value" {
		t.Fatalf("expected credentials from OCI Vault, got %#v, %v", credentials, err)
	}

	want := []string{
		"vault-1/db-password/instance_principal",
		"vault-1/db-username/instance_principal",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected OCI Vault calls: got %#v want %#v", calls, want)
	}
}

func TestDatabaseConfigGetsCredentialsFromOCIUsernamePasswordSecret(t *testing.T) {
	original := getOCIUsernamePasswordSecret
	var calls []string
	getOCIUsernamePasswordSecret = func(vaultID, secretName string, authMode oci.AuthMode) (string, string, error) {
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
	getOCIUsernamePasswordSecret = func(vaultID, secretName string, authMode oci.AuthMode) (string, string, error) {
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
	getOCIVaultSecret = func(vaultID, secretName string, authMode oci.AuthMode) (string, error) {
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
	getOCIVaultSecret = func(vaultID, secretName string, authMode oci.AuthMode) (string, error) {
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

func TestMetricsConfigurationValidateOCIVaultAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authModes := []oci.AuthMode{"", "config_file", "instance_principal", "resource_principal", "workload_identity"}

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

func TestLoadMetricsConfigurationAppliesConfigFileDefaults(t *testing.T) {
	configPath := writeExporterConfig(t, `
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
`)

	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if cfg.MetricsPath != "/metrics" {
		t.Fatalf("expected default metrics path, got %q", cfg.MetricsPath)
	}
	if cfg.Metrics.Default != "default-metrics.toml" {
		t.Fatalf("expected default metrics file, got %q", cfg.Metrics.Default)
	}
	if cfg.Logging.Level != "info" {
		t.Fatalf("expected default log level, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "logfmt" {
		t.Fatalf("expected default log format, got %q", cfg.Logging.Format)
	}
	if cfg.LogDestination() != "/log/alert.log" {
		t.Fatalf("expected default log destination, got %q", cfg.LogDestination())
	}
	if cfg.LogInterval() != 15*time.Second {
		t.Fatalf("expected default log interval, got %s", cfg.LogInterval())
	}
	if got := *cfg.Web.ListenAddresses; len(got) != 1 || got[0] != ":9161" {
		t.Fatalf("expected default web listen address, got %#v", got)
	}
}

func TestLoadMetricsConfigurationLoadsDatabasesFromOCI(t *testing.T) {
	configPath := writeExporterConfig(t, `
databasesFrom:
  oci:
    auth: workload_identity
    compartments:
      - ocid1.compartment.oc1..example1
      - ocid1.compartment.oc1..example2
    filters:
      lifecycleState: AVAILABLE
      requiredTags:
        oracledb-metrics-exporter-enabled: "true"
`)

	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}
	if cfg.DatabasesFrom == nil || cfg.DatabasesFrom.OCI == nil {
		t.Fatal("expected OCI database discovery configuration")
	}
	if got := cfg.DatabasesFrom.OCI.Auth; got != oci.AuthModeWorkloadIdentity {
		t.Fatalf("expected workload identity auth, got %q", got)
	}
	if got := cfg.DatabasesFrom.OCI.Compartments; len(got) != 2 || got[0] != "ocid1.compartment.oc1..example1" || got[1] != "ocid1.compartment.oc1..example2" {
		t.Fatalf("unexpected compartments: %#v", got)
	}
	if got := cfg.DatabasesFrom.OCI.Filters.LifecycleState; got != "AVAILABLE" {
		t.Fatalf("expected AVAILABLE lifecycle state, got %q", got)
	}
	if got := cfg.DatabasesFrom.OCI.Filters.RequiredTags; got["oracledb-metrics-exporter-enabled"] != "true" {
		t.Fatalf("expected required tag, got %#v", got)
	}
}

func TestLoadMetricsConfigurationMapsLegacyListenAddressToWebConfig(t *testing.T) {
	configPath := writeExporterConfig(t, `
listenAddress: 127.0.0.1:9161
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
`)

	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if got := *cfg.Web.ListenAddresses; len(got) != 1 || got[0] != "127.0.0.1:9161" {
		t.Fatalf("expected legacy listenAddress to configure web listen address, got %#v", got)
	}
}

func TestLoadMetricsConfigurationPrefersWebListenAddresses(t *testing.T) {
	configPath := writeExporterConfig(t, `
listenAddress: 127.0.0.1:9161
web:
  listenAddresses:
    - 127.0.0.1:9162
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
`)

	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if got := *cfg.Web.ListenAddresses; len(got) != 1 || got[0] != "127.0.0.1:9162" {
		t.Fatalf("expected web.listenAddresses to take precedence, got %#v", got)
	}
}

func TestLoadMetricsConfigurationAcceptsLogLevelAndFormat(t *testing.T) {
	configPath := writeExporterConfig(t, `
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
log:
  level: debug
  format: json
`)

	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("expected configured log level, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("expected configured log format, got %q", cfg.Logging.Format)
	}
}

func TestLoadMetricsConfigurationRejectsInvalidLogLevelAndFormat(t *testing.T) {
	tests := []struct {
		name    string
		logYAML string
		wantErr string
	}{
		{
			name: "invalid level",
			logYAML: `
log:
  level: trace
`,
			wantErr: "invalid log.level",
		},
		{
			name: "invalid format",
			logYAML: `
log:
  format: text
`,
			wantErr: "invalid log.format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := writeExporterConfig(t, `
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
`+tt.logYAML)

			_, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
			if err == nil {
				t.Fatal("expected invalid logging config to fail")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
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

func TestLoadMetricsConfigurationRequiresConfigFile(t *testing.T) {
	_, err := LoadMetricsConfiguration(testLogger(), &Config{})
	if err == nil {
		t.Fatal("expected missing config file to fail")
	}
	if !strings.Contains(err.Error(), "config file is required") {
		t.Fatalf("expected required config file error, got %v", err)
	}
}

func TestLoadMetricsConfigurationLoadsOTLPConfig(t *testing.T) {
	t.Setenv("OTLP_TOKEN", "test-token")
	configPath := writeExporterConfig(t, `
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
metrics:
  scrapeInterval: 15s
otlp:
  endpoint: https://otel-collector:4317
  headers:
    Authorization: "Bearer ${OTLP_TOKEN}"
  resourceAttributes:
    deployment.environment: test
  tls:
    caFile: /etc/otel/ca.pem
    serverName: collector.internal
    minVersion: TLS1.3
`)

	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}
	if cfg.OTLP == nil || cfg.OTLP.Endpoint != "https://otel-collector:4317" {
		t.Fatalf("unexpected OTLP endpoint: %#v", cfg.OTLP)
	}
	if got := *cfg.OTLP.Timeout; got != 10*time.Second {
		t.Fatalf("expected default OTLP timeout of 10s, got %s", got)
	}
	if got := cfg.OTLP.Headers["Authorization"]; got != "Bearer test-token" {
		t.Fatalf("expected expanded OTLP header, got %q", got)
	}
	if cfg.OTLP.TLS == nil || cfg.OTLP.TLS.CAFile != "/etc/otel/ca.pem" || cfg.OTLP.TLS.ServerName != "collector.internal" || cfg.OTLP.TLS.MinVersion != "TLS1.3" {
		t.Fatalf("unexpected OTLP TLS configuration: %#v", cfg.OTLP.TLS)
	}
}

func TestLoadMetricsConfigurationRejectsInvalidOTLPConfig(t *testing.T) {
	tests := []struct {
		name    string
		metrics string
		otlp    string
		wantErr string
	}{
		{name: "missing endpoint", metrics: "scrapeInterval: 15s", otlp: "headers: {}", wantErr: "otlp.endpoint"},
		{name: "missing interval", metrics: "", otlp: "endpoint: https://otel-collector:4317", wantErr: "metrics.scrapeInterval"},
		{name: "zero timeout", metrics: "scrapeInterval: 15s", otlp: "endpoint: https://otel-collector:4317\n  timeout: 0s", wantErr: "otlp.timeout"},
		{name: "scheme-less endpoint", metrics: "scrapeInterval: 15s", otlp: "endpoint: otel-collector:4317", wantErr: "otlp.endpoint must"},
		{name: "TLS with HTTP endpoint", metrics: "scrapeInterval: 15s", otlp: "endpoint: http://otel-collector:4317\n  tls: {}", wantErr: "otlp.tls cannot"},
		{name: "incomplete mTLS", metrics: "scrapeInterval: 15s", otlp: "endpoint: https://otel-collector:4317\n  tls:\n    certFile: client.crt", wantErr: "otlp.tls.certFile"},
		{name: "invalid TLS version", metrics: "scrapeInterval: 15s", otlp: "endpoint: https://otel-collector:4317\n  tls:\n    minVersion: TLS1.1", wantErr: "otlp.tls.minVersion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := writeExporterConfig(t, `
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
metrics:
  `+tt.metrics+`
otlp:
  `+tt.otlp)
			_, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func writeExporterConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}
	return path
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
