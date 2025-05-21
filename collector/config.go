// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"github.com/godror/godror/dsn"
	"github.com/oracle/oracle-db-appdev-monitoring/azvault"
	"github.com/oracle/oracle-db-appdev-monitoring/ocivault"
	"gopkg.in/yaml.v2"
	"log/slog"
	"maps"
	"os"
	"strings"
	"time"
)

type MetricsConfiguration struct {
	MetricsPath string                    `yaml:"metricsPath"`
	Databases   map[string]DatabaseConfig `yaml:"databases"`
	Metrics     MetricsFilesConfig        `yaml:"metrics"`
	Logging     LoggingConfig             `yaml:"log"`
}

type DatabaseConfig struct {
	Username      string
	Password      string
	URL           string `yaml:"url"`
	ConnectConfig `yaml:",inline"`
}

type ConnectConfig struct {
	Role               dsn.AdminRole
	TNSAdmin           string `yaml:"tnsAdmin"`
	ExternalAuth       bool   `yaml:"externalAuth"`
	MaxOpenConns       *int   `yaml:"maxOpenConns"`
	MaxIdleConns       *int   `yaml:"maxIdleConns"`
	PoolIncrement      *int   `yaml:"poolIncrement"`
	PoolMaxConnections *int   `yaml:"poolMaxConnections"`
	PoolMinConnections *int   `yaml:"poolMinConnections"`
	QueryTimeout       *int   `yaml:"queryTimeout"`
}

type MetricsFilesConfig struct {
	Default        string
	Custom         []string
	ScrapeInterval *time.Duration `yaml:"scrapeInterval"`
}

type LoggingConfig struct {
	LogDisable     *int           `yaml:"disable"`
	LogInterval    *time.Duration `yaml:"interval"`
	LogDestination string         `yaml:"destination"`
}

func (m *MetricsConfiguration) LogDestination() string {
	return m.Logging.LogDestination
}

func (m *MetricsConfiguration) LogInterval() time.Duration {
	return *m.Logging.LogInterval
}

func (m *MetricsConfiguration) LogDisable() int {
	return *m.Logging.LogDisable
}

func (m *MetricsConfiguration) ScrapeInterval() time.Duration {
	return *m.Metrics.ScrapeInterval
}

func (m *MetricsConfiguration) CustomMetricsFiles() []string {
	return m.Metrics.Custom
}

func (c ConnectConfig) GetMaxOpenConns() int {
	if c.MaxOpenConns == nil {
		return 10
	}
	return *c.MaxOpenConns
}

func (c ConnectConfig) GetMaxIdleConns() int {
	if c.MaxIdleConns == nil {
		return 10
	}
	return *c.MaxIdleConns
}

func (c ConnectConfig) GetPoolMaxConnections() int {
	if c.PoolMaxConnections == nil {
		return -1
	}
	return *c.PoolMaxConnections
}

func (c ConnectConfig) GetPoolMinConnections() int {
	if c.PoolMinConnections == nil {
		return -1
	}
	return *c.PoolMinConnections
}

func (c ConnectConfig) GetPoolIncrement() int {
	if c.PoolIncrement == nil {
		return -1
	}
	return *c.PoolIncrement
}

func (c ConnectConfig) GetQueryTimeout() int {
	if c.QueryTimeout == nil {
		return 5
	}
	return *c.QueryTimeout
}

func LoadMetricsConfiguration(logger *slog.Logger, cfg *Config, path string) (*MetricsConfiguration, error) {
	m := &MetricsConfiguration{}
	if len(cfg.ConfigFile) > 0 {
		content, err := os.ReadFile(cfg.ConfigFile)
		if err != nil {
			return m, err
		}
		expanded := os.ExpandEnv(string(content))
		if yerr := yaml.UnmarshalStrict([]byte(expanded), m); yerr != nil {
			return m, yerr
		}
	} else {
		m.Databases = make(map[string]DatabaseConfig)
		m.Databases["default"] = m.defaultDatabase(cfg)
	}

	m.merge(cfg, path)

	// TODO: rework vault support for multi-database.
	// Currently, the vault user/password is applied for every database.
	// It must be configurable at the database level for true multi-database support.
	m.setKeyVaultUserPassword(logger)
	return m, nil
}

