// Copyright (c) 2021, 2024, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package main

import (
	"context"
	"net/http"
	"os"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	cversion "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"

	// Required for debugging
	// _ "net/http/pprof"

	"github.com/oracle/oracle-db-appdev-monitoring/alertlog"
	"github.com/oracle/oracle-db-appdev-monitoring/collector"
	"github.com/oracle/oracle-db-appdev-monitoring/vault"
)

var (
	// Version will be set at build time.
	Version            = "0.0.0.dev"
	metricPath         = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics. (env: TELEMETRY_PATH)").Default(getEnv("TELEMETRY_PATH", "/metrics")).String()
	defaultFileMetrics = kingpin.Flag("default.metrics", "File with default metrics in a TOML file. (env: DEFAULT_METRICS)").Default(getEnv("DEFAULT_METRICS", "default-metrics.toml")).String()
	customMetrics      = kingpin.Flag("custom.metrics", "File that may contain various custom metrics in a TOML file. (env: CUSTOM_METRICS)").Default(getEnv("CUSTOM_METRICS", "")).String()
	queryTimeout       = kingpin.Flag("query.timeout", "Query timeout (in seconds). (env: QUERY_TIMEOUT)").Default(getEnv("QUERY_TIMEOUT", "5")).Int()
	maxIdleConns       = kingpin.Flag("database.maxIdleConns", "Number of maximum idle connections in the connection pool. (env: DATABASE_MAXIDLECONNS)").Default(getEnv("DATABASE_MAXIDLECONNS", "0")).Int()
	maxOpenConns       = kingpin.Flag("database.maxOpenConns", "Number of maximum open connections in the connection pool. (env: DATABASE_MAXOPENCONNS)").Default(getEnv("DATABASE_MAXOPENCONNS", "10")).Int()
	scrapeInterval     = kingpin.Flag("scrape.interval", "Interval between each scrape. Default is to scrape on collect requests").Default("0s").Duration()
	logDisable         = kingpin.Flag("log.disable", "Set to 1 to disable alert logs").Default("0").Int()
	logInterval        = kingpin.Flag("log.interval", "Interval between log updates (e.g. 5s).").Default("15s").Duration()
	logDestination     = kingpin.Flag("log.destination", "File to output the alert log to. (env: LOG_DESTINATION)").Default(getEnv("LOG_DESTINATION", "/log/alert.log")).String()
	toolkitFlags       = webflag.AddFlags(kingpin.CommandLine, ":9161")
)

func main() {
	promLogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promLogConfig)
	kingpin.HelpFlag.Short('\n')
	kingpin.Version(version.Print("oracledb_exporter"))
	kingpin.Parse()
	logger := promlog.New(promLogConfig)
	user := os.Getenv("DB_USERNAME")
	password := os.Getenv("DB_PASSWORD")
	connectString := os.Getenv("DB_CONNECT_STRING")
	dbrole := os.Getenv("DB_ROLE")

	vaultName, useVault := os.LookupEnv("VAULT_ID")
	if useVault {
		level.Info(logger).Log("msg", "VAULT_ID env var is present so using OCI Vault", "vault_name", vaultName)
		password = vault.GetVaultSecret(vaultName, os.Getenv("VAULT_SECRET_NAME"))
	}

	freeOSMemInterval, enableFree := os.LookupEnv("FREE_INTERVAL")
	if enableFree {
		level.Info(logger).Log("msg", "FREE_INTERVAL env var is present, so will attempt to release OS memory", "free_interval", freeOSMemInterval)
	} else {
		level.Info(logger).Log("msg", "FREE_INTERVAL end var is not present, will not periodically attempt to release memory")
	}

	restartInterval, enableRestart := os.LookupEnv("RESTART_INTERVAL")
	if enableRestart {
		level.Info(logger).Log("msg", "RESTART_INTERVAL env var is present, so will restart my own process periodically", "restart_interval", restartInterval)
	} else {
		level.Info(logger).Log("msg", "RESTART_INTERVAL env var is not present, so will not restart myself periodically")
	}

	config := &collector.Config{
		User:               user,
		Password:           password,
		ConnectString:      connectString,
		DbRole:             dbrole,
		MaxOpenConns:       *maxOpenConns,
		MaxIdleConns:       *maxIdleConns,
		CustomMetrics:      *customMetrics,
		QueryTimeout:       *queryTimeout,
		DefaultMetricsFile: *defaultFileMetrics,
	}
	exporter, err := collector.NewExporter(logger, config)
	if err != nil {
		level.Error(logger).Log("msg", "unable to connect to DB", "error", err)
	}

	if *scrapeInterval != 0 {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go exporter.RunScheduledScrapes(ctx, *scrapeInterval)
	}

	prometheus.MustRegister(exporter)
	prometheus.MustRegister(cversion.NewCollector("oracledb_exporter"))

	level.Info(logger).Log("msg", "Starting oracledb_exporter", "version", Version)
	level.Info(logger).Log("msg", "Build context", "build", version.BuildContext())
	level.Info(logger).Log("msg", "Collect from: ", "metricPath", *metricPath)

	opts := promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	}
	http.Handle(*metricPath, promhttp.HandlerFor(prometheus.DefaultGatherer, opts))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Oracle DB Exporter " + Version + "</title></head><body><h1>Oracle DB Exporter " + Version + "</h1><p><a href='" + *metricPath + "'>Metrics</a></p></body></html>"))
	})

	// start a ticker to cause rebirth
	if enableRestart {
		duration, err := time.ParseDuration(restartInterval)
		if err != nil {
			level.Info(logger).Log("msg", "Could not parse RESTART_INTERVAL, so ignoring it")
		}
		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		go func() {
			<-ticker.C
			level.Info(logger).Log("msg", "Restarting the process...")
			executable, _ := os.Executable()
			execErr := syscall.Exec(executable, os.Args, os.Environ())
			if execErr != nil {
				panic(execErr)
			}
		}()
	}

	// start a ticker to free OS memory
	if enableFree {
		duration, err := time.ParseDuration(freeOSMemInterval)
		if err != nil {
			level.Info(logger).Log("msg", "Could not parse FREE_INTERVAL, so ignoring it")
		}
		memTicker := time.NewTicker(duration)
		defer memTicker.Stop()

		go func() {
			for {
				<-memTicker.C
				level.Info(logger).Log("msg", "attempting to free OS memory")
				debug.FreeOSMemory()
			}
		}()

	}

	// start the log exporter
	if *logDisable == 1 {
		level.Info(logger).Log("msg", "log.disable set to 1, so will not export the alert logs")
	} else {
		level.Info(logger).Log("msg", "Exporting alert logs to "+*logDestination)
		logTicker := time.NewTicker(*logInterval)
		defer logTicker.Stop()

		go func() {
			for {
				<-logTicker.C
				level.Debug(logger).Log("msg", "updating alert log")
				alertlog.UpdateLog(*logDestination, logger, exporter.GetDB())
			}
		}()
	}

	// start the main server thread
	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		level.Error(logger).Log("msg", "Listening error", "error", err)
		os.Exit(1)
	}

}

// getEnv returns the value of an environment variable, or returns the provided fallback value
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
