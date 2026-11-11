// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultAlertStreamPreservesLegacySettings(t *testing.T) {
	for _, levels := range []bool{false, true} {
		cfg := MetricsConfiguration{}
		cfg.merge()
		cfg.Logging.LogLevelEnabled = &levels
		interval := 7 * time.Second
		cfg.Logging.LogInterval = &interval
		cfg.Logging.QueryTimeout = 12 * time.Second
		cfg.Logging.InitialLookback = time.Hour
		cfg.Logging.MaxRows = 321
		cfg.Logging.MaxBytes = 4096
		cfg.Logging.Output = LogOutput{Type: "stdout"}
		streams, err := LoadLogStreams(&cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(streams) != 1 {
			t.Fatalf("expected one default stream: %+v", streams)
		}
		s := streams[0]
		if s.ID != "alert" || s.TimestampColumn != "ORIGINATING_TIMESTAMP" || s.Interval != interval || s.QueryTimeout != cfg.Logging.QueryTimeout || s.InitialLookback != time.Hour || s.MaxRows != 321 || s.MaxBytes != 4096 || s.Output.Type != "stdout" {
			t.Fatalf("legacy settings lost: %+v", s)
		}
		for _, part := range []string{"to_timestamp_tz(:1", "to_timestamp_tz(:2", "NLS_DATE_LANGUAGE=American", `order by originating_timestamp, "PARTITION", record_id, filename, process_id, thread_id`} {
			if !strings.Contains(s.Query, part) {
				t.Fatalf("missing %q: %s", part, s.Query)
			}
		}
		if strings.Contains(s.Query, `as "level"`) != levels {
			t.Fatalf("unexpected level projection: %s", s.Query)
		}
		if levels && !strings.Contains(s.Query, "filename,\n case message_type") {
			t.Fatalf("level projection is not separated by a comma: %s", s.Query)
		}
		if !levels && !strings.Contains(s.Query, "filename\n from") {
			t.Fatalf("unexpected default projection: %s", s.Query)
		}
		// Repeated loading must rebuild, not accumulate, the built-in stream.
		cfg.LogStreams = streams
		again, err := LoadLogStreams(&cfg)
		if err != nil || len(again) != 1 {
			t.Fatalf("reload: %+v, %v", again, err)
		}
		disabled := 1
		cfg.Logging.LogDisable = &disabled
		streams, err = LoadLogStreams(&cfg)
		if err != nil || len(streams) != 0 {
			t.Fatalf("disabled alert still loaded: %+v, %v", streams, err)
		}
	}
}

func TestCustomStreamOverridesDefaultAlert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.yaml")
	if err := os.WriteFile(path, []byte("logStreams:\n  - id: alert\n    query: SELECT ts FROM events\n    timestampColumn: ts\n    output: {type: stdout}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, disabled := range []int{0, 1} {
		cfg := MetricsConfiguration{LogStreamFiles: LogStreamFilesConfig{Custom: []string{path}}}
		cfg.Logging.LogDisable = &disabled
		streams, err := LoadLogStreams(&cfg)
		if err != nil || len(streams) != 1 || streams[0].TimestampColumn != "ts" || streams[0].Output.Type != "stdout" {
			t.Fatalf("override not applied: %+v, %v", streams, err)
		}
	}
}

func TestShippedLogStreamFilesLoadAsOrdinaryDefinitions(t *testing.T) {
	for _, name := range []string{"default_log_streams.yaml", "default_log_streams_with_levels.yaml"} {
		cfg := MetricsConfiguration{LogStreamFiles: LogStreamFilesConfig{Default: name}}
		// Legacy settings must not rewrite explicit replacement files.
		cfg.Logging.Output = LogOutput{Type: "stdout"}
		streams, err := LoadLogStreams(&cfg)
		if err != nil || len(streams) != 1 || streams[0].Output.Type != "file" || streams[0].Output.Destination != "/log/alert.log" {
			t.Fatalf("%s is not an ordinary definition: %+v, %v", name, streams, err)
		}
	}
	empty := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(empty, []byte("logStreams: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := MetricsConfiguration{LogStreamFiles: LogStreamFilesConfig{Default: empty}}
	streams, err := LoadLogStreams(&cfg)
	if err != nil || len(streams) != 0 {
		t.Fatalf("default replacement ignored: %+v, %v", streams, err)
	}
	cfg.LogStreamFiles.Default = empty + ".missing"
	if _, err := LoadLogStreams(&cfg); err == nil {
		t.Fatal("missing replacement silently fell back")
	}
}
