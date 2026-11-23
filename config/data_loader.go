// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v2"
)

// LoadMetrics loads the configured default and custom metric definitions.
func LoadMetrics(logger *slog.Logger, configuration *MetricsConfiguration) (map[string]*Metric, error) {
	metrics, _, err := LoadMetricsWithFingerprints(logger, configuration)
	return metrics, err
}

// LoadMetricsWithFingerprints returns definitions and fingerprints of the bytes
// decoded from each custom file in configuration order.
func LoadMetricsWithFingerprints(logger *slog.Logger, configuration *MetricsConfiguration) (map[string]*Metric, [][sha256.Size]byte, error) {
	metricsToScrape := DefaultMetrics(logger, configuration.Metrics)
	customFiles := configuration.CustomMetricsFiles()

	if len(customFiles) == 0 {
		logger.Debug("No custom metrics defined.")
		return metricsToScrape, nil, nil
	}

	fingerprints := make([][sha256.Size]byte, len(customFiles))
	for i, customMetricsFile := range customFiles {
		metrics := &Metrics{}

		contents, err := loadMetricsConfig(customMetricsFile, metrics)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load custom metrics %s: %w", customMetricsFile, err)
		}
		fingerprints[i] = sha256.Sum256(contents)

		logger.Info("Successfully loaded custom metrics from " + customMetricsFile)
		mergeMetrics(metricsToScrape, metrics)
	}

	return metricsToScrape, fingerprints, nil
}

func mergeMetrics(dst map[string]*Metric, metrics *Metrics) {
	for _, metric := range metrics.Metric {
		dst[metric.ID] = metric
	}
}

func loadMetricsConfig(metricsFileName string, metrics *Metrics) ([]byte, error) {
	contents, err := os.ReadFile(metricsFileName)
	if err != nil {
		return nil, fmt.Errorf("cannot read the metrics config %s: %w", metricsFileName, err)
	}
	if strings.HasSuffix(metricsFileName, "toml") {
		if err := toml.Unmarshal(contents, metrics); err != nil {
			return nil, fmt.Errorf("cannot decode TOML metrics config %s: %w", metricsFileName, err)
		}
	} else if err := yaml.Unmarshal(contents, metrics); err != nil {
		return nil, fmt.Errorf("cannot decode YAML metrics config %s: %w", metricsFileName, err)
	}
	metrics.normalizeIdentifiers()
	return contents, nil
}
