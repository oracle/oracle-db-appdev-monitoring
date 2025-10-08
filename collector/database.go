// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"database/sql"
	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
	"time"
)

const (
	ora01017code = 1017
	ora28000code = 28000
)

func (d *Database) UpMetric(exporterLabels map[string]string) prometheus.Metric {
	desc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Whether the Oracle database server is up.",
		nil,
		d.constLabels(exporterLabels),
	)
	return prometheus.MustNewConstMetric(desc,
		prometheus.GaugeValue,
		d.Up,
	)
}

func (d *Database) constLabels(labels map[string]string) map[string]string {
	labels["database"] = d.Name

	// configured per-database labels added to constLabels
	for label, value := range d.Config.Labels {
		labels[label] = value
	}
	return labels
}

func NewDatabase(logger *slog.Logger, dbname string, dbconfig DatabaseConfig) *Database {
	db := connect(logger, dbname, dbconfig)
	return &Database{
		Name:    dbname,
		Up:      0,
		Session: db,
		Config:  dbconfig,
		Valid:   true,
	}
}

// initCache resets the metrics cached. Used on startup and when metrics are reloaded.
func (d *Database) initCache(metrics map[string]*Metric) {
	d.MetricsCache = NewMetricsCache(metrics)
}

// WarmupConnectionPool serially acquires connections to "warm up" the connection pool.
// This is a workaround for a perceived bug in ODPI_C where rapid acquisition of connections
// results in a SIGABRT.
func (d *Database) WarmupConnectionPool(logger *slog.Logger) {
	var connections []*sql.Conn
	poolSize := d.Config.GetMaxOpenConns()
	if poolSize < 1 {
		poolSize = d.Config.GetPoolMaxConnections()
	}
	if poolSize > 100 { // defensively cap poolsize
		poolSize = 100
	}
	warmup := func(i int) {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, err := d.Session.Conn(ctx)
		if err != nil {
			logger.Debug("Failed to open database connection on warmup", "conn", i, "error", err, "database", d.Name)
			return
		}
		connections = append(connections, conn)
	}
	for i := 0; i < poolSize; i++ {
		warmup(i + 1)
	}

	logger.Debug("Warmed connection pool", "total", len(connections), "database", d.Name)
	for i, conn := range connections {
		if err := conn.Close(); err != nil {
			logger.Debug("Failed to return database connection to pool on warmup", "conn", i+1, "error", err, "database", d.Name)
		}
	}
}

func (d *Database) IsValid() bool {
	return d.Valid
}

func (d *Database) invalidate() {
	d.Valid = false
}
