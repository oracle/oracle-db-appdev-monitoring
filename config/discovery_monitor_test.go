// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func monitorConfiguration() *MetricsConfiguration {
	return &MetricsConfiguration{DatabasesFrom: &DatabasesFromConfig{OCI: &OCIDatabasesFromConfig{}}}
}

func TestDiscoveryMonitorDetectsEffectiveTargetChanges(t *testing.T) {
	for _, tt := range []struct {
		name          string
		initial, next map[string]DatabaseConfig
		want          bool
	}{
		{"unchanged", map[string]DatabaseConfig{"orders": {URL: "orders", ResourceID: "ocid1"}}, map[string]DatabaseConfig{"orders": {URL: "orders", ResourceID: "ocid1"}}, false},
		{"added", map[string]DatabaseConfig{}, map[string]DatabaseConfig{"orders": {URL: "orders"}}, true},
		{"removed", map[string]DatabaseConfig{"orders": {URL: "orders"}}, map[string]DatabaseConfig{}, true},
		{"URL changed", map[string]DatabaseConfig{"orders": {URL: "old"}}, map[string]DatabaseConfig{"orders": {URL: "new"}}, true},
		{"renamed", map[string]DatabaseConfig{"orders": {URL: "orders"}}, map[string]DatabaseConfig{"renamed": {URL: "orders"}}, true},
		{"resource replaced", map[string]DatabaseConfig{"orders": {URL: "orders", ResourceID: "ocid1"}}, map[string]DatabaseConfig{"orders": {URL: "orders", ResourceID: "ocid2"}}, true},
		{"labels changed", map[string]DatabaseConfig{"orders": {URL: "orders", Labels: map[string]string{"team": "one"}}}, map[string]DatabaseConfig{"orders": {URL: "orders", Labels: map[string]string{"team": "two"}}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewDiscoveryMonitor(monitorConfiguration())
			m.baseline = normalizedDiscoveryTargets(tt.initial)
			m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
				return tt.next, nil
			})
			got, err := m.Poll(context.Background(), testLogger())
			if err != nil || got != tt.want {
				t.Fatalf("changed=%v error=%v, want %v", got, err, tt.want)
			}
		})
	}
}

func TestDiscoveryMonitorIgnoresCollidingDiscoveredTargets(t *testing.T) {
	cfg := monitorConfiguration()
	cfg.Databases = map[string]DatabaseConfig{"orders": {URL: "static"}}
	m := NewDiscoveryMonitor(cfg)
	m.baseline = normalizedDiscoveryTargets(cfg.Databases)
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return map[string]DatabaseConfig{"orders": {URL: "changed-discovered"}}, nil
	})
	if changed, err := m.Poll(context.Background(), testLogger()); err != nil || changed {
		t.Fatalf("static collision triggered a change: %v, %v", changed, err)
	}
}

func TestDiscoveryMonitorFailureRetainsLastSuccessfulState(t *testing.T) {
	m := NewDiscoveryMonitor(monitorConfiguration())
	m.baseline = map[string]DatabaseConfig{"orders": {URL: "orders"}}
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return map[string]DatabaseConfig{"orders": {URL: "orders"}}, nil
	})
	if _, err := m.Poll(context.Background(), testLogger()); err != nil {
		t.Fatal(err)
	}
	lastSuccess := m.lastSuccess
	want := errors.New("OCI unavailable")
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return nil, want
	})
	if changed, err := m.Poll(context.Background(), testLogger()); changed || !errors.Is(err, want) {
		t.Fatalf("failed poll changed targets: %v, %v", changed, err)
	}
	if m.success != 0 || m.lastSuccess != lastSuccess || m.targets != 1 || m.baseline["orders"].URL != "orders" {
		t.Fatalf("last successful state changed: %#v", m)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(m)
	metrics, err := registry.Gather()
	if err != nil || len(metrics) != 6 {
		t.Fatalf("unexpected discovery metrics: %v, %v", metrics, err)
	}
}

func TestDiscoveryMonitorBoundsRequests(t *testing.T) {
	cfg := monitorConfiguration()
	timeout := 10 * time.Millisecond
	cfg.DatabasesFrom.Timeout = &timeout
	m := NewDiscoveryMonitor(cfg)
	m.snapshot = testDiscoverySnapshot(func(ctx context.Context, _ *slog.Logger, _ *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if changed, err := m.Poll(context.Background(), testLogger()); changed || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected discovery deadline: %v, %v", changed, err)
	}
}

func TestDiscoveryMonitorStartupFailurePreservesStaticTargets(t *testing.T) {
	original := discoverOCIStats
	defer func() { discoverOCIStats = original }()
	discoverOCIStats = testOCIDiscoverySnapshot(func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return nil, errors.New("listing failed")
	})
	cfg := monitorConfiguration()
	cfg.Databases = map[string]DatabaseConfig{"orders": {URL: "static"}}
	m := NewDiscoveryMonitor(cfg)
	if err := m.Initialize(context.Background(), testLogger()); err == nil {
		t.Fatal("expected initial listing failure")
	}
	if len(cfg.Databases) != 1 || cfg.Databases["orders"].URL != "static" || m.baseline["orders"].URL != "static" {
		t.Fatalf("static targets lost: %#v", cfg.Databases)
	}
}

