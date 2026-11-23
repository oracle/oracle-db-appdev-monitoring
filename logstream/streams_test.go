// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package logstream

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
)

func TestLogStreamFileWatcherValidatesAllFilesAndNotifiesOnReplacement(t *testing.T) {
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
	// A bad edit in another file must suppress the change notification.
	write(second, "logStreams: [\n")
	select {
	case <-changes:
		t.Fatal("invalid definitions triggered a change notification")
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
		t.Fatal("atomic file replacement did not trigger a change notification")
	}
	streams, err := config.LoadLogStreams(cfg)
	if err != nil || len(streams) != 1 || streams[0].ID != "application" {
		t.Fatalf("loading definitions did not retain all files: %+v, %v", streams, err)
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

func TestLogStreamStartupSkipsOnlyUnsafeDiscoveredFileAlias(t *testing.T) {
	dir := t.TempDir()
	streamsPath := filepath.Join(dir, "streams.yaml")
	streams := `logStreams:
  - id: per-db
    databases: [valid-db, db/invalid]
    query: SELECT ts FROM events
    timestampColumn: ts
    output: {type: file, destination: events.log, perDatabaseFiles: true}
  - id: independent
    query: SELECT ts FROM events
    timestampColumn: ts
    output: {type: stdout}
  - id: pending
    databases: [absent-db]
    query: SELECT ts FROM events
    timestampColumn: ts
    output: {type: stdout}
`
	if err := os.WriteFile(streamsPath, []byte(streams), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "exporter.yaml")
	settings := "databasesFrom:\n  oci:\n    compartments: [test-compartment]\nlog:\n  disable: 1\n  stateDirectory: " + dir + "\nlogStreams:\n  custom: [" + streamsPath + "]\n"
	if err := os.WriteFile(path, []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg, err := config.LoadMetricsConfiguration(logger, &config.Config{ConfigFile: path})
	if err != nil {
		t.Fatalf("failed initial configuration load: %v", err)
	}

	cfg.Databases = map[string]config.DatabaseConfig{
		"valid-db":   {ResourceID: "valid-ocid"},
		"db/invalid": {ResourceID: "invalid-alias-ocid"},
		"other-db":   {ResourceID: "other-ocid"},
	}
	databases := []*db.Database{
		{Name: "db/invalid", Config: cfg.Databases["db/invalid"]},
		{Name: "valid-db", Config: cfg.Databases["valid-db"]},
		{Name: "other-db", Config: cfg.Databases["other-db"]},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var workers sync.WaitGroup
	if err := Start(ctx, &workers, logger, cfg, databases, "test"); err != nil {
		t.Fatalf("one filename alias prevented startup: %v", err)
	}
	cancel()
	workers.Wait()
	messages := logs.String()
	if !strings.Contains(messages, "level=WARN") || !strings.Contains(messages, "database=db/invalid stream=per-db") {
		t.Fatalf("missing warning for skipped file collector: %s", messages)
	}
	if !strings.Contains(messages, "streams=4") {
		t.Fatalf("expected one per-database file collector and three stdout collectors: %s", messages)
	}
}
