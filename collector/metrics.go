// Copyright (c) 2024, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl

package collector

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// isScrapeMetric returns true if a metric should be scraped. Metrics may not be scraped if they have a custom scrape interval,
// and the time since the last scrape is less than the custom scrape interval.
// If there is no tick time or last known tick, the metric is always scraped.
func (e *Exporter) isScrapeMetric(tick *time.Time, metric *Metric, d *Database) bool {
	// If the metric isn't enabled for the database, don't scrape it.
	if !metric.IsEnabledForDatabase(d) {
		return false
	}

	// Always scrape the metric if we don't have a current tick.
	if tick == nil {
		return true
	}
	// If the metric doesn't have a custom scrape interval, scrape it.
	interval, ok := e.getScrapeInterval(metric.Context, metric.ScrapeInterval)
	if !ok {
		return true
	}
	lastScraped := d.MetricsCache.GetLastScraped(metric)
	shouldScrape := lastScraped == nil ||
		// If the metric's scrape interval is less than the time elapsed since the last scrape,
		// we should scrape the metric.
		interval < tick.Sub(*lastScraped)
	if shouldScrape {
		d.MetricsCache.SetLastScraped(metric, lastScraped)
	}
	return shouldScrape
}

func (e *Exporter) getScrapeInterval(context, scrapeInterval string) (time.Duration, bool) {
	if len(scrapeInterval) > 0 {
		si, err := time.ParseDuration(scrapeInterval)
		if err != nil {
			e.logger.Error("Unable to convert scrapeinterval to duration (metric=" + context + ")")
			return 0, false
		}
		return si, true
	}
	return 0, false
}

func (e *Exporter) getQueryTimeout(metric *Metric, d *Database) time.Duration {
	if len(metric.QueryTimeout) > 0 {
		qt, err := time.ParseDuration(metric.QueryTimeout)
		if err != nil {
			e.logger.Error("Unable to convert querytimeout to duration (metric=" + metric.Context + ")")
			return time.Duration(d.Config.GetQueryTimeout()) * time.Second
		}
		return qt
	}
	return time.Duration(d.Config.GetQueryTimeout()) * time.Second
}

func (e *Exporter) parseFloat(metric, metricHelp string, row map[string]string) (float64, bool) {
	value, ok := row[metric]
	if !ok || value == "<nil>" {
		// treat nil value as 0
		return 0.0, ok
	}
	valueFloat, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		e.logger.Error("Unable to convert current value to float (metric=" + metric +
			",metricHelp=" + metricHelp + ",value=<" + row[metric] + ">)")
		return -1, false
	}
	return valueFloat, true
}

func (m *Metric) GetLabels() []string {
	if len(m.FieldToAppend) == 0 {
		return m.Labels
	}
	// Do not include FieldToAppend in metric labels,
	// as this field is appended to the metric FQDN.
	var labels []string
	for _, label := range m.Labels {
		if label != m.FieldToAppend {
			labels = append(labels, label)
		}
	}
	return labels
}

// IsEnabledForDatabase checks if a metric is enabled for a database.
// If the m.Databases slice is nil, the metric is enabled for all databases.
// If the m.Databases slice contains the database name, the metric is enabled for that database.
// Otherwise, the metric is disabled for all databases (non-nil, empty m.Databases slice)
func (m *Metric) IsEnabledForDatabase(d *Database) bool {
	if m.Databases == nil || slices.Contains(m.Databases, d.Name) {
		return true
	}
	return false
}
