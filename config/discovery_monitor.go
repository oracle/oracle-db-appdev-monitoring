// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"log/slog"
	"maps"
	"reflect"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

// DiscoveryMonitor reconciles OCI target metadata through a controlled process
// restart. It never changes a live collector's database list or provisions
// wallets during polling. Failed listings leave the active targets untouched.
type DiscoveryMonitor struct {
	configuration *MetricsConfiguration
	static        map[string]DatabaseConfig
	baseline      map[string]DatabaseConfig
	failedWallets map[string]DatabaseConfig
	snapshot      func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (discoverySnapshot, error)
	discoveryMetrics
}

// NewDiscoveryMonitor captures the static targets before discovery. Returns nil
// when no OCI discovery source is configured.
func NewDiscoveryMonitor(configuration *MetricsConfiguration) *DiscoveryMonitor {
	if configuration.DatabasesFrom == nil || configuration.DatabasesFrom.OCI == nil {
		return nil
	}
	return &DiscoveryMonitor{
		configuration:    configuration,
		static:           normalizedDiscoveryTargets(configuration.Databases),
		snapshot:         discoverOCIDatabases,
		discoveryMetrics: newDiscoveryMetrics(),
	}
}

func normalizedDiscoveryTargets(targets map[string]DatabaseConfig) map[string]DatabaseConfig {
	copied := make(map[string]DatabaseConfig, len(targets))
	for name, cfg := range targets {
		// HashiCorp's fetched secret cache belongs to connection initialization,
		// not target identity. Copy only immutable source fields so comparisons do
		// not alias or read the cache while a connection refresh writes it.
		if cfg.Vault != nil && cfg.Vault.HashiCorp != nil {
			vault := *cfg.Vault
			source := cfg.Vault.HashiCorp
			vault.HashiCorp = &HashiCorpVault{
				Socket: source.Socket, MountType: source.MountType, MountName: source.MountName,
				SecretPath: source.SecretPath, UsernameAttr: source.UsernameAttr,
				PasswordAttr: source.PasswordAttr, AsProxy: source.AsProxy,
			}
			cfg.Vault = &vault
		}
		// Generated wallet paths are runtime state; compare wallet sources instead.
		if cfg.DiscoveryWallet != nil {
			cfg.TNSAdmin = ""
		}
		copied[name] = cfg
	}
	return copied
}

// Initialize runs the materializing discovery once, before database registration.
// Failed discovery leaves static targets intact; merging commits atomically.
func (m *DiscoveryMonitor) Initialize(ctx context.Context, logger *slog.Logger) error {
	started := time.Now()
	discoveryCtx, cancel := context.WithTimeout(ctx, m.configuration.DatabasesFrom.DiscoveryTimeout())
	defer cancel()
	if err := oci.InitializeWalletDirectory(m.configuration.DatabasesFrom.WalletDirectory); err != nil {
		m.baseline = normalizedDiscoveryTargets(m.configuration.Databases)
		m.record(started, err, 0, 0, 0)
		return err
	}
	snapshot, err := m.snapshot(discoveryCtx, logger, m.configuration.DatabasesFrom.OCI)
	if err == nil {
		var skipped int
		m.failedWallets, skipped, err = m.configuration.mergeDiscoveredDatabases(discoveryCtx, logger, snapshot.Databases)
		snapshot.Skipped += skipped
	}
	m.baseline = normalizedDiscoveryTargets(m.configuration.Databases)
	// Keep failed wallet candidates in the metadata baseline so an unchanged
	// listing does not restart the exporter on every refresh. The existing
	// wallet renewal timer also retries these targets.
	maps.Copy(m.baseline, m.failedWallets)
	accepted := len(m.configuration.Databases) - len(m.static)
	m.record(started, err, accepted, snapshot.Matched, snapshot.Skipped)
	if err != nil {
		return err
	}
	if len(m.configuration.Databases) == 0 {
		logger.Warn("OCI discovery found no usable database targets; serving discovery status without database targets")
	}
	logger.Info("Completed OCI database discovery", "discovered targets", accepted, "static targets", len(m.static), "matched resources", snapshot.Matched, "skipped resources", snapshot.Skipped)
	return nil
}

// Poll reports whether effective target metadata changed. A complete successful
// result is required before a change can request a restart. It does not modify
// the baseline; after the first change, Run returns and shutdown owns cleanup.
func (m *DiscoveryMonitor) Poll(ctx context.Context, logger *slog.Logger) (bool, error) {
	started := time.Now()
	discoveryCtx, cancel := context.WithTimeout(ctx, m.configuration.DatabasesFrom.DiscoveryTimeout())
	defer cancel()
	snapshot, err := m.snapshot(discoveryCtx, logger, m.configuration.DatabasesFrom.OCI)
	if err != nil {
		m.record(started, err, 0, 0, 0)
		return false, err
	}
	effective := maps.Clone(m.static)
	accepted := 0
	for name, cfg := range snapshot.Databases {
		if _, exists := effective[name]; exists {
			snapshot.Skipped++
			continue
		}
		effective[name] = cfg
		if failed, ok := m.failedWallets[name]; ok && reflect.DeepEqual(failed, cfg) {
			snapshot.Skipped++
			continue
		}
		accepted++
	}
	m.record(started, nil, accepted, snapshot.Matched, snapshot.Skipped)
	return !reflect.DeepEqual(m.baseline, normalizedDiscoveryTargets(effective)), nil
}

// Run periodically detects metadata changes and renews managed wallets by a
// controlled restart. Wallet renewal is independent of metadata polling; zero
// disables each interval separately.
func (m *DiscoveryMonitor) Run(ctx context.Context, logger *slog.Logger, requestRestart func()) {
	interval := m.configuration.DatabasesFrom.DiscoveryRefreshInterval()
	var refresh <-chan time.Time
	if interval > 0 {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		refresh = ticker.C
	}
	var renew <-chan time.Time
	var renewalTimer *time.Timer
	renewalInterval := m.configuration.DatabasesFrom.DiscoveryWalletRefreshInterval()
	if renewalInterval > 0 {
		for _, database := range m.baseline {
			if database.DiscoveryWallet != nil {
				renewalTimer = time.NewTimer(renewalInterval)
				defer renewalTimer.Stop()
				renew = renewalTimer.C
				break
			}
		}
	}
	if refresh == nil && renew == nil {
		return
	}
	for {
		var renewal bool
		select {
		case <-ctx.Done():
			return
		case <-renew:
			renewal = true
		case <-refresh:
		}
		// Renewal also probes metadata before restarting so known OCI outages
		// retain active targets. Neither kind of poll generates wallets.
		changed, err := m.Poll(ctx, logger)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if renewal {
				logger.Warn("OCI wallet renewal discovery failed; retaining active targets and retrying", "error", err)
				retry := interval
				if retry <= 0 {
					retry = 5 * time.Minute
				}
				renewalTimer.Reset(retry)
			} else {
				logger.Warn("OCI discovery refresh failed; retaining active database targets", "error", err)
			}
			continue
		}
		if renewal {
			logger.Info("OCI discovery wallet refresh interval elapsed; requesting controlled restart")
		} else if changed {
			logger.Info("OCI database targets changed; requesting controlled restart")
		} else {
			continue
		}
		requestRestart()
		return
	}
}
