// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"github.com/godror/godror/dsn"
	"github.com/oracle/oracle-db-appdev-monitoring/azvault"
	"github.com/oracle/oracle-db-appdev-monitoring/ocivault"
	"gopkg.in/yaml.v2"
	"log/slog"
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
	Vault         *VaultConfig      `yaml:"vault,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
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

type VaultConfig struct {
	// OCI if present, OCI vault will be used to load username and/or password.
	OCI *OCIVault `yaml:"oci"`
	// Azure if present, Azure vault will be used to load username and/or password.
	Azure *AZVault `yaml:"azure"`
}

type OCIVault struct {
	ID             string `yaml:"id"`
	UsernameSecret string `yaml:"usernameSecret"`
	PasswordSecret string `yaml:"passwordSecret"`
}

type AZVault struct {
	ID             string `yaml:"id"`
	UsernameSecret string `yaml:"usernameSecret"`
	PasswordSecret string `yaml:"passwordSecret"`
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

func (d DatabaseConfig) GetUsername() string {

	if d.Vault.OCI.UsernameSecret != "" {
		return ocivault.GetVaultSecret(d.Vault.OCI.ID, d.Vault.OCI.UsernameSecret)
	}
	if d.Vault.Azure.UsernameSecret != "" {
		return azvault.GetVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.UsernameSecret)
	}
	return d.Username
}

func (d DatabaseConfig) GetPassword() string {

	if d.Vault.OCI.PasswordSecret != "" {
		return ocivault.GetVaultSecret(d.Vault.OCI.ID, d.Vault.OCI.PasswordSecret)
	}
	if d.Vault.Azure.PasswordSecret != "" {
		return azvault.GetVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.PasswordSecret)
	}
	return d.Password
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
		logger.Warn("Configuring default database from CLI parameters is deprecated. Use of the '--config.file' argument is preferred. See https://github.com/oracle/oracle-db-appdev-monitoring?tab=readme-ov-file#standalone-binary")
		m.Databases = make(map[string]DatabaseConfig)
		m.Databases["default"] = m.defaultDatabase(cfg)
	}

	m.merge(cfg, path)
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

// defaultDatabase creates a database named "default" if CLI arguments are used. It is for backwards compatibility when the exporter
// was only configurable through CLI arguments for a single database instance.
func (m *MetricsConfiguration) defaultDatabase(cfg *Config) DatabaseConfig {
	dbconfig := DatabaseConfig{
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
	// Vault ID lookup through environment variables is the historic method of loading vault metadata.
	// These semantics are preserved if the "default" database from CLI config is requested.
	if ociVaultID, useOciVault := os.LookupEnv("OCI_VAULT_ID"); useOciVault {
		dbconfig.Vault = &VaultConfig{
			OCI: &OCIVault{
				ID:             ociVaultID,
				UsernameSecret: os.Getenv("OCI_VAULT_USERNAME_SECRET"),
				PasswordSecret: os.Getenv("OCI_VAULT_PASSWORD_SECRET"),
			},
		}
	} else if azVaultID, useAzVault := os.LookupEnv("AZ_VAULT_ID"); useAzVault {
		dbconfig.Vault = &VaultConfig{
			Azure: &AZVault{
				ID:             azVaultID,
				UsernameSecret: os.Getenv("AZ_VAULT_USERNAME_SECRET"),
				PasswordSecret: os.Getenv("AZ_VAULT_PASSWORD_SECRET"),
			},
		}
	}
	return dbconfig
}
