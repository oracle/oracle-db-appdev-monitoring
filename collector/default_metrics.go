// Copyright (c) 2021, 2024, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package collector

import (
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/go-kit/log/level"
)

//go:embed default_metrics.toml
var defaultMetricsToml string

// DefaultMetrics is a somewhat hacky way to load the default metrics
func (e *Exporter) DefaultMetrics() Metrics {
	var metricsToScrape Metrics
	if e.config.DefaultMetricsFile != "" {
		if _, err := toml.DecodeFile(filepath.Clean(e.config.DefaultMetricsFile), &metricsToScrape); err != nil {
			level.Error(e.logger).Log("msg", fmt.Sprintf("there was an issue while loading specified default metrics file at: "+e.config.DefaultMetricsFile+", proceeding to run with default metrics."),
				"error", err)
		}
		return metricsToScrape
	}

	if _, err := toml.Decode(defaultMetricsToml, &metricsToScrape); err != nil {
		level.Error(e.logger).Log(err)
		panic(errors.New("Error while loading " + defaultMetricsToml))
	}
	return metricsToScrape
}