func TestDiscoveryMonitorSuccessfulEmptyStartup(t *testing.T) {
	original := discoverOCIStats
	defer func() { discoverOCIStats = original }()
	discoverOCIStats = testOCIDiscoverySnapshot(func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return map[string]DatabaseConfig{}, nil
	})
	m := NewDiscoveryMonitor(monitorConfiguration())
	if err := m.Initialize(context.Background(), testLogger()); err != nil {
		t.Fatalf("empty successful discovery should remain observable: %v", err)
	}
	if m.success != 1 || m.targets != 0 {
		t.Fatalf("unexpected discovery status: %#v", m)
	}
}

func TestDiscoveryMonitorRunsUntilFirstChange(t *testing.T) {
	cfg := monitorConfiguration()
	interval := time.Millisecond
	cfg.DatabasesFrom.RefreshInterval = &interval
	m := NewDiscoveryMonitor(cfg)
	m.baseline = map[string]DatabaseConfig{}
	calls := 0
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("temporary failure")
		}
		return map[string]DatabaseConfig{"orders": {URL: "orders"}}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	restarts := 0
	m.Run(ctx, testLogger(), func() { restarts++ })
	if restarts != 1 || calls != 2 {
		t.Fatalf("unexpected monitor behavior: %d restarts, %d polls", restarts, calls)
	}
}

func TestDiscoveryMonitorDisabledAndUnconfigured(t *testing.T) {
	if NewDiscoveryMonitor(&MetricsConfiguration{}) != nil {
		t.Fatal("monitor created without discovery")
	}
	cfg := monitorConfiguration()
	zero := time.Duration(0)
	cfg.DatabasesFrom.RefreshInterval = &zero
	m := NewDiscoveryMonitor(cfg)
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		t.Fatal("disabled monitor polled")
		return nil, nil
	})
	m.Run(context.Background(), testLogger(), func() { t.Fatal("disabled monitor requested restart") })
}

func TestDiscoveryMonitorComparesWalletSourcesInsteadOfGeneratedPaths(t *testing.T) {
	wallet := &DiscoveryWalletConfig{DatabaseID: "ocid1", VaultID: "vault", SecretName: "wallet"}
	cfg := DatabaseConfig{URL: "orders", DiscoveryWallet: wallet}
	cfg.TNSAdmin = "/tmp/generated-wallet"
	m := NewDiscoveryMonitor(monitorConfiguration())
	m.baseline = normalizedDiscoveryTargets(map[string]DatabaseConfig{"orders": cfg})
	cfg.TNSAdmin = ""
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return map[string]DatabaseConfig{"orders": cfg}, nil
	})
	if changed, err := m.Poll(context.Background(), testLogger()); changed || err != nil {
		t.Fatalf("generated wallet path triggered restart: %v, %v", changed, err)
	}
	replacement := *wallet
	replacement.SecretName = "replacement-wallet"
	cfg.DiscoveryWallet = &replacement
	if changed, err := m.Poll(context.Background(), testLogger()); !changed || err != nil {
		t.Fatalf("wallet source change was not detected: %v, %v", changed, err)
	}
}

func TestDiscoveryMonitorRenewsWalletWithMetadataPollingDisabled(t *testing.T) {
	cfg := monitorConfiguration()
	zero := time.Duration(0)
	interval := time.Millisecond
	cfg.DatabasesFrom.RefreshInterval = &zero
	cfg.DatabasesFrom.WalletRefreshInterval = &interval
	m := NewDiscoveryMonitor(cfg)
	m.baseline = map[string]DatabaseConfig{"orders": {DiscoveryWallet: &DiscoveryWalletConfig{DatabaseID: "ocid1"}}}
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return map[string]DatabaseConfig{"orders": {DiscoveryWallet: &DiscoveryWalletConfig{DatabaseID: "ocid1"}}}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	restarts := 0
	m.Run(ctx, testLogger(), func() { restarts++ })
	if restarts != 1 {
		t.Fatalf("expected wallet renewal restart, got %d", restarts)
	}
}

func TestDiscoveryMonitorWalletRenewalRequiresManagedWallet(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		cfg := monitorConfiguration()
		zero := time.Duration(0)
		interval := time.Millisecond
		cfg.DatabasesFrom.RefreshInterval = &zero
		cfg.DatabasesFrom.WalletRefreshInterval = &interval
		m := NewDiscoveryMonitor(cfg)
		if disabled {
			cfg.DatabasesFrom.WalletRefreshInterval = &zero
			m.baseline = map[string]DatabaseConfig{"orders": {DiscoveryWallet: &DiscoveryWalletConfig{DatabaseID: "ocid1"}}}
		} else {
			m.baseline = map[string]DatabaseConfig{"orders": {ConnectConfig: ConnectConfig{TNSAdmin: "/external-wallet"}}}
		}
		m.Run(context.Background(), testLogger(), func() { t.Fatal("unexpected wallet renewal restart") })
	}
}

