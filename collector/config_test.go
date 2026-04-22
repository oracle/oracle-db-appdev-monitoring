// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/exporter-toolkit/web"
)

func TestMetricsConfigurationHelperDefaults(t *testing.T) {
	cfg := &MetricsConfiguration{
		Metrics: MetricsFilesConfig{},
		Logging: LoggingConfig{
			LogDisable:     ptr(0),
			LogInterval:    ptr(15 * time.Second),
			LogDestination: "/tmp/alert.log",
		},
	}

	if got := cfg.DatabaseLabel(); got != "database" {
		t.Fatalf("expected default database label, got %q", got)
	}
	if got := cfg.LogPerDatabaseFiles(); got {
		t.Fatalf("expected perDatabaseFiles to default false")
	}
	if got := cfg.ConnectionBackoff(); got != 5*time.Minute {
		t.Fatalf("expected default backoff 5m, got %v", got)
	}
	if got := (ConnectConfig{}).GetMaxOpenConns(); got != 10 {
		t.Fatalf("expected default max open conns 10, got %d", got)
	}
	if got := (ConnectConfig{}).GetMaxIdleConns(); got != 10 {
		t.Fatalf("expected default max idle conns 10, got %d", got)
	}
	if got := (ConnectConfig{}).GetPoolMaxConnections(); got != -1 {
		t.Fatalf("expected default pool max -1, got %d", got)
	}
	if got := (ConnectConfig{}).GetPoolMinConnections(); got != -1 {
		t.Fatalf("expected default pool min -1, got %d", got)
	}
	if got := (ConnectConfig{}).GetPoolIncrement(); got != -1 {
		t.Fatalf("expected default pool increment -1, got %d", got)
	}
	if got := (ConnectConfig{}).GetQueryTimeout(); got != 5 {
		t.Fatalf("expected default query timeout 5, got %d", got)
	}
}

func TestHashiCorpVaultAttributeDefaults(t *testing.T) {
	tests := []struct {
		name     string
		vault    HashiCorpVault
		username string
		password string
	}{
		{
			name:     "defaults to username password",
			vault:    HashiCorpVault{},
			username: "username",
			password: "password",
		},
		{
			name: "database mount forces database attrs",
			vault: HashiCorpVault{
				MountType:    "database",
				UsernameAttr: "custom-user",
				PasswordAttr: "custom-pass",
			},
			username: "username",
			password: "password",
		},
		{
			name: "custom attrs used for kv",
			vault: HashiCorpVault{
				MountType:    "kv",
				UsernameAttr: "db_user",
				PasswordAttr: "db_pass",
			},
			username: "db_user",
			password: "db_pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.vault.GetUsernameAttr(); got != tt.username {
				t.Fatalf("expected username attr %q, got %q", tt.username, got)
			}
			if got := tt.vault.GetPasswordAttr(); got != tt.password {
				t.Fatalf("expected password attr %q, got %q", tt.password, got)
			}
		})
	}
}

func TestDatabaseConfigCredentialHelpers(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "password.txt")
	if err := os.WriteFile(passwordFile, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}

	config := DatabaseConfig{
		Username:     "user",
		Password:     "literal",
		PasswordFile: passwordFile,
	}

	if got := config.GetUsername(); got != "user" {
		t.Fatalf("expected username user, got %q", got)
	}
	if got := config.GetPassword(); got != "from-file" {
		t.Fatalf("expected password from file, got %q", got)
	}
	if config.isOCIVault() || config.isAzureVault() || config.isHashiCorpVault() {
		t.Fatal("expected vault helpers to return false when no vault is configured")
	}
}

func TestMetricsConfigurationMergeUsesCLIAndFlagsFallbacks(t *testing.T) {
	cfg := &MetricsConfiguration{}
	flags := &web.FlagConfig{
		WebListenAddresses: ptr([]string{":9162"}),
		WebSystemdSocket:   ptr(true),
		WebConfigFile:      ptr("web-config.yml"),
	}
	input := &Config{
		DefaultMetricsFile: "default-metrics.toml",
		CustomMetrics:      "a.toml,b.toml",
		ScrapeInterval:     30 * time.Second,
		LoggingConfig: LoggingConfig{
			LogDisable:          ptr(1),
			LogInterval:         ptr(45 * time.Second),
			LogDestination:      "/tmp/alert.log",
			LogPerDatabaseFiles: ptr(true),
		},
	}

	cfg.merge(input, "/metrics", flags)

	if cfg.MetricsPath != "/metrics" {
		t.Fatalf("expected metrics path /metrics, got %q", cfg.MetricsPath)
	}
	if got := cfg.LogDestination(); got != "/tmp/alert.log" {
		t.Fatalf("expected log destination /tmp/alert.log, got %q", got)
	}
	if got := cfg.LogInterval(); got != 45*time.Second {
		t.Fatalf("expected log interval 45s, got %v", got)
	}
	if got := cfg.LogDisable(); got != 1 {
		t.Fatalf("expected log disable 1, got %d", got)
	}
	if !cfg.LogPerDatabaseFiles() {
		t.Fatal("expected perDatabaseFiles to be merged from CLI config")
	}
	if got := cfg.ScrapeInterval(); got != 30*time.Second {
		t.Fatalf("expected scrape interval 30s, got %v", got)
	}
	if got := cfg.CustomMetricsFiles(); len(got) != 2 || got[0] != "a.toml" || got[1] != "b.toml" {
		t.Fatalf("expected custom metrics split from CLI config, got %v", got)
	}
	if got := cfg.Web.Flags(); got == nil || got.WebConfigFile == nil || *got.WebConfigFile != "web-config.yml" {
		t.Fatalf("expected merged web config flags, got %#v", got)
	}
}

