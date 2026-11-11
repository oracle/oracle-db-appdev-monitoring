// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package logstream

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
)

func TestLogStreamFileReloadValidatesAllFilesAndHandlesReplacement(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	definition := "logStreams:\n  - id: application\n    query: SELECT ts, message FROM events WHERE ts BETWEEN :1 AND :2 ORDER BY ts\n    timestampColumn: ts\n    output: {type: stdout}\n"
	write(first, definition)
	write(second, "logStreams: []\n")
	cfg := &config.MetricsConfiguration{LogStreamFiles: config.LogStreamFilesConfig{Default: second, Custom: []string{first}}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes := make(chan struct{}, 10)
	if err := watchFiles(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, func() { changes <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	// A bad edit in another file must prevent replacement of the whole set.
	write(second, "logStreams: [\n")
	select {
	case <-changes:
		t.Fatal("invalid definitions triggered reload")
	case <-time.After(200 * time.Millisecond):
	}
	replacement := second + ".new"
	write(replacement, "logStreams: []\n# valid replacement\n")
	if err := os.Rename(replacement, second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changes:
	case <-time.After(3 * time.Second):
		t.Fatal("atomic file replacement did not trigger reload")
	}
	streams, err := config.LoadLogStreams(cfg)
	if err != nil || len(streams) != 1 || streams[0].ID != "application" {
		t.Fatalf("reload did not retain all files: %+v, %v", streams, err)
	}
}

func TestLogCollectorsStartWithUnavailableCheckpointStorage(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "exporter.yaml")
	if err := os.WriteFile(path, []byte("log:\n  stateDirectory: "+blocked+"/checkpoints\n  output: {type: stdout}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg, err := config.LoadMetricsConfiguration(logger, &config.Config{ConfigFile: path})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	defer workers.Wait()
	defer cancel()
	if err := Start(ctx, &workers, logger, cfg, []*db.Database{{Name: "db"}}, "test"); err != nil {
		t.Fatalf("checkpoint storage prevented startup: %v", err)
	}
}