func TestDiscoveryMonitorIgnoresConnectionSecretCache(t *testing.T) {
	cfg := monitorConfiguration()
	source := &HashiCorpVault{Socket: "socket", MountType: "kvv2", MountName: "mount", SecretPath: "path"}
	cfg.Databases = map[string]DatabaseConfig{"orders": {URL: "orders", Vault: &VaultConfig{HashiCorp: source}}}
	m := NewDiscoveryMonitor(cfg)
	m.baseline = normalizedDiscoveryTargets(cfg.Databases)
	source.fetchedSecert = map[string]string{"username": "runtime-user", "password": "runtime-password"}
	if len(m.static["orders"].Vault.HashiCorp.fetchedSecert) != 0 || len(m.baseline["orders"].Vault.HashiCorp.fetchedSecert) != 0 {
		t.Fatal("discovery target identity aliased runtime secret cache")
	}
	m.snapshot = testDiscoverySnapshot(func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error) {
		return nil, nil
	})
	if changed, err := m.Poll(context.Background(), testLogger()); changed || err != nil {
		t.Fatalf("runtime secret cache triggered target change: %v, %v", changed, err)
	}
}

func testDiscoverySnapshot(fn func(context.Context, *slog.Logger, *DatabasesFromConfig) (map[string]DatabaseConfig, error)) func(context.Context, *slog.Logger, *DatabasesFromConfig) (DiscoverySnapshot, error) {
	return func(ctx context.Context, logger *slog.Logger, cfg *DatabasesFromConfig) (DiscoverySnapshot, error) {
		targets, err := fn(ctx, logger, cfg)
		return DiscoverySnapshot{Databases: targets, Matched: len(targets)}, err
	}
}
func testOCIDiscoverySnapshot(fn func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (map[string]DatabaseConfig, error)) func(context.Context, *slog.Logger, *OCIDatabasesFromConfig) (DiscoverySnapshot, error) {
	return func(ctx context.Context, logger *slog.Logger, cfg *OCIDatabasesFromConfig) (DiscoverySnapshot, error) {
		targets, err := fn(ctx, logger, cfg)
		return DiscoverySnapshot{Databases: targets, Matched: len(targets)}, err
	}
}

func TestDiscoveryMonitorCountsMatchedSkippedAndStaticCollisions(t *testing.T) {
	cfg := monitorConfiguration()
	cfg.Databases = map[string]DatabaseConfig{"static": {URL: "static"}}
	m := NewDiscoveryMonitor(cfg)
	m.baseline = normalizedDiscoveryTargets(cfg.Databases)
	m.snapshot = func(context.Context, *slog.Logger, *DatabasesFromConfig) (DiscoverySnapshot, error) {
		return DiscoverySnapshot{Databases: map[string]DatabaseConfig{"static": {URL: "discovered"}, "accepted": {URL: "accepted"}}, Matched: 4, Skipped: 2}, nil
	}
	if changed, err := m.Poll(context.Background(), testLogger()); !changed || err != nil {
		t.Fatalf("expected valid changed snapshot: %v, %v", changed, err)
	}
	if m.matched != 4 || m.skipped != 3 || m.targets != 1 {
		t.Fatalf("unexpected counts: matched=%v skipped=%v accepted=%v", m.matched, m.skipped, m.targets)
	}
	m.snapshot = func(context.Context, *slog.Logger, *DatabasesFromConfig) (DiscoverySnapshot, error) {
		return DiscoverySnapshot{}, errors.New("API unavailable")
	}
	m.Poll(context.Background(), testLogger())
	if m.matched != 4 || m.skipped != 3 || m.targets != 1 || m.success != 0 {
		t.Fatalf("failed poll overwrote successful counts: %#v", m)
	}
}

func TestDiscoveryMonitorWalletRenewalRetainsTargetsDuringKnownOutage(t *testing.T) {
	cfg := monitorConfiguration()
	zero := time.Duration(0)
	interval := time.Millisecond
	cfg.DatabasesFrom.RefreshInterval = &zero
	cfg.DatabasesFrom.WalletRefreshInterval = &interval
	m := NewDiscoveryMonitor(cfg)
	m.baseline = map[string]DatabaseConfig{"orders": {DiscoveryWallet: &DiscoveryWalletConfig{DatabaseID: "ocid1"}}}
	polls := 0
	m.snapshot = func(context.Context, *slog.Logger, *DatabasesFromConfig) (DiscoverySnapshot, error) {
		polls++
		return DiscoverySnapshot{}, errors.New("OCI unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	m.Run(ctx, testLogger(), func() { t.Fatal("wallet renewal restarted during known OCI outage") })
	if polls != 1 || m.success != 0 || m.baseline["orders"].DiscoveryWallet == nil {
		t.Fatalf("unexpected renewal failure behavior: %d polls, %#v", polls, m)
	}
}
