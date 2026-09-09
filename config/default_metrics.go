// Copyright (c) 2021, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package config

import (
	_ "embed"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

//go:embed default_metrics.toml
var defaultMetricsToml string

// DefaultMetrics is a somewhat hacky way to load the default metrics.
func DefaultMetrics(logger *slog.Logger, metricsConfig MetricsFilesConfig) map[string]*Metric {
	var metricsToScrape Metrics
	if metricsConfig.Default != "" {
		if err := loadMetricsConfig(filepath.Clean(metricsConfig.Default), &metricsToScrape); err != nil {
			logger.Error(fmt.Sprintf("there was an issue while loading specified default metrics file at: %s, proceeding to run with default metrics.", metricsConfig.Default),
				"error", err)
		} else {
			return metricsToScrape.toMap()
		}
	}

	if _, err := toml.Decode(defaultMetricsToml, &metricsToScrape); err != nil {
		logger.Error("failed to load default metrics", "error", err)
		return map[string]*Metric{}
	}
	metricsToScrape.normalizeIdentifiers()
	return metricsToScrape.toMap()
}
