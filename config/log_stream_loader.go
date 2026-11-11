// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v2"
)

// LoadLogStreams loads and validates a complete replacement before callers stop
// their current collectors. As with metrics.custom, later definitions override
// earlier ones by ID. Paths are relative to the process working directory.
func LoadLogStreams(configuration *MetricsConfiguration) ([]LogStreamConfig, error) {
	updated := *configuration
	updated.LogStreams = nil
	updated.mergeLoggingConfig()
	updated.mergeLogStreams()
	streams, err := defaultLogStreams(&updated)
	if err != nil {
		return nil, fmt.Errorf("load default log streams: %w", err)
	}
	indices := make(map[string]int)
	for i, stream := range streams {
		indices[stream.ID] = i
	}
	for _, path := range configuration.LogStreamFiles.Custom {
		definitions, err := loadLogStreamFile(path)
		if err != nil {
			return nil, fmt.Errorf("load log streams %q: %w", path, err)
		}
		for _, stream := range definitions {
			if i, ok := indices[stream.ID]; ok {
				streams[i] = stream
			} else {
				indices[stream.ID] = len(streams)
				streams = append(streams, stream)
			}
		}
	}
	updated.LogStreams = streams
	if err := updated.validateLogStreams(); err != nil {
		return nil, err
	}
	return updated.LogStreams, nil
}

func loadLogStreamFile(path string) ([]LogStreamConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, "toml") {
		// Normalize TOML into the same strict schema as YAML. In particular,
		// duration strings use YAML's time.Duration decoding in both formats.
		var document map[string]any
		if _, err := toml.Decode(string(content), &document); err != nil {
			return nil, err
		}
		content, err = yaml.Marshal(document)
		if err != nil {
			return nil, err
		}
	}
	return decodeLogStreams(content)
}

func decodeLogStreams(content []byte) ([]LogStreamConfig, error) {
	var document struct {
		Streams []LogStreamConfig `yaml:"logStreams"`
	}
	if err := yaml.UnmarshalStrict(content, &document); err != nil {
		return nil, err
	}
	return document.Streams, nil
}
