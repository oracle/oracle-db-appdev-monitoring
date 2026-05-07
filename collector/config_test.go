// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestLoadMetricsConfigurationRequiresConfigFile(t *testing.T) {
	_, err := LoadMetricsConfiguration(testLogger(), &Config{})
	if err == nil {
		t.Fatal("expected missing config file to fail")
	}
	if !strings.Contains(err.Error(), "config file is required") {
		t.Fatalf("expected required config file error, got %v", err)
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
