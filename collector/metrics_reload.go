// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import "github.com/oracle/oracle-db-appdev-monitoring/config"

func (e *Exporter) reloadMetrics() bool {
	metricsToScrape, hashes, err := config.LoadMetricsWithFingerprints(e.logger, e.MetricsConfiguration)
	if err != nil {
		e.logger.Error("failed to reload metrics; continuing with last known good metrics", "error", err)
		return false
	}

	e.metricsToScrape = metricsToScrape
	e.customMetricsHashes = hashes
	e.initCache()
	// Metric definitions that no longer exist must not keep reporting the duration of their last scrape.
	// The vector is nil when metrics.perMetricScrapeDuration.enabled is false.
	if e.metricScrapeDuration != nil {
		e.metricScrapeDuration.Reset()
	}
	return true
}
