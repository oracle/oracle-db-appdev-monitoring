// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import _ "embed"

//go:embed default_log_streams.yaml
var defaultLogStreamsYAML []byte

//go:embed default_log_streams_with_levels.yaml
var defaultLogStreamsWithLevelsYAML []byte

// defaultLogStreams loads ordinary definitions. Legacy log options apply only
// to the embedded defaults; replacement files and custom overrides stand alone.
func defaultLogStreams(m *MetricsConfiguration) ([]LogStreamConfig, error) {
	if m.LogDisable() == 1 {
		return nil, nil
	}
	if m.LogStreamFiles.Default != "" {
		return loadLogStreamFile(m.LogStreamFiles.Default)
	}
	content := defaultLogStreamsYAML
	if m.LogMessageLevelEnabled() {
		content = defaultLogStreamsWithLevelsYAML
	}
	streams, err := decodeLogStreams(content)
	if err != nil {
		return nil, err
	}
	for i := range streams {
		s := &streams[i]
		s.Interval = m.LogInterval()
		s.QueryTimeout = m.Logging.QueryTimeout
		s.InitialLookback = m.Logging.InitialLookback
		s.MaxRows = m.Logging.MaxRows
		s.MaxBytes = m.Logging.MaxBytes
		s.Output = m.AlertLogOutput()
	}
	return streams, nil
}
