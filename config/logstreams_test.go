// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLogStreamDefaultsAndValidation(t *testing.T) {
	cfg := MetricsConfiguration{Databases: map[string]DatabaseConfig{"db": {}}, LogStreams: []LogStreamConfig{{ID: "application", Query: "SELECT ts, message FROM events WHERE ts BETWEEN to_timestamp_tz(:1, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9TZH:TZM') AND to_timestamp_tz(:2, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9TZH:TZM') ORDER BY ts, id", TimestampColumn: "ts", Output: LogOutput{Type: "stdout"}}}}
	cfg.merge()
	if err := cfg.validateLogStreams(); err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.StateDirectory != "/log/checkpoints" || cfg.LogStreams[0].Interval != 15*time.Second {
		t.Fatal("wrong defaults")
	}
	for _, mutate := range []func(*MetricsConfiguration){func(m *MetricsConfiguration) { m.LogStreams[0].TimestampColumn = "" }, func(m *MetricsConfiguration) { m.LogStreams[0].ID = "" }, func(m *MetricsConfiguration) { m.LogStreams[0].Query = "" }, func(m *MetricsConfiguration) { m.LogStreams[0].Databases = []string{"missing"} }, func(m *MetricsConfiguration) { m.LogStreams[0].Output.PerDatabaseFiles = true }, func(m *MetricsConfiguration) { m.LogStreams[0].QueryTimeout = -time.Second }} {
		copy := cfg
		copy.LogStreams = append([]LogStreamConfig(nil), cfg.LogStreams...)
		mutate(&copy)
		if err := copy.validateLogStreams(); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
func TestOTLPLogsDoNotRequireScheduledMetrics(t *testing.T) {
	cfg := MetricsConfiguration{}
	cfg.merge()
	cfg.Logging.Output = LogOutput{Type: "otlp"}
	cfg.OTLP = &OTLPConfig{Endpoint: "http://localhost:4317"}
	streams, err := LoadLogStreams(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.LogStreams = streams
	if err := cfg.validateOTLPConfig(); err != nil {
		t.Fatal(err)
	}
	if cfg.Metrics.ScrapeInterval != nil && *cfg.Metrics.ScrapeInterval != 0 {
		t.Fatal("logs enabled scheduled metrics")
	}
	if cfg.OTLP.Timeout != nil {
		t.Fatal("validation mutated the shared OTLP configuration")
	}
}
func TestStdoutPerDatabaseRejected(t *testing.T) {
	cfg := MetricsConfiguration{}
	cfg.merge()
	cfg.Logging.LogDestination = "/dev/stdout"
	yes := true
	cfg.Logging.LogPerDatabaseFiles = &yes
	if _, err := LoadLogStreams(&cfg); err == nil {
		t.Fatal("stdout per-database files accepted")
	}
}

func TestNestedFileOutputAndLegacyStdoutConflict(t *testing.T) {
	cfg := MetricsConfiguration{}
	cfg.merge()
	cfg.Logging.Output = LogOutput{Type: "file", Destination: "/new/path.log", PerDatabaseFiles: true}
	if got := cfg.AlertLogOutput(); got.Destination != "/new/path.log" || !got.PerDatabaseFiles {
		t.Fatalf("nested file options ignored: %+v", got)
	}
	yes := true
	cfg.Logging.LogPerDatabaseFiles = &yes
	cfg.Logging.Output = LogOutput{Type: "stdout"}
	if _, err := LoadLogStreams(&cfg); err == nil {
		t.Fatal("legacy per-database setting silently ignored for stdout")
	}
}

func TestLogStreamYAMLDurationAndTimestampColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.yaml")
	if err := os.WriteFile(path, []byte(`logStreams:
  - id: application
    interval: 20s
    query: SELECT ts, message FROM events WHERE ts BETWEEN :1 AND :2 ORDER BY ts
    timestampColumn: ts
    output:
      type: stdout
`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := MetricsConfiguration{LogStreamFiles: LogStreamFilesConfig{Custom: []string{path}}}
	streams, err := LoadLogStreams(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if streams[1].Interval != 20*time.Second || streams[1].TimestampColumn != "ts" {
		t.Fatal("YAML options were not preserved")
	}
}

func TestLogStreamQueryIsPassedThroughWithoutSQLValidation(t *testing.T) {
	for _, query := range []string{
		"SELECT ts, 'prefix; suffix' AS message FROM events",
		"WITH events AS (SELECT systimestamp ts FROM dual) SELECT ts FROM events",
		"/* application events */ SELECT ts FROM events",
	} {
		cfg := MetricsConfiguration{LogStreams: []LogStreamConfig{{ID: "events", Query: query, TimestampColumn: "ts", Output: LogOutput{Type: "stdout"}}}}
		if err := cfg.validateLogStreams(); err != nil {
			t.Fatalf("query rejected: %s: %v", query, err)
		}
		if cfg.LogStreams[0].Query != query {
			t.Fatal("query text changed")
		}
	}
}

func TestCheckpointDirectoryDefaultsBesideFileDestination(t *testing.T) {
	for _, tc := range []struct {
		name, destination, stateDirectory string
		output                            LogOutput
		want                              string
	}{
		{name: "legacy file", destination: "/Users/anders/alert.log", want: "/Users/anders/checkpoints"},
		{name: "nested file takes precedence", destination: "/legacy/alert.log", output: LogOutput{Type: "file", Destination: "/logs/alert.log"}, want: "/logs/checkpoints"},
		{name: "relative file", destination: "logs/alert.log", want: "logs/checkpoints"},
		{name: "file in working directory", destination: "alert.log", want: "checkpoints"},
		{name: "explicit state", destination: "/logs/alert.log", stateDirectory: "/state/exporter", want: "/state/exporter"},
		{name: "stdout destination", destination: "/dev/stdout", want: "/log/checkpoints"},
		{name: "stdout alias", destination: "stdout", want: "/log/checkpoints"},
		{name: "stdout output", destination: "/unused/alert.log", output: LogOutput{Type: "stdout"}, want: "/log/checkpoints"},
		{name: "otlp output", destination: "/unused/alert.log", output: LogOutput{Type: "otlp"}, want: "/log/checkpoints"},
		{name: "default destination", want: "/log/checkpoints"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := MetricsConfiguration{Logging: LoggingConfig{LogDestination: tc.destination, StateDirectory: tc.stateDirectory, Output: tc.output}}
			cfg.merge()
			if cfg.Logging.StateDirectory != tc.want {
				t.Fatalf("got %q, want %q", cfg.Logging.StateDirectory, tc.want)
			}
		})
	}
}

func TestSharedOTLPConfigurationLoadsForLogsOnly(t *testing.T) {
	path := writeExporterConfig(t, `
log:
  output:
    type: otlp
otlp:
  endpoint: http://collector:4317
  headers:
    authorization: Bearer test
`)
	cfg, err := LoadMetricsConfiguration(testLogger(), &Config{ConfigFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LogStreams) != 1 || cfg.LogStreams[0].Output.Type != "otlp" || *cfg.Metrics.ScrapeInterval != 0 || cfg.OTLP.Headers["authorization"] != "Bearer test" || *cfg.OTLP.Timeout != 10*time.Second {
		t.Fatalf("unexpected shared configuration: %+v", cfg)
	}
}

func TestConcurrentLogStreamLoadsDoNotMutateSharedOTLP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streams.yaml")
	override := "logStreams:\n  - id: alert\n    query: SELECT ts FROM events\n    timestampColumn: ts\n    output: {type: stdout}\n"
	if err := os.WriteFile(path, []byte(override), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := MetricsConfiguration{OTLP: &OTLPConfig{Endpoint: "http://collector:4317"}, LogStreamFiles: LogStreamFilesConfig{Custom: []string{path}}}
	cfg.Logging.Output = LogOutput{Type: "otlp"}
	if _, err := LoadLogStreams(&cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("logStreams: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		workers.Go(func() {
			<-start
			streams, err := LoadLogStreams(&cfg)
			if err != nil || len(streams) != 1 || streams[0].Output.Type != "otlp" {
				t.Errorf("concurrent reload: %+v, %v", streams, err)
			}
		})
	}
	close(start)
	workers.Wait()
	if cfg.OTLP.Timeout != nil {
		t.Fatal("reload validation mutated shared OTLP timeout")
	}
}

func TestOTLPLogStreamRequiresSharedTransport(t *testing.T) {
	cfg := MetricsConfiguration{}
	cfg.Logging.Output = LogOutput{Type: "otlp"}
	if _, err := LoadLogStreams(&cfg); err == nil {
		t.Fatal("missing shared transport accepted")
	}
	zero := time.Duration(0)
	cfg.OTLP = &OTLPConfig{Endpoint: "http://collector:4317", Timeout: &zero}
	if _, err := LoadLogStreams(&cfg); err == nil {
		t.Fatal("invalid shared timeout accepted")
	}
}
