// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

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
