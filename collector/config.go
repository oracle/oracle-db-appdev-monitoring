// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/azvault"
	"github.com/oracle/oracle-db-appdev-monitoring/hashivault"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
	"github.com/oracle/oracle-db-appdev-monitoring/ocivault"
	"github.com/prometheus/exporter-toolkit/web"
	"go.yaml.in/yaml/v2"
)

var (
	getOCIVaultSecret       = ocivault.GetVaultSecret
	getAZVaultSecret        = azvault.GetVaultSecret
	getHashiCorpVaultSecret = func(logger *slog.Logger, cfg *HashiCorpVault, requiredKeys []string) (map[string]string, error) {
		client, err := hashivault.CreateVaultClient(logger, cfg.Socket)
		if err != nil {
			return nil, err
		}
		return client.GetVaultSecret(cfg.MountType, cfg.MountName, cfg.SecretPath, requiredKeys)
	}
)

type MetricsConfiguration struct {
	ListenAddress string                    `yaml:"listenAddress"`
	MetricsPath   string                    `yaml:"metricsPath"`
	Databases     map[string]DatabaseConfig `yaml:"databases"`
	Metrics       MetricsFilesConfig        `yaml:"metrics"`
	Logging       LoggingConfig             `yaml:"log"`
	Web           WebConfig                 `yaml:"web"`
	OTLP          *OTLPConfig               `yaml:"otlp"`
	DatabasesFrom *DatabasesFromConfig      `yaml:"databasesFrom"`
}

// DatabasesFromConfig configures database discovery sources.
type DatabasesFromConfig struct {
	OCI *OCIDatabasesFromConfig `yaml:"oci"`
}

// OCIDatabasesFromConfig configures OCI database discovery.
type OCIDatabasesFromConfig struct {
	Auth         oci.AuthMode                  `yaml:"auth"`
	Compartments []string                      `yaml:"compartments"`
	Filters      OCIDatabasesFromFiltersConfig `yaml:"filters"`
}

// OCIDatabasesFromFiltersConfig narrows the databases discovered from OCI.
type OCIDatabasesFromFiltersConfig struct {
	LifecycleState adb.DatabaseLifecycleStateEnum `yaml:"lifecycleState"`
	RequiredTags   map[string]string              `yaml:"requiredTags"`
}

// OTLPConfig configures scheduled metric publishing to an OTLP/gRPC endpoint.
// The endpoint is required whenever this section is present. HTTP and HTTPS
// endpoint URLs select plaintext and TLS transport respectively.
type OTLPConfig struct {
	Endpoint           string            `yaml:"endpoint"`
	Timeout            *time.Duration    `yaml:"timeout"`
	Headers            map[string]string `yaml:"headers"`
	ResourceAttributes map[string]string `yaml:"resourceAttributes"`
	TLS                *OTLPTLSConfig    `yaml:"tls"`
}

