// Copyright (c) 2024, 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl

package collector

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
)

// isScrapeMetric returns true if a metric should be scraped. Metrics may not be scraped if they have a custom scrape interval,
// and the time since the last scrape is less than the custom scrape interval.
// If there is no tick time or last known tick, the metric is always scraped.
func (e *Exporter) isScrapeMetric(logger *slog.Logger, tick *time.Time, metric *config.Metric, d *db.Database) bool {
	// If the metric isn't enabled for the database, don't scrape it.
	if !metric.IsEnabledForDatabase(d.Name) {
		return false
	}

	// Always scrape the metric if we don't have a current tick.
	if tick == nil {
		return true
	}
	// If the metric doesn't have a custom scrape interval, scrape it.
	interval, ok := getScrapeInterval(logger, metric.Context, metric.ScrapeInterval)
	if !ok {
		return true
	}
	lastScraped := e.metricsCache(d).GetLastScraped(metric)
	shouldScrape := lastScraped == nil ||
		// If the metric's scrape interval is less than the time elapsed since the last scrape,
		// we should scrape the metric.
		interval < tick.Sub(*lastScraped)
	return shouldScrape
}

func getScrapeInterval(logger *slog.Logger, context, scrapeInterval string) (time.Duration, bool) {
	if len(scrapeInterval) > 0 {
		si, err := time.ParseDuration(scrapeInterval)
		if err != nil {
			logger.Error("Unable to convert scrapeinterval to duration (metric=" + context + ")")
			return 0, false
		}
		return si, true
	}
	return 0, false
}

func getQueryTimeout(logger *slog.Logger, metric *config.Metric, d *db.Database) time.Duration {
	if len(metric.QueryTimeout) > 0 {
		qt, err := time.ParseDuration(metric.QueryTimeout)
		if err != nil {
			logger.Error("Unable to convert querytimeout to duration (metric=" + metric.Context + ")")
			return time.Duration(d.Config.GetQueryTimeout()) * time.Second
		}
		return qt
	}
	return time.Duration(d.Config.GetQueryTimeout()) * time.Second
}

func parseFloat(logger *slog.Logger, metric, metricHelp string, row map[string]string) (float64, bool) {
	value, ok := row[metric]
	if !ok || value == "<nil>" {
		// treat nil value as 0
		return 0.0, ok
	}
	valueFloat, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		logger.Error("Unable to convert current value to float (metric=" + metric +
			",metricHelp=" + metricHelp + ",value=<" + row[metric] + ">)")
		return -1, false
	}
	return valueFloat, true
}
