// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

// DatabasesFromConfig configures database discovery sources.
type DatabasesFromConfig struct {
	WalletDirectory       string                  `yaml:"walletDirectory"`
	OCI                   *OCIDatabasesFromConfig `yaml:"oci"`
	Timeout               *time.Duration          `yaml:"timeout"`
	RefreshInterval       *time.Duration          `yaml:"refreshInterval"`
	WalletRefreshInterval *time.Duration          `yaml:"walletRefreshInterval"`
}

// OCIDatabasesFromConfig configures OCI database discovery.
type OCIDatabasesFromConfig struct {
	Auth         oci.AuthMode                  `yaml:"auth"`
	Compartments []string                      `yaml:"compartments"`
	Filters      OCIDatabasesFromFiltersConfig `yaml:"filters"`
}

// OCIDatabasesFromFiltersConfig narrows the databases discovered from OCI.
type OCIDatabasesFromFiltersConfig struct {
	LifecycleState adb.AutonomousDatabaseLifecycleStateEnum `yaml:"lifecycleState"`
	RequiredTags   map[string]string                        `yaml:"requiredTags"`
}

func (m *MetricsConfiguration) mergeDatabasesFrom() {
	if m.DatabasesFrom == nil {
		return
	}
	if m.DatabasesFrom.WalletDirectory == "" {
		m.DatabasesFrom.WalletDirectory = "/tmp/exporter"
	}
	if m.DatabasesFrom.Timeout == nil {
		value := m.DatabasesFrom.DiscoveryTimeout()
		m.DatabasesFrom.Timeout = &value
	}
	if m.DatabasesFrom.RefreshInterval == nil {
		value := m.DatabasesFrom.DiscoveryRefreshInterval()
		m.DatabasesFrom.RefreshInterval = &value
	}
	if m.DatabasesFrom.WalletRefreshInterval == nil {
		value := m.DatabasesFrom.DiscoveryWalletRefreshInterval()
		m.DatabasesFrom.WalletRefreshInterval = &value
	}
	if m.DatabasesFrom.OCI != nil {
		if len(m.DatabasesFrom.OCI.Filters.LifecycleState) == 0 {
			m.DatabasesFrom.OCI.Filters.LifecycleState = adb.AutonomousDatabaseLifecycleStateAvailable
		}
		if len(m.DatabasesFrom.OCI.Filters.RequiredTags) == 0 {
			m.DatabasesFrom.OCI.Filters.RequiredTags = map[string]string{
				tagKeyExporterEnabled: "true",
			}
		}
	}
}

func (c *DatabasesFromConfig) DiscoveryTimeout() time.Duration {
	if c == nil || c.Timeout == nil {
		return 30 * time.Second
	}
	return *c.Timeout
}

func (c *DatabasesFromConfig) DiscoveryRefreshInterval() time.Duration {
	if c == nil || c.RefreshInterval == nil {
		return 5 * time.Minute
	}
	return *c.RefreshInterval
}

func (c *DatabasesFromConfig) DiscoveryWalletRefreshInterval() time.Duration {
	if c == nil || c.WalletRefreshInterval == nil {
		return 24 * time.Hour
	}
	return *c.WalletRefreshInterval
}

func (m *MetricsConfiguration) validateDatabasesFromConfig() error {
	if m.DatabasesFrom == nil {
		return nil
	}
	if strings.TrimSpace(m.DatabasesFrom.WalletDirectory) == "" {
		return errors.New("databasesFrom.walletDirectory must not be blank")
	}
	if m.DatabasesFrom.DiscoveryTimeout() <= 0 {
		return errors.New("databasesFrom.timeout must be greater than zero")
	}
	if m.DatabasesFrom.DiscoveryRefreshInterval() < 0 {
		return errors.New("databasesFrom.refreshInterval must not be negative")
	}
	if m.DatabasesFrom.DiscoveryWalletRefreshInterval() < 0 {
		return errors.New("databasesFrom.walletRefreshInterval must not be negative")
	}
	if m.DatabasesFrom.OCI != nil {
		if len(m.DatabasesFrom.OCI.Compartments) == 0 {
			return errors.New("databasesFrom.oci.compartments is required when databasesFrom.oci is configured")
		}
		for _, compartment := range m.DatabasesFrom.OCI.Compartments {
			if strings.TrimSpace(compartment) == "" {
				return errors.New("databasesFrom.oci.compartments must not contain empty entries")
			}
		}
		state, ok := adb.GetMappingAutonomousDatabaseLifecycleStateEnum(string(m.DatabasesFrom.OCI.Filters.LifecycleState))
		if !ok {
			return fmt.Errorf("invalid databasesFrom.oci.filters.lifecycleState %q", m.DatabasesFrom.OCI.Filters.LifecycleState)
		}
		m.DatabasesFrom.OCI.Filters.LifecycleState = state
		if err := oci.ValidateAuthMode(m.DatabasesFrom.OCI.Auth); err != nil {
			return fmt.Errorf("databasesFrom OCI auth: %w", err)
		}
	}
	return nil
}
