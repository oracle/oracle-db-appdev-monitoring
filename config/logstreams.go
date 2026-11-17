// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// LogStreamFilesConfig lists definition files, following metrics.default and metrics.custom.
type LogStreamFilesConfig struct {
	Default string   `yaml:"default"`
	Custom  []string `yaml:"custom"`
}

func (m *MetricsConfiguration) LogStreamDefinitionFiles() []string {
	paths := make([]string, 0, len(m.LogStreamFiles.Custom)+1)
	if m.LogStreamFiles.Default != "" && (m.Logging.LogDisable == nil || m.LogDisable() != 1) {
		paths = append(paths, m.LogStreamFiles.Default)
	}
	return append(paths, m.LogStreamFiles.Custom...)
}

// LogStreamConfig describes an append-only query. Its SQL binds :1 and :2 are
// inclusive UTC boundaries in the documented fixed timestamp format.
type LogStreamConfig struct {
	ID              string        `yaml:"id"`
	Databases       []string      `yaml:"databases"`
	Query           string        `yaml:"query"`
	TimestampColumn string        `yaml:"timestampColumn"`
	Interval        time.Duration `yaml:"interval"`
	QueryTimeout    time.Duration `yaml:"queryTimeout"`
	InitialLookback time.Duration `yaml:"initialLookback"`
	MaxRows         int           `yaml:"maxRows"`
	MaxBytes        int           `yaml:"maxBytes"`
	Output          LogOutput     `yaml:"output"`
}

type LogOutput struct {
	Type             string `yaml:"type"`
	Destination      string `yaml:"destination"`
	PerDatabaseFiles bool   `yaml:"perDatabaseFiles"`
}

func IsStdout(path string) bool { return path == "stdout" || path == "/dev/stdout" }

// AlertLogOutput translates legacy file options into the shared output shape.
// An explicit nested destination takes precedence over the legacy destination.
func (m *MetricsConfiguration) AlertLogOutput() LogOutput {
	out := m.Logging.Output
	if out.Type == "file" && out.Destination == "" {
		out.Destination = m.LogDestination()
	}
	out.PerDatabaseFiles = out.PerDatabaseFiles || m.LogPerDatabaseFiles()
	return out
}

func (m *MetricsConfiguration) mergeLogStreams() {
	if m.Logging.Output.Type == "" {
		m.Logging.Output.Type = "file"
	}
	if m.Logging.StateDirectory == "" {
		m.Logging.StateDirectory = "/log/checkpoints"
		out := m.AlertLogOutput()
		if out.Type == "file" && out.Destination != "" && !IsStdout(out.Destination) {
			m.Logging.StateDirectory = filepath.Join(filepath.Dir(out.Destination), "checkpoints")
		}
	}
	if m.Logging.QueryTimeout == 0 {
		m.Logging.QueryTimeout = 30 * time.Second
	}
	if m.Logging.MaxRows == 0 {
		m.Logging.MaxRows = 10000
	}
	if m.Logging.MaxBytes == 0 {
		m.Logging.MaxBytes = 16 << 20
	}
	for i := range m.LogStreams {
		s := &m.LogStreams[i]
		if s.Interval == 0 {
			s.Interval = 15 * time.Second
		}
		if s.QueryTimeout == 0 {
			s.QueryTimeout = 30 * time.Second
		}
		if s.MaxRows == 0 {
			s.MaxRows = 10000
		}
		if s.MaxBytes == 0 {
			s.MaxBytes = 16 << 20
		}
		if s.Output.Type == "" {
			s.Output.Type = "file"
		}
	}
}

func validateLogOutput(out LogOutput) error {
	switch out.Type {
	case "file":
		if out.Destination == "" {
			return fmt.Errorf("file output destination is required")
		}
		if IsStdout(out.Destination) && out.PerDatabaseFiles {
			return fmt.Errorf("stdout cannot use perDatabaseFiles")
		}
	case "stdout":
		if out.PerDatabaseFiles || (out.Destination != "" && !IsStdout(out.Destination)) {
			return fmt.Errorf("stdout cannot configure a file destination or perDatabaseFiles")
		}
	case "otlp":
		if out.Destination != "" || out.PerDatabaseFiles {
			return fmt.Errorf("otlp cannot configure a file destination or perDatabaseFiles")
		}
	default:
		return fmt.Errorf("output.type must be file, stdout, or otlp")
	}
	return nil
}

func (m *MetricsConfiguration) validateLogStreams() error {
	m.mergeLoggingConfig()
	m.mergeLogStreams()
	if m.LogDisable() != 1 || len(m.LogStreams) > 0 {
		if strings.TrimSpace(m.Logging.StateDirectory) == "" {
			return fmt.Errorf("log.stateDirectory is required")
		}
	}

	seen := map[string]bool{}
	for _, s := range m.LogStreams {
		if s.ID == "" || seen[s.ID] {
			return fmt.Errorf("logStreams IDs must be nonempty and unique")
		}
		seen[s.ID] = true
		if s.Interval <= 0 || s.QueryTimeout <= 0 || s.InitialLookback < 0 || s.MaxRows < 1 || s.MaxBytes < 1024 {
			return fmt.Errorf("log stream %q: intervals and limits must be positive", s.ID)
		}
		if strings.TrimSpace(s.Query) == "" || strings.TrimSpace(s.TimestampColumn) == "" {
			return fmt.Errorf("log stream %q: query and timestampColumn are required", s.ID)
		}
		for _, name := range s.Databases {
			if _, ok := m.Databases[name]; !ok {
				return fmt.Errorf("log stream %q: unknown database %q", s.ID, name)
			}
		}
		if err := validateLogOutput(s.Output); err != nil {
			return fmt.Errorf("log stream %q: %w", s.ID, err)
		}
		if s.Output.Type == "otlp" {
			if m.OTLP == nil {
				return fmt.Errorf("log stream %q: top-level otlp configuration is required", s.ID)
			}
			if err := validateOTLPTransport(m.OTLP); err != nil {
				return fmt.Errorf("log stream %q: %w", s.ID, err)
			}
		}
	}
	for _, s := range m.LogStreams {
		if !s.Output.PerDatabaseFiles {
			continue
		}
		names := s.Databases
		if len(names) == 0 {
			names = make([]string, 0, len(m.Databases))
			for name := range m.Databases {
				names = append(names, name)
			}
		}
		for _, name := range names {
			if name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
				return fmt.Errorf("database name %q cannot be used in an output filename", name)
			}
		}
	}
	return nil
}
