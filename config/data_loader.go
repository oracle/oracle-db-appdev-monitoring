// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v2"
)

// LoadMetrics loads the configured default and custom metric definitions.
func LoadMetrics(logger *slog.Logger, configuration *MetricsConfiguration) (map[string]*Metric, error) {
	metricsToScrape := DefaultMetrics(logger, configuration.Metrics)

	if len(configuration.CustomMetricsFiles()) == 0 {
		logger.Debug("No custom metrics defined.")
		return metricsToScrape, nil
	}

	for _, customMetricsFile := range configuration.CustomMetricsFiles() {
		metrics := &Metrics{}

		if err := loadMetricsConfig(customMetricsFile, metrics); err != nil {
			return nil, fmt.Errorf("failed to load custom metrics %s: %w", customMetricsFile, err)
		}

		logger.Info("Successfully loaded custom metrics from " + customMetricsFile)
		mergeMetrics(metricsToScrape, metrics)
	}

	return metricsToScrape, nil
}

func mergeMetrics(dst map[string]*Metric, metrics *Metrics) {
	for _, metric := range metrics.Metric {
		dst[metric.ID] = metric
	}
}

func loadYamlMetricsConfig(metricsFileName string, metrics *Metrics) error {
	yamlBytes, err := os.ReadFile(metricsFileName)
	if err != nil {
		return fmt.Errorf("cannot read the metrics config %s: %w", metricsFileName, err)
	}
	if err := yaml.Unmarshal(yamlBytes, metrics); err != nil {
		return fmt.Errorf("cannot unmarshal the metrics config %s: %w", metricsFileName, err)
	}
	return nil
}

func loadTomlMetricsConfig(customMetricsFile string, metrics *Metrics) error {
	if _, err := toml.DecodeFile(customMetricsFile, metrics); err != nil {
		return fmt.Errorf("cannot read the metrics config %s: %w", customMetricsFile, err)
	}
	return nil
}

func loadMetricsConfig(customMetricsFile string, metrics *Metrics) error {
	if strings.HasSuffix(customMetricsFile, "toml") {
		if err := loadTomlMetricsConfig(customMetricsFile, metrics); err != nil {
			return fmt.Errorf("cannot load toml based metrics: %w", err)
		}
	} else {
		if err := loadYamlMetricsConfig(customMetricsFile, metrics); err != nil {
			return fmt.Errorf("cannot load yaml based metrics: %w", err)
		}
	}
	metrics.normalizeIdentifiers()
	return nil
}
