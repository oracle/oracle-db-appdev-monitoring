// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"fmt"
	"github.com/oracle/oracle-db-appdev-monitoring/azvault"
	"github.com/oracle/oracle-db-appdev-monitoring/hashivault"
	"github.com/oracle/oracle-db-appdev-monitoring/ocivault"
	"github.com/prometheus/exporter-toolkit/web"
	"gopkg.in/yaml.v2"
	"log/slog"
	"os"
	"strings"
	"time"
)

type MetricsConfiguration struct {
	ListenAddress string                    `yaml:"listenAddress"`
	MetricsPath   string                    `yaml:"metricsPath"`
	Databases     map[string]DatabaseConfig `yaml:"databases"`
	Metrics       MetricsFilesConfig        `yaml:"metrics"`
	Logging       LoggingConfig             `yaml:"log"`
	Web           WebConfig                 `yaml:"web"`
}

type WebConfig struct {
	ListenAddresses *[]string `yaml:"listenAddresses"`
	SystemdSocket   *bool     `yaml:"systemdSocket"`
	ConfigFile      *string   `yaml:"configFile"`
}

type DatabaseConfig struct {
	Username      string
	Password      string
	PasswordFile  string `yaml:"passwordFile"`
	URL           string `yaml:"url"`
	ConnectConfig `yaml:",inline"`
	Vault         *VaultConfig      `yaml:"vault,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
}

type ConnectConfig struct {
	Role               string
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
	// HashiCorp Vault if present. HashiCorp Vault will be used to fetch database credentials.
	HashiCorp *HashiCorpVault `yaml:"hashicorp"`
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

type HashiCorpVault struct {
	Socket       string `yaml:"proxySocket"`
	MountType    string `yaml:"mountType"`
	MountName    string `yaml:"mountName"`
	SecretPath   string `yaml:"secretPath"`
	UsernameAttr string `yaml:"usernameAttribute"`
	PasswordAttr string `yaml:"passwordAttribute"`
	AsProxy      string `yaml:"useAsProxyFor"`
	// Private to avoid making multiple calls
	fetchedSecert map[string]string
}

type MetricsFilesConfig struct {
	DatabaseLabel     string `yaml:"databaseLabel"`
	Default           string
	Custom            []string
	ScrapeInterval    *time.Duration `yaml:"scrapeInterval"`
	ConnectionBackoff *time.Duration `yaml:"connectionBackoff"`
}

type LoggingConfig struct {
	LogDisable     *int           `yaml:"disable"`
	LogInterval    *time.Duration `yaml:"interval"`
	LogDestination string         `yaml:"destination"`
}

func (m *MetricsConfiguration) DatabaseLabel() string {
	if len(m.Metrics.DatabaseLabel) == 0 {
		return "database"
	}
	return m.Metrics.DatabaseLabel
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

func (h HashiCorpVault) GetUsernameAttr() string {
	if h.UsernameAttr == "" || h.MountType == hashivault.MountTypeDatabase {
		return "username"
	}
	return h.UsernameAttr
}

func (h HashiCorpVault) GetPasswordAttr() string {
	if h.PasswordAttr == "" || h.MountType == hashivault.MountTypeDatabase {
		return "password"
	}
	return h.PasswordAttr
}

func (d DatabaseConfig) fetchHashiCorpVaultSecret() {
	if len(d.Vault.HashiCorp.fetchedSecert) > 0 {
		// Secret is already fetched, do nothing
		return
	}
	vc := hashivault.CreateVaultClient(slog.Default(), d.Vault.HashiCorp.Socket)
	// Set default username and password attribute values
	requiredKeys := []string{d.Vault.HashiCorp.GetUsernameAttr(), d.Vault.HashiCorp.GetPasswordAttr()}
	d.Vault.HashiCorp.fetchedSecert = vc.GetVaultSecret(d.Vault.HashiCorp.MountType, d.Vault.HashiCorp.MountName, d.Vault.HashiCorp.SecretPath, requiredKeys)
}

func (d DatabaseConfig) GetUsername() string {
	if d.isOCIVault() && d.Vault.OCI.UsernameSecret != "" {
		return ocivault.GetVaultSecret(d.Vault.OCI.ID, d.Vault.OCI.UsernameSecret)
	}
	if d.isAzureVault() && d.Vault.Azure.UsernameSecret != "" {
		return azvault.GetVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.UsernameSecret)
	}
	if d.isHashiCorpVault() && d.Vault.HashiCorp.MountType != "" && d.Vault.HashiCorp.MountName != "" && d.Vault.HashiCorp.SecretPath != "" {
		d.fetchHashiCorpVaultSecret()
		userName := d.Vault.HashiCorp.fetchedSecert[d.Vault.HashiCorp.GetUsernameAttr()]
		if d.Vault.HashiCorp.AsProxy == "" {
			return userName
		} else {
			return fmt.Sprintf("%s[%s]", userName, d.Vault.HashiCorp.AsProxy)
		}
	}
	return d.Username
}

func (d DatabaseConfig) GetPassword() string {
	if d.PasswordFile != "" {
		bytes, err := os.ReadFile(d.PasswordFile)
		if err != nil {
			// If there is an invalid file, exporter cannot continue processing.
			panic(fmt.Errorf("failed to read password file: %v", err))
		}
		return string(bytes)
	}
	if d.isOCIVault() && d.Vault.OCI.PasswordSecret != "" {
		return ocivault.GetVaultSecret(d.Vault.OCI.ID, d.Vault.OCI.PasswordSecret)
	}
	if d.isAzureVault() && d.Vault.Azure.PasswordSecret != "" {
		return azvault.GetVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.PasswordSecret)
	}
	if d.isHashiCorpVault() && d.Vault.HashiCorp.MountType != "" && d.Vault.HashiCorp.MountName != "" && d.Vault.HashiCorp.SecretPath != "" {
		d.fetchHashiCorpVaultSecret()
		return d.Vault.HashiCorp.fetchedSecert[d.Vault.HashiCorp.GetPasswordAttr()]
	}
	return d.Password
}

func (d DatabaseConfig) isOCIVault() bool {
	return d.Vault != nil && d.Vault.OCI != nil
}

func (d DatabaseConfig) isAzureVault() bool {
	return d.Vault != nil && d.Vault.Azure != nil
}

func (d DatabaseConfig) isHashiCorpVault() bool {
	return d.Vault != nil && d.Vault.HashiCorp != nil
}

func LoadMetricsConfiguration(logger *slog.Logger, cfg *Config, path string, flags *web.FlagConfig) (*MetricsConfiguration, error) {
	m := &MetricsConfiguration{}
	if len(cfg.ConfigFile) > 0 {
		content, err := os.ReadFile(cfg.ConfigFile)
		if err != nil {
			return m, err
		}
		expanded := os.Expand(string(content), func(s string) string {
			// allows escaping literal $ characters
			if s == "$" {
				return "$"
			}
			return os.Getenv(s)
		})
		if yerr := yaml.UnmarshalStrict([]byte(expanded), m); yerr != nil {
			return m, yerr
		}
	} else {
		logger.Warn("Configuring default database from CLI parameters is deprecated. Use of the '--config.file' argument is preferred. See https://oracle.github.io/oracle-db-appdev-monitoring/docs/getting-started/basics#standalone-binary")
		m.Databases = make(map[string]DatabaseConfig)
		m.Databases["default"] = m.defaultDatabase(cfg)
	}

	m.merge(cfg, path, flags)
	return m, m.validate(logger)
}

func (wc WebConfig) Flags() *web.FlagConfig {
	return &web.FlagConfig{
		WebListenAddresses: wc.ListenAddresses,
		WebSystemdSocket:   wc.SystemdSocket,
		WebConfigFile:      wc.ConfigFile,
	}
}

func (m *MetricsConfiguration) merge(cfg *Config, path string, flags *web.FlagConfig) {
	if len(m.MetricsPath) == 0 {
		m.MetricsPath = path
	}
	m.mergeWebConfig(flags)
	m.mergeLoggingConfig(cfg)
	m.mergeMetricsConfig(cfg)
	if m.Metrics.ScrapeInterval == nil {
		m.Metrics.ScrapeInterval = &cfg.ScrapeInterval
	}
}

func (m *MetricsConfiguration) mergeWebConfig(flags *web.FlagConfig) {
	if m.Web.ListenAddresses == nil {
		m.Web.ListenAddresses = flags.WebListenAddresses
	}
	if m.Web.SystemdSocket == nil {
		m.Web.SystemdSocket = flags.WebSystemdSocket
	}
	if m.Web.ConfigFile == nil {
		m.Web.ConfigFile = flags.WebConfigFile
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

func (m *MetricsConfiguration) validate(logger *slog.Logger) error {
	m.checkDuplicatedDatabases(logger)
	return nil
}

// checkDuplicatedDatabases validates duplicated databases. If a database entry is duplicated, log a warning.
func (m *MetricsConfiguration) checkDuplicatedDatabases(logger *slog.Logger) {
	type dbkey struct {
		URL      string
		Username string
	}

	dbs := map[dbkey][]string{}
	for db, cfg := range m.Databases {
		key := dbkey{
			URL:      cfg.URL,
			Username: strings.ToLower(cfg.Username),
		}
		dbs[key] = append(dbs[key], db)
	}

	for _, v := range dbs {
		if len(v) > 1 {
			logger.Warn("duplicated database connections", "database connections", strings.Join(v, ", "), "count", len(v))
		}
	}
}

func (m *MetricsConfiguration) ConnectionBackoff() time.Duration {
	if m.Metrics.ConnectionBackoff == nil {
		return 5 * time.Minute
	}
	return *m.Metrics.ConnectionBackoff
}
