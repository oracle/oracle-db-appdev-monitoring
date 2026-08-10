// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package restart

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/oracle/oracle-db-appdev-monitoring/collector"
)

func TestWatchConfigFileRestartsForAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	writeConfig(t, configFile, "metricsPath: /metrics\n")

	changed := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := WatchConfigFile(ctx, testSlogLogger(), configFile, func() error { return nil }, func() {
		changed <- struct{}{}
	}); err != nil {
		t.Fatalf("watchConfigFile() error = %v", err)
	}

	writeConfig(t, filepath.Join(dir, "unrelated.yaml"), "unrelated: true\n")
	assertNoChange(t, changed)

	replacement := filepath.Join(dir, "config.yaml.new")
	writeConfig(t, replacement, "metricsPath: /reloaded\n")
	if err := os.Rename(replacement, configFile); err != nil {
		t.Fatalf("replace config file: %v", err)
	}
	assertChange(t, changed)
}

func TestWatchConfigFileKeepsRunningForInvalidConfiguration(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	writeConfig(t, configFile, "metricsPath: /metrics\n")

	changed := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := WatchConfigFile(ctx, testSlogLogger(), configFile, func() error {
		_, err := collector.LoadMetricsConfiguration(testSlogLogger(), &collector.Config{ConfigFile: configFile})
		return err
	}, func() {
		changed <- struct{}{}
	}); err != nil {
		t.Fatalf("watchConfigFile() error = %v", err)
	}

	// In-place writes can briefly expose an empty or partially written file. The
	// watcher must validate only the settled contents, rather than restarting for
	// a transient configuration that happens to be valid.
	writeConfig(t, configFile, "log:\n  level: invalid\n")
	assertNoChange(t, changed)

	writeConfig(t, configFile, "log: [\n")
	assertNoChange(t, changed)

	writeConfig(t, configFile, "metricsPath: /reloaded\n")
	assertChange(t, changed)
}

func TestIsConfigChangeEventRecognizesKubernetesDataSymlink(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	event := fsnotify.Event{Name: filepath.Join(dir, "..data"), Op: fsnotify.Rename}

	if !isConfigChangeEvent(configFile, event) {
		t.Fatal("expected Kubernetes ConfigMap data symlink update to be recognized")
	}
}

func TestRequestRestartCoalescesRequests(t *testing.T) {
	requests := make(chan Request, 1)
	RequestRestart(requests, "configuration file changed")
	RequestRestart(requests, "RESTART_INTERVAL elapsed")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var restarts atomic.Int32
	done := make(chan struct{})
	go func() {
		RunRestartCoordinator(ctx, testSlogLogger(), requests, func() error {
			restarts.Add(1)
			cancel()
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("restart coordinator did not stop")
	}
	if got := restarts.Load(); got != 1 {
		t.Fatalf("restart calls = %d, want 1", got)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func assertChange(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("configuration change did not request restart")
	}
}

func assertNoChange(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
		t.Fatal("unexpected restart request")
	case <-time.After(200 * time.Millisecond):
	}
}

func testSlogLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
