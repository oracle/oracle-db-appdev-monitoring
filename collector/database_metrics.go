// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"github.com/oracle/oracle-db-appdev-monitoring/db"
	"github.com/prometheus/client_golang/prometheus"
)

func databaseConstLabels(labels map[string]string, database *db.Database) map[string]string {
	labels[database.DatabaseLabel] = database.Name
	for label, value := range database.Config.Labels {
		labels[label] = value
	}
	return labels
}

func (e *Exporter) databaseUpMetric(database *db.Database) prometheus.Metric {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Whether the Oracle AI Database server is up.",
		nil,
		databaseConstLabels(e.constLabels(), database),
	)
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, database.Up())
}