func (m *MetricsConfiguration) merge(cfg *Config, path string) {
	if len(m.MetricsPath) == 0 {
		m.MetricsPath = path
	}
	m.mergeLoggingConfig(cfg)
	m.mergeMetricsConfig(cfg)
	if m.Metrics.ScrapeInterval == nil {
		m.Metrics.ScrapeInterval = &cfg.ScrapeInterval
	}
}

func (m *MetricsConfiguration) mergeLoggingConfig(cfg *Config) {
	if m.Logging.LogDisable == nil {
		m.Logging.LogDisable = cfg.LoggingConfig.LogDisable
	}
	if m.Logging.LogInterval == nil {
		m.Logging.LogInterval = cfg.LoggingConfig.LogInterval
	}
	if len(m.Logging.LogDestination) == 0 {
		m.Logging.LogDestination = cfg.LoggingConfig.LogDestination
	}
}

func (m *MetricsConfiguration) mergeMetricsConfig(cfg *Config) {
	if len(m.Metrics.Default) == 0 {
		m.Metrics.Default = cfg.DefaultMetricsFile
	}
	if len(m.Metrics.Custom) == 0 {
		m.Metrics.Custom = strings.Split(cfg.CustomMetrics, ",")
	}
}

func (m *MetricsConfiguration) defaultDatabase(cfg *Config) DatabaseConfig {
	return DatabaseConfig{
		Username: cfg.User,
		Password: cfg.Password,
		URL:      cfg.ConnectString,
		ConnectConfig: ConnectConfig{
			Role:               cfg.DbRole,
			TNSAdmin:           cfg.ConfigDir,
			ExternalAuth:       cfg.ExternalAuth,
			MaxOpenConns:       &cfg.MaxOpenConns,
			MaxIdleConns:       &cfg.MaxIdleConns,
			PoolIncrement:      &cfg.PoolIncrement,
			PoolMaxConnections: &cfg.PoolMaxConnections,
			PoolMinConnections: &cfg.PoolMinConnections,
			QueryTimeout:       &cfg.QueryTimeout,
		},
	}
}

func (m *MetricsConfiguration) setKeyVaultUserPassword(logger *slog.Logger) {
	if user, password, ok := getKeyVaultUserPassword(logger); ok {
		for dbname := range maps.Keys(m.Databases) {
			db := m.Databases[dbname]
			db.Password = password
			if len(user) > 0 {
				db.Username = user
			}
			m.Databases[dbname] = db
		}
	}
}

func getKeyVaultUserPassword(logger *slog.Logger) (user string, password string, ok bool) {
	ociVaultID, useOciVault := os.LookupEnv("OCI_VAULT_ID")
	if useOciVault {

		logger.Info("OCI_VAULT_ID env var is present so using OCI Vault", "vaultOCID", ociVaultID)
		password = ocivault.GetVaultSecret(ociVaultID, os.Getenv("OCI_VAULT_SECRET_NAME"))
		return "", password, true
	}

	azVaultID, useAzVault := os.LookupEnv("AZ_VAULT_ID")
	if useAzVault {

		logger.Info("AZ_VAULT_ID env var is present so using Azure Key Vault", "VaultID", azVaultID)
		logger.Info("Using the environment variables AZURE_TENANT_ID, AZURE_CLIENT_ID, and AZURE_CLIENT_SECRET to authentication with Azure.")
		user = azvault.GetVaultSecret(azVaultID, os.Getenv("AZ_VAULT_USERNAME_SECRET"))
		password = azvault.GetVaultSecret(azVaultID, os.Getenv("AZ_VAULT_PASSWORD_SECRET"))
		return user, password, true
	}
	return user, password, ok
}
