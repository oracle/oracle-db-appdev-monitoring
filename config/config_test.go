// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

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

	"github.com/oracle/oracle-db-appdev-monitoring/v2/oci"
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
	if cfg.PerMetricScrapeDurationEnabled() {
		t.Fatal("expected the per-metric scrape duration metric to be disabled by default")
	}
}

func TestLoadMetricsConfigurationReadsPerMetricScrapeDuration(t *testing.T) {
	tests := []struct {
		name    string
		metrics string
		want    bool
	}{
		{
			name: "enabled",
			metrics: `
metrics:
  perMetricScrapeDuration:
    enabled: true
`,
			want: true,
		},
		{
			name: "explicitly disabled",
			metrics: `
metrics:
  perMetricScrapeDuration:
    enabled: false
`,
			want: false,
		},
		{
			name:    "not configured",
			metrics: "",
			want:    false,
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
`+tt.metrics)

			cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath})
			if err != nil {
				t.Fatalf("expected config to load, got %v", err)
			}
			if got := cfg.PerMetricScrapeDurationEnabled(); got != tt.want {
				t.Fatalf("expected PerMetricScrapeDurationEnabled() to be %t, got %t", tt.want, got)
			}
		})
	}
}

// The config is parsed with yaml.UnmarshalStrict, so a misspelled key must not be silently ignored.
func TestLoadMetricsConfigurationRejectsUnknownMetricsKey(t *testing.T) {
	configPath := writeExporterConfig(t, `
databases:
  default:
    username: scott
    password: tiger
    url: localhost:1521/freepdb1
metrics:
  perMetricScrapeDurations:
    enabled: true
`)

	if _, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: configPath}); err == nil {
		t.Fatal("expected a misspelled metrics key to be rejected")
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

func TestMetricsNormalizeIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, metrics *Metrics)
	}{
		{
			name: "metrics desc key",
			check: func(t *testing.T, metrics *Metrics) {
				if _, ok := metrics.Metric[0].MetricsDesc["sqlid_without_profile_on_wcr_pta_multi_deep_bin_v"]; !ok {
					t.Fatal("expected metricsdesc key to be normalized to lowercase")
				}
			},
		},
		{
			name: "metrics bucket key",
			check: func(t *testing.T, metrics *Metrics) {
				if _, ok := metrics.Metric[0].MetricsBuckets["sqlid_without_profile_on_wcr_pta_multi_deep_bin_v"]; !ok {
					t.Fatal("expected metricsbuckets key to be normalized to lowercase")
				}
			},
		},
		{
			name: "metrics bucket field key",
			check: func(t *testing.T, metrics *Metrics) {
				if _, ok := metrics.Metric[0].MetricsBuckets["sqlid_without_profile_on_wcr_pta_multi_deep_bin_v"]["bucket_1"]; !ok {
					t.Fatal("expected histogram bucket field key to be normalized to lowercase")
				}
			},
		},
		{
			name: "field to append",
			check: func(t *testing.T, metrics *Metrics) {
				if metrics.Metric[0].FieldToAppend != "sql_id" {
					t.Fatalf("expected fieldtoappend to be normalized to lowercase, got %q", metrics.Metric[0].FieldToAppend)
				}
			},
		},
		{
			name: "labels",
			check: func(t *testing.T, metrics *Metrics) {
				if metrics.Metric[0].Labels[0] != "sql_id" || metrics.Metric[0].Labels[1] != "inst_id" {
					t.Fatalf("expected labels to be normalized to lowercase, got %v", metrics.Metric[0].Labels)
				}
			},
		},
		{
			name: "all loaded metrics",
			check: func(t *testing.T, metrics *Metrics) {
				if metrics.Metric[1].FieldToAppend != "db_name" {
					t.Fatalf("expected second metric fieldtoappend to be normalized to lowercase, got %q", metrics.Metric[1].FieldToAppend)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &Metrics{
				Metric: []*Metric{
					{
						Labels:        []string{"SQL_ID", "Inst_ID"},
						FieldToAppend: "SQL_ID",
						MetricsDesc: map[string]string{
							"sqlid_without_profile_on_WCR_PTA_MULTI_DEEP_BIN_V": "test metric",
						},
						MetricsType: map[string]string{
							"sqlid_without_profile_on_WCR_PTA_MULTI_DEEP_BIN_V": "histogram",
						},
						MetricsBuckets: map[string]map[string]string{
							"sqlid_without_profile_on_WCR_PTA_MULTI_DEEP_BIN_V": {
								"Bucket_1": "1",
							},
						},
					},
					{
						Labels:        []string{"DB_NAME"},
						FieldToAppend: "DB_NAME",
					},
				},
			}

			metrics.normalizeIdentifiers()
			tt.check(t, metrics)
		})
	}
}

func TestMetricNormalizeIdentifiersSetsDeterministicID(t *testing.T) {
	metric := &Metric{
		Context: "sessions",
		MetricsDesc: map[string]string{
			"B": "second",
			"a": "first",
		},
	}

	metric.normalizeIdentifiers()

	if metric.ID != "sessions_a_b" {
		t.Fatalf("expected normalized metric ID %q, got %q", "sessions_a_b", metric.ID)
	}
}

func TestMetricsToMapUsesStableMetricID(t *testing.T) {
	firstDesc := make(map[string]string, 2)
	firstDesc["b"] = "second"
	firstDesc["a"] = "first"

	secondDesc := make(map[string]string, 2)
	secondDesc["a"] = "first"
	secondDesc["b"] = "second"

	metrics := Metrics{
		Metric: []*Metric{
			{
				Context:     "sessions",
				MetricsDesc: firstDesc,
				Request:     "first",
			},
			{
				Context:     "sessions",
				MetricsDesc: secondDesc,
				Request:     "second",
			},
		},
	}

	metrics.normalizeIdentifiers()
	got := metrics.toMap()

	if len(got) != 1 {
		t.Fatalf("expected one merged metric, got %d", len(got))
	}
	if got["sessions_a_b"] == nil {
		t.Fatalf("expected merged metric keyed by %q", "sessions_a_b")
	}
	if got["sessions_a_b"].Request != "second" {
		t.Fatalf("expected later metric to overwrite existing entry, got request %q", got["sessions_a_b"].Request)
	}
}

func TestDefaultMetricsAssignsIDs(t *testing.T) {
	metrics := DefaultMetrics(testLogger(), MetricsFilesConfig{})

	if len(metrics) == 0 {
		t.Fatal("expected embedded default metrics to load")
	}

	for id, metric := range metrics {
		if id == "" {
			t.Fatal("expected default metric map key to be non-empty")
		}
		if metric == nil {
			t.Fatalf("expected metric for ID %q", id)
		}
		if metric.ID != id {
			t.Fatalf("expected metric ID %q to match map key, got %q", id, metric.ID)
		}
	}
}

func TestMetricGetLabels(t *testing.T) {
	tests := []struct {
		name     string
		metric   Metric
		expected []string
	}{
		{
			name: "returns all labels when field to append is empty",
			metric: Metric{
				Labels: []string{"database", "instance"},
			},
			expected: []string{"database", "instance"},
		},
		{
			name: "omits field to append from labels",
			metric: Metric{
				Labels:        []string{"database", "instance", "sql_id"},
				FieldToAppend: "sql_id",
			},
			expected: []string{"database", "instance"},
		},
		{
			name: "returns labels unchanged when field to append is not present",
			metric: Metric{
				Labels:        []string{"database", "instance"},
				FieldToAppend: "sql_id",
			},
			expected: []string{"database", "instance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metric.GetLabels()
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d labels, got %d (%v)", len(tt.expected), len(got), got)
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Fatalf("expected labels %v, got %v", tt.expected, got)
				}
			}
		})
	}
}

func TestMetricIsEnabledForDatabase(t *testing.T) {
	tests := []struct {
		name     string
		metric   Metric
		database string
		expected bool
	}{
		{
			name:     "enabled for all databases when databases is nil",
			metric:   Metric{Databases: nil},
			database: "prod",
			expected: true,
		},
		{
			name:     "enabled when database is listed",
			metric:   Metric{Databases: []string{"prod", "staging"}},
			database: "prod",
			expected: true,
		},
		{
			name:     "disabled when database is not listed",
			metric:   Metric{Databases: []string{"staging"}},
			database: "prod",
			expected: false,
		},
		{
			name:     "disabled for all databases when list is empty but non nil",
			metric:   Metric{Databases: []string{}},
			database: "prod",
			expected: false,
		},
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			got := tests[i].metric.IsEnabledForDatabase(tests[i].database)
			if got != tests[i].expected {
				t.Fatalf("expected %v, got %v", tests[i].expected, got)
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
