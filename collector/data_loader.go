// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

func (e *Exporter) reloadMetrics() {
	// Truncate metricsToScrape
	e.metricsToScrape.Metric = []*Metric{}

	// Load default metrics
	defaultMetrics := e.DefaultMetrics()
	e.metricsToScrape.Metric = defaultMetrics.Metric

	// If custom metrics, load it
	if len(e.CustomMetricsFiles()) > 0 {
		for _, _customMetrics := range e.CustomMetricsFiles() {
			metrics := &Metrics{}

			if err := loadMetricsConfig(_customMetrics, metrics); err != nil {
				e.logger.Error("failed to load custom metrics", "error", err)
				panic(errors.New("Error while loading " + _customMetrics))
			} else {
				e.logger.Info("Successfully loaded custom metrics from " + _customMetrics)
			}

			e.metricsToScrape.Metric = append(e.metricsToScrape.Metric, metrics.Metric...)
		}
	} else {
		e.logger.Debug("No custom metrics defined.")
	}
	e.initCache()
}

func loadYamlMetricsConfig(_metricsFileName string, metrics *Metrics) error {
	yamlBytes, err := os.ReadFile(_metricsFileName)
	if err != nil {
		return fmt.Errorf("cannot read the metrics config %s: %w", _metricsFileName, err)
	}
	if err := yaml.Unmarshal(yamlBytes, metrics); err != nil {
		return fmt.Errorf("cannot unmarshal the metrics config %s: %w", _metricsFileName, err)
	}
	return nil
}

func loadTomlMetricsConfig(_customMetrics string, metrics *Metrics) error {
	if _, err := toml.DecodeFile(_customMetrics, metrics); err != nil {
		return fmt.Errorf("cannot read the metrics config %s: %w", _customMetrics, err)
	}
	return nil
}

func loadMetricsConfig(_customMetrics string, metrics *Metrics) error {
	if strings.HasSuffix(_customMetrics, "toml") {
		if err := loadTomlMetricsConfig(_customMetrics, metrics); err != nil {
			return fmt.Errorf("cannot load toml based metrics: %w", err)
		}
	} else {
		if err := loadYamlMetricsConfig(_customMetrics, metrics); err != nil {
			return fmt.Errorf("cannot load yaml based metrics: %w", err)
		}
	}
	return nil
}
