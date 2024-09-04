// Copyright (c) 2024, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl

package collector

import (
	"github.com/go-kit/log/level"
	"strconv"
	"time"
)

// isScrapeMetric returns true if a metric should be scraped. Metrics may not be scraped if they have a custom scrape interval,
// and the time since the last scrape is less than the custom scrape interval.
// If there is no tick time or last known tick, the metric is always scraped.
func (e *Exporter) isScrapeMetric(tick *time.Time, metric Metric) bool {
	// Always scrape the metric if we don't have a current or last known tick.
	if tick == nil || e.lastTick == nil {
		return true
	}
	// If the metric doesn't have a custom scrape interval, scrape it.
	interval, ok := e.getScrapeInterval(metric.Context, metric.ScrapeInterval)
	if !ok {
		return true
	}
	// If the metric's scrape interval is less than the time elapsed since the last scrape,
	// we should scrape the metric.
	return interval < tick.Sub(*e.lastTick)
}

func (e *Exporter) getScrapeInterval(context, scrapeInterval string) (time.Duration, bool) {
	if len(scrapeInterval) > 0 {
		si, err := time.ParseDuration(scrapeInterval)
		if err != nil {
			level.Error(e.logger).Log("msg", "Unable to convert scrapeinterval to duration (metric="+context+")")
			return 0, false
		}
		return si, true
	}
	return 0, false
}

func (e *Exporter) getQueryTimeout(metric Metric) time.Duration {
	if len(metric.QueryTimeout) > 0 {
		qt, err := time.ParseDuration(metric.QueryTimeout)
		if err != nil {
			level.Error(e.logger).Log("msg", "Unable to convert querytimeout to duration (metric="+metric.Context+")")
			return time.Duration(e.config.QueryTimeout) * time.Second
		}
		return qt
	}
	return time.Duration(e.config.QueryTimeout) * time.Second
}

func (e *Exporter) parseFloat(metric, metricHelp string, row map[string]string) (float64, bool) {
	value, ok := row[metric]
	if !ok {
		return -1, ok
	}
	valueFloat, err := strconv.ParseFloat(value, 64)
	if err != nil {
		level.Error(e.logger).Log("msg", "Unable to convert current value to float (metric="+metric+
			",metricHelp="+metricHelp+",value=<"+row[metric]+">)")
		return -1, false
	}
	return valueFloat, true
}
