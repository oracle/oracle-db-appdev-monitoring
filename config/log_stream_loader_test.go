// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadLogStreamFilesAndOverrides(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "streams.yaml")
	tomlPath := filepath.Join(dir, "override.toml")
	for path, content := range map[string]string{
		yamlPath: `logStreams:
  - id: application
    query: SELECT ts, message FROM original WHERE ts BETWEEN :1 AND :2 ORDER BY ts
    timestampColumn: ts
    output: {type: stdout}
  - id: other
    query: SELECT ts, message FROM other WHERE ts BETWEEN :1 AND :2 ORDER BY ts
    timestampColumn: ts
    output: {type: stdout}
`,
		tomlPath: `[[logStreams]]
id = "application"
query = "SELECT ts, message FROM changed WHERE ts BETWEEN :1 AND :2 ORDER BY ts"
interval = "20s"
queryTimeout = "12s"
initialLookback = "1h"
maxRows = 2000
maxBytes = 65536
timestampColumn = "ts"
[logStreams.output]
type = "file"
destination = "/log/application.log"
perDatabaseFiles = true
`,
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(dir, "exporter.yaml")
	mainConfig := fmt.Sprintf("log:\n  disable: 1\nlogStreams:\n  custom: [%q, %q]\n", yamlPath, tomlPath)
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: mainPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LogStreams) != 2 || cfg.LogStreams[0].ID != "application" || cfg.LogStreams[1].ID != "other" {
		t.Fatalf("incorrect merged streams: %+v", cfg.LogStreams)
	}
	s := cfg.LogStreams[0]
	if !strings.Contains(s.Query, "FROM changed") || s.Interval != 20*time.Second || s.QueryTimeout != 12*time.Second || s.InitialLookback != time.Hour || !s.Output.PerDatabaseFiles || s.TimestampColumn != "ts" {
		t.Fatalf("TOML options or override lost: %+v", s)
	}
	if err := os.WriteFile(tomlPath, []byte("[[logStreams]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLogStreams(cfg); err == nil {
		t.Fatal("invalid file accepted")
	}
	if !strings.Contains(cfg.LogStreams[0].Query, "FROM changed") {
		t.Fatal("failed reload replaced active definitions")
	}
	if err := os.WriteFile(tomlPath, []byte("logStreams = []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadLogStreams(cfg)
	if err != nil || len(reloaded) != 2 || !strings.Contains(reloaded[0].Query, "FROM original") {
		t.Fatalf("removed override did not restore earlier file: %+v, %v", reloaded, err)
	}
}

func TestLogStreamFileErrors(t *testing.T) {
	for _, tc := range []struct{ name, extension, content string }{
		{"unknown YAML key", "yaml", "logStreams: []\nunkown: true\n"},
		{"unknown TOML key", "toml", "logStreams = []\nunkown = true\n"},
		{"invalid stream", "yaml", "logStreams:\n  - id: alert\n"},
		{"missing file", "yaml", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "streams."+tc.extension)
			if tc.content != "" {
				if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &MetricsConfiguration{LogStreamFiles: LogStreamFilesConfig{Custom: []string{path}}}
			if _, err := LoadLogStreams(cfg); err == nil {
				t.Fatal("invalid file accepted")
			}
		})
	}
}