// OTLPTLSConfig configures transport security for the outbound OTLP/gRPC client.
type OTLPTLSConfig struct {
	CAFile             string `yaml:"caFile"`
	CertFile           string `yaml:"certFile"`
	KeyFile            string `yaml:"keyFile"`
	ServerName         string `yaml:"serverName"`
	MinVersion         string `yaml:"minVersion"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

type WebConfig struct {
	ListenAddresses   *[]string      `yaml:"listenAddresses"`
	SystemdSocket     *bool          `yaml:"systemdSocket"`
	ConfigFile        *string        `yaml:"configFile"`
	ReadHeaderTimeout *time.Duration `yaml:"readHeaderTimeout"`
	ReadTimeout       *time.Duration `yaml:"readTimeout"`
	IdleTimeout       *time.Duration `yaml:"idleTimeout"`
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
	TNSAdmin           string         `yaml:"tnsAdmin"`
	ExternalAuth       bool           `yaml:"externalAuth"`
	ConnMaxLifetime    *time.Duration `yaml:"connMaxLifetime"`
	MaxOpenConns       *int           `yaml:"maxOpenConns"`
	MaxIdleConns       *int           `yaml:"maxIdleConns"`
	PoolIncrement      *int           `yaml:"poolIncrement"`
	PoolMaxConnections *int           `yaml:"poolMaxConnections"`
	PoolMinConnections *int           `yaml:"poolMinConnections"`
	QueryTimeout       *int           `yaml:"queryTimeout"`
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
	ID             string       `yaml:"id"`
	Auth           oci.AuthMode `yaml:"auth,omitempty"`
	UsernameSecret string       `yaml:"usernameSecret"`
	PasswordSecret string       `yaml:"passwordSecret"`
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
	LogLevelEnabled     *bool          `yaml:"messageLevelEnabled"`
	LogDisable          *int           `yaml:"disable"`
	LogInterval         *time.Duration `yaml:"interval"`
	LogDestination      string         `yaml:"destination"`
	LogPerDatabaseFiles *bool          `yaml:"perDatabaseFiles"`
	Level               string         `yaml:"level"`
	Format              string         `yaml:"format"`
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

func (m *MetricsConfiguration) LogMessageLevelEnabled() bool {
	return *m.Logging.LogLevelEnabled
}

func (m *MetricsConfiguration) LogPerDatabaseFiles() bool {
	if m.Logging.LogPerDatabaseFiles == nil {
		return false
	}
	return *m.Logging.LogPerDatabaseFiles
}

func (m *MetricsConfiguration) ScrapeInterval() time.Duration {
	return *m.Metrics.ScrapeInterval
}

func (wc WebConfig) GetReadHeaderTimeout() time.Duration {
	if wc.ReadHeaderTimeout == nil {
		return 10 * time.Second
	}
	return *wc.ReadHeaderTimeout
}

func (wc WebConfig) GetReadTimeout() time.Duration {
	if wc.ReadTimeout == nil {
		return 30 * time.Second
	}
	return *wc.ReadTimeout
}

func (wc WebConfig) GetIdleTimeout() time.Duration {
	if wc.IdleTimeout == nil {
		return 120 * time.Second
	}
	return *wc.IdleTimeout
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

func (c ConnectConfig) GetConnMaxLifetime() time.Duration {
	if c.ConnMaxLifetime == nil {
		return 30 * time.Minute
	}
	return *c.ConnMaxLifetime
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

func (d DatabaseConfig) fetchHashiCorpVaultSecret() error {
	if len(d.Vault.HashiCorp.fetchedSecert) > 0 {
		// Secret is already fetched, do nothing
		return nil
	}
	// Set default username and password attribute values
	requiredKeys := []string{d.Vault.HashiCorp.GetUsernameAttr(), d.Vault.HashiCorp.GetPasswordAttr()}
	secret, err := getHashiCorpVaultSecret(slog.Default(), d.Vault.HashiCorp, requiredKeys)
	if err != nil {
		return err
	}
	d.Vault.HashiCorp.fetchedSecert = secret
	return nil
}

func (d DatabaseConfig) GetUsername() (string, error) {
	if d.isOCIVault() && d.Vault.OCI.UsernameSecret != "" {
		return getOCIVaultSecret(d.Vault.OCI.ID, d.Vault.OCI.UsernameSecret, d.Vault.OCI.Auth)
	}
	if d.isAzureVault() && d.Vault.Azure.UsernameSecret != "" {
		return getAZVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.UsernameSecret)
	}
	if d.isHashiCorpVault() && d.Vault.HashiCorp.MountType != "" && d.Vault.HashiCorp.MountName != "" && d.Vault.HashiCorp.SecretPath != "" {
		if err := d.fetchHashiCorpVaultSecret(); err != nil {
			return "", err
		}
		userName := d.Vault.HashiCorp.fetchedSecert[d.Vault.HashiCorp.GetUsernameAttr()]
		if d.Vault.HashiCorp.AsProxy == "" {
			return userName, nil
		} else {
			return fmt.Sprintf("%s[%s]", userName, d.Vault.HashiCorp.AsProxy), nil
		}
	}
	return d.Username, nil
}

func (d DatabaseConfig) GetPassword() (string, error) {
	if d.PasswordFile != "" {
		bytes, err := os.ReadFile(d.PasswordFile)
		if err != nil {
			return "", fmt.Errorf("failed to read password file %q: %w", d.PasswordFile, err)
		}
		return string(bytes), nil
	}
	if d.isOCIVault() && d.Vault.OCI.PasswordSecret != "" {
		return getOCIVaultSecret(d.Vault.OCI.ID, d.Vault.OCI.PasswordSecret, d.Vault.OCI.Auth)
	}
	if d.isAzureVault() && d.Vault.Azure.PasswordSecret != "" {
		return getAZVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.PasswordSecret)
	}
	if d.isHashiCorpVault() && d.Vault.HashiCorp.MountType != "" && d.Vault.HashiCorp.MountName != "" && d.Vault.HashiCorp.SecretPath != "" {
		if err := d.fetchHashiCorpVaultSecret(); err != nil {
			return "", err
		}
		return d.Vault.HashiCorp.fetchedSecert[d.Vault.HashiCorp.GetPasswordAttr()], nil
	}
	return d.Password, nil
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

func LoadMetricsConfiguration(logger *slog.Logger, cfg *Config) (*MetricsConfiguration, error) {
	m := &MetricsConfiguration{}
	if cfg == nil {
		return m, fmt.Errorf("config file is required")
	}
	if len(strings.TrimSpace(cfg.ConfigFile)) == 0 {
		return m, fmt.Errorf("config file is required")
	}

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

	m.merge()
	return m, m.validate(logger)
}

func (wc WebConfig) Flags() *web.FlagConfig {
	return &web.FlagConfig{
		WebListenAddresses: wc.ListenAddresses,
		WebSystemdSocket:   wc.SystemdSocket,
		WebConfigFile:      wc.ConfigFile,
	}
}

func (m *MetricsConfiguration) merge() {
	if len(m.MetricsPath) == 0 {
		m.MetricsPath = "/metrics"
	}
	m.mergeWebConfig()
	m.mergeLoggingConfig()
	m.mergeMetricsConfig()
	m.mergeOTLPConfig()
	if m.Metrics.ScrapeInterval == nil {
		scrapeInterval := time.Duration(0)
		m.Metrics.ScrapeInterval = &scrapeInterval
	}
}

func (m *MetricsConfiguration) mergeWebConfig() {
	if m.Web.ListenAddresses == nil {
		listenAddress := strings.TrimSpace(m.ListenAddress)
		if listenAddress == "" {
			listenAddress = ":9161"
		}
		listenAddresses := []string{listenAddress}
		m.Web.ListenAddresses = &listenAddresses
	}
	if m.Web.SystemdSocket == nil {
		systemdSocket := false
		m.Web.SystemdSocket = &systemdSocket
	}
	if m.Web.ConfigFile == nil {
		configFile := ""
		m.Web.ConfigFile = &configFile
	}
}

func (m *MetricsConfiguration) mergeLoggingConfig() {
	if m.Logging.LogDisable == nil {
		disable := 0
		m.Logging.LogDisable = &disable
	}
	if m.Logging.LogInterval == nil {
		interval := 15 * time.Second
		m.Logging.LogInterval = &interval
	}
	if m.Logging.LogPerDatabaseFiles == nil {
		perDatabaseFiles := false
		m.Logging.LogPerDatabaseFiles = &perDatabaseFiles
	}
	if len(m.Logging.LogDestination) == 0 {
		m.Logging.LogDestination = "/log/alert.log"
	}
	if len(m.Logging.Level) == 0 {
		m.Logging.Level = "info"
	}
	if len(m.Logging.Format) == 0 {
		m.Logging.Format = "logfmt"
	}
	if m.Logging.LogLevelEnabled == nil {
		logLevelEnabled := false
		m.Logging.LogLevelEnabled = &logLevelEnabled
	}
}

func (m *MetricsConfiguration) mergeMetricsConfig() {
	if len(m.Metrics.Default) == 0 {
		m.Metrics.Default = "default-metrics.toml"
	}
}

func (m *MetricsConfiguration) mergeOTLPConfig() {
	if m.OTLP != nil && m.OTLP.Timeout == nil {
		timeout := 10 * time.Second
		m.OTLP.Timeout = &timeout
	}
}

func (m *MetricsConfiguration) mergeDatabasesFrom() {
	if m.DatabasesFrom == nil {
		return
	}
	if m.DatabasesFrom.OCI != nil {
		if len(m.DatabasesFrom.OCI.Filters.LifecycleState) == 0 {
			m.DatabasesFrom.OCI.Filters.LifecycleState = adb.DatabaseLifecycleStateAvailable
		}
		if len(m.DatabasesFrom.OCI.Filters.RequiredTags) == 0 {
			m.DatabasesFrom.OCI.Filters.RequiredTags = map[string]string{
				tagKeyExporterEnabled: "true",
			}
		}
	}
}

func (m *MetricsConfiguration) validate(logger *slog.Logger) error {
	m.checkDuplicatedDatabases(logger)
	if err := m.validateOCIVaultAuth(); err != nil {
		return err
	}
	if err := m.validateLoggingConfig(); err != nil {
		return err
	}
	if err := m.validateOTLPConfig(); err != nil {
		return err
	}
	if err := m.validateDatabasesFromConfig(); err != nil {
		return err
	}
	return nil
}

func (m *MetricsConfiguration) validateDatabasesFromConfig() error {
	if m.DatabasesFrom == nil {
		return nil
	}
	if m.DatabasesFrom.OCI != nil {
		if len(m.DatabasesFrom.OCI.Compartments) == 0 {
			return errors.New("databasesFrom.oci.compartments is required when databasesFrom.oci is configured")
		}
		if err := oci.ValidateAuthMode(m.DatabasesFrom.OCI.Auth); err != nil {
			return fmt.Errorf("databaseFrom OCI Auth: %w", err)
		}
	}
	return nil
}

func (m *MetricsConfiguration) validateOTLPConfig() error {
	if m.OTLP == nil {
		return nil
	}
	if strings.TrimSpace(m.OTLP.Endpoint) == "" {
		return errors.New("otlp.endpoint is required when otlp is configured")
	}
	if m.Metrics.ScrapeInterval == nil || *m.Metrics.ScrapeInterval <= 0 {
		return errors.New("metrics.scrapeInterval must be greater than zero when otlp is configured")
	}
	if m.OTLP.Timeout == nil || *m.OTLP.Timeout <= 0 {
		return errors.New("otlp.timeout must be greater than zero")
	}
	endpointURL, err := url.ParseRequestURI(m.OTLP.Endpoint)
	if err != nil || endpointURL.Host == "" || (endpointURL.Scheme != "http" && endpointURL.Scheme != "https") {
		return errors.New("otlp.endpoint must be an http:// or https:// URL")
	}
	usesPlaintextURL := endpointURL.Scheme == "http"
	if usesPlaintextURL && m.OTLP.TLS != nil {
		return errors.New("otlp.tls cannot be configured when otlp.endpoint uses http")
	}
	if tls := m.OTLP.TLS; tls != nil {
		if (tls.CertFile == "") != (tls.KeyFile == "") {
			return errors.New("otlp.tls.certFile and otlp.tls.keyFile must be configured together")
		}
		switch tls.MinVersion {
		case "", "TLS1.2", "TLS1.3":
		default:
			return errors.New("otlp.tls.minVersion must be TLS1.2 or TLS1.3")
		}
	}
	return nil
}

func (m *MetricsConfiguration) validateOCIVaultAuth() error {
	for name, cfg := range m.Databases {
		if cfg.Vault == nil || cfg.Vault.OCI == nil {
			continue
		}
		if err := oci.ValidateAuthMode(cfg.Vault.OCI.Auth); err != nil {
			return fmt.Errorf("database %q: %w", name, err)
		}
	}
	return nil
}

func (m *MetricsConfiguration) validateLoggingConfig() error {
	switch m.Logging.Level {
	case "", "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log.level %q; accepted values are debug, info, warn, error", m.Logging.Level)
	}
	switch m.Logging.Format {
	case "", "logfmt", "json":
	default:
		return fmt.Errorf("invalid log.format %q; accepted values are logfmt, json", m.Logging.Format)
	}
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