func TestLoadMetricsConfigurationFromFileExpandsEnvAndMergesDefaults(t *testing.T) {
	t.Setenv("CFG_USER", "scott")
	t.Setenv("CFG_PASSWORD", "tiger")

	configPath := filepath.Join(t.TempDir(), "config.yml")
	content := `
metricsPath: /custom
databases:
  prod:
    username: ${CFG_USER}
    password: ${CFG_PASSWORD}
    url: dbhost/service
metrics:
  databaseLabel: db_name
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg := &Config{
		ConfigFile:         configPath,
		DefaultMetricsFile: "default-metrics.toml",
		QueryTimeout:       7,
		ScrapeInterval:     20 * time.Second,
		LoggingConfig: LoggingConfig{
			LogDisable:     ptr(0),
			LogInterval:    ptr(15 * time.Second),
			LogDestination: "/tmp/alert.log",
		},
	}
	flags := &web.FlagConfig{
		WebListenAddresses: ptr([]string{":9161"}),
		WebSystemdSocket:   ptr(false),
	}

	got, err := LoadMetricsConfiguration(testLogger(), cfg, "/metrics", flags)
	if err != nil {
		t.Fatalf("load metrics config: %v", err)
	}

	db := got.Databases["prod"]
	if db.Username != "scott" || db.Password != "tiger" {
		t.Fatalf("expected expanded credentials, got username=%q password=%q", db.Username, db.Password)
	}
	if got.DatabaseLabel() != "db_name" {
		t.Fatalf("expected configured database label, got %q", got.DatabaseLabel())
	}
	if got.MetricsPath != "/custom" {
		t.Fatalf("expected metricsPath from file, got %q", got.MetricsPath)
	}
	if got.Web.ListenAddresses == nil || len(*got.Web.ListenAddresses) != 1 || (*got.Web.ListenAddresses)[0] != ":9161" {
		t.Fatalf("expected merged web listen address, got %#v", got.Web.ListenAddresses)
	}
}

func TestLoadMetricsConfigurationWithoutFileUsesDefaultDatabaseAndVaultEnv(t *testing.T) {
	t.Setenv("OCI_VAULT_ID", "vault-id")
	t.Setenv("OCI_VAULT_USERNAME_SECRET", "user-secret")
	t.Setenv("OCI_VAULT_PASSWORD_SECRET", "pass-secret")

	cfg := &Config{
		User:               "cli-user",
		Password:           "cli-pass",
		ConnectString:      "db/service",
		DbRole:             "SYSDBA",
		ConfigDir:          "/wallet",
		MaxOpenConns:       12,
		MaxIdleConns:       8,
		PoolIncrement:      2,
		PoolMaxConnections: 20,
		PoolMinConnections: 5,
		QueryTimeout:       9,
		DefaultMetricsFile: "default-metrics.toml",
		ScrapeInterval:     0,
		LoggingConfig: LoggingConfig{
			LogDisable:     ptr(0),
			LogInterval:    ptr(10 * time.Second),
			LogDestination: "/tmp/alert.log",
		},
	}

	got, err := LoadMetricsConfiguration(testLogger(), cfg, "/metrics", &web.FlagConfig{})
	if err != nil {
		t.Fatalf("load metrics config: %v", err)
	}

	db := got.Databases["default"]
	if db.Username != "cli-user" || db.Password != "cli-pass" {
		t.Fatalf("expected CLI credentials, got %#v", db)
	}
	if db.Vault == nil || db.Vault.OCI == nil || db.Vault.OCI.ID != "vault-id" {
		t.Fatalf("expected OCI vault config from env, got %#v", db.Vault)
	}
	if db.GetQueryTimeout() != 9 {
		t.Fatalf("expected query timeout 9, got %d", db.GetQueryTimeout())
	}
}

func TestLoadMetricsConfigurationRejectsUnknownField(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(configPath, []byte("unknownField: true\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err := LoadMetricsConfiguration(testLogger(), &Config{
		ConfigFile:         configPath,
		DefaultMetricsFile: "default-metrics.toml",
		LoggingConfig: LoggingConfig{
			LogDisable:     ptr(0),
			LogInterval:    ptr(10 * time.Second),
			LogDestination: "/tmp/alert.log",
		},
	}, "/metrics", &web.FlagConfig{})
	if err == nil {
		t.Fatal("expected strict yaml error")
	}
}

func TestCheckDuplicatedDatabasesLogsWarning(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg := &MetricsConfiguration{
		Databases: map[string]DatabaseConfig{
			"db1": {Username: "SCOTT", URL: "db/service"},
			"db2": {Username: "scott", URL: "db/service"},
		},
	}

	cfg.checkDuplicatedDatabases(logger)

	if !strings.Contains(logs.String(), "duplicated database connections") {
		t.Fatalf("expected duplicate database warning, got %q", logs.String())
	}
}
