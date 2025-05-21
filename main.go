// Copyright (c) 2021, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"

	"github.com/godror/godror/dsn"
	"github.com/prometheus/client_golang/prometheus"
	cversion "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/alecthomas/kingpin/v2"

	// Required for debugging
	// _ "net/http/pprof"

	"github.com/oracle/oracle-db-appdev-monitoring/alertlog"
	"github.com/oracle/oracle-db-appdev-monitoring/collector"
)

var (
	// Version will be set at build time.
	Version            = "0.0.0.dev"
	configFile         = kingpin.Flag("config.file", "File with metrics exporter configuration. (env: CONFIG_FILE)").Default(getEnv("CONFIG_FILE", "")).String()
	metricPath         = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics. (env: TELEMETRY_PATH)").Default(getEnv("TELEMETRY_PATH", "/metrics")).String()
	defaultFileMetrics = kingpin.Flag("default.metrics", "File with default metrics in a TOML file. (env: DEFAULT_METRICS)").Default(getEnv("DEFAULT_METRICS", "default-metrics.toml")).String()
	customMetrics      = kingpin.Flag("custom.metrics", "Comma separated list of file(s) that contain various custom metrics in a TOML format. (env: CUSTOM_METRICS)").Default(getEnv("CUSTOM_METRICS", "")).String()
	queryTimeout       = kingpin.Flag("query.timeout", "Query timeout (in seconds). (env: QUERY_TIMEOUT)").Default(getEnv("QUERY_TIMEOUT", "5")).Int()
	maxIdleConns       = kingpin.Flag("database.maxIdleConns", "Number of maximum idle connections in the connection pool. (env: DATABASE_MAXIDLECONNS)").Default(getEnv("DATABASE_MAXIDLECONNS", "10")).Int()
	maxOpenConns       = kingpin.Flag("database.maxOpenConns", "Number of maximum open connections in the connection pool. (env: DATABASE_MAXOPENCONNS)").Default(getEnv("DATABASE_MAXOPENCONNS", "10")).Int()
	poolIncrement      = kingpin.Flag("database.poolIncrement", "Connection increment when the connection pool reaches max capacity. (env: DATABASE_POOLINCREMENT)").Default(getEnv("DATABASE_POOLINCREMENT", "-1")).Int()
	poolMaxConnections = kingpin.Flag("database.poolMaxConnections", "Maximum number of connections in the connection pool. (env: DATABASE_POOLMAXCONNECTIONS)").Default(getEnv("DATABASE_POOLMAXCONNECTIONS", "-1")).Int()
	poolMinConnections = kingpin.Flag("database.poolMinConnections", "Minimum number of connections in the connection pool. (env: DATABASE_POOLMINCONNECTIONS)").Default(getEnv("DATABASE_POOLMINCONNECTIONS", "-1")).Int()
	scrapeInterval     = kingpin.Flag("scrape.interval", "Interval between each scrape. Default is to scrape on collect requests.").Default("0s").Duration()
	logDisable         = kingpin.Flag("log.disable", "Set to 1 to disable alert logs").Default("0").Int()
	logInterval        = kingpin.Flag("log.interval", "Interval between log updates (e.g. 5s).").Default("15s").Duration()
	logDestination     = kingpin.Flag("log.destination", "File to output the alert log to. (env: LOG_DESTINATION)").Default(getEnv("LOG_DESTINATION", "/log/alert.log")).String()
	toolkitFlags       = webflag.AddFlags(kingpin.CommandLine, ":9161")
)

func main() {
	promLogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promLogConfig)
	kingpin.HelpFlag.Short('\n')
	kingpin.Version(version.Print("oracledb_exporter"))
	kingpin.Parse()
	logger := promslog.New(promLogConfig)
	user := os.Getenv("DB_USERNAME")
	password := os.Getenv("DB_PASSWORD")
	connectString := os.Getenv("DB_CONNECT_STRING")
	dbrole := os.Getenv("DB_ROLE")
	tnsadmin := os.Getenv("TNS_ADMIN")
	// externalAuth - Default to user/password but if no password is supplied then will automagically set to true
	externalAuth := false

	freeOSMemInterval, enableFree := os.LookupEnv("FREE_INTERVAL")
	if enableFree {
		logger.Info("FREE_INTERVAL env var is present, so will attempt to release OS memory", "free_interval", freeOSMemInterval)
	} else {
		logger.Info("FREE_INTERVAL end var is not present, will not periodically attempt to release memory")
	}

	restartInterval, enableRestart := os.LookupEnv("RESTART_INTERVAL")
	if enableRestart {
		logger.Info("RESTART_INTERVAL env var is present, so will restart my own process periodically", "restart_interval", restartInterval)
	} else {
		logger.Info("RESTART_INTERVAL env var is not present, so will not restart myself periodically")
	}

	config := &collector.Config{
		ConfigFile:         *configFile,
		User:               user,
		Password:           password,
		ConnectString:      connectString,
		DbRole:             dsn.AdminRole(dbrole),
		ConfigDir:          tnsadmin,
		ExternalAuth:       externalAuth,
		MaxOpenConns:       *maxOpenConns,
		MaxIdleConns:       *maxIdleConns,
		PoolIncrement:      *poolIncrement,
		PoolMaxConnections: *poolMaxConnections,
		PoolMinConnections: *poolMinConnections,
		CustomMetrics:      *customMetrics,
		QueryTimeout:       *queryTimeout,
		DefaultMetricsFile: *defaultFileMetrics,
		ScrapeInterval:     *scrapeInterval,
		LoggingConfig: collector.LoggingConfig{
			LogDisable:     logDisable,
			LogInterval:    logInterval,
			LogDestination: *logDestination,
		},
	}
	m, err := collector.LoadMetricsConfiguration(logger, config, *metricPath)
	if err != nil {
		logger.Error("unable to load metrics configuration", "error", err)
		return
	}

	for dbname, db := range m.Databases {
		if db.GetMaxOpenConns() > 0 {
			logger.Info(dbname + " database max idle connections is greater than 0, so will use go-sql connection pool and pooling settings will be ignored")
		} else {
			logger.Info(dbname + " database max idle connections is 0, so will use Oracle connection pool. Tune with database pooling settings")
		}
	}
	exporter := collector.NewExporter(logger, m)
	if exporter.ScrapeInterval() != 0 {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go exporter.RunScheduledScrapes(ctx)
	}

	prometheus.MustRegister(exporter)
	prometheus.MustRegister(cversion.NewCollector("oracledb_exporter"))

	logger.Info("Starting oracledb_exporter", "version", Version)
	logger.Info("Build context", "build", version.BuildContext())
	logger.Info("Collect from: ", "metricPath", m.MetricsPath)

	opts := promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	}
	http.Handle(m.MetricsPath, promhttp.HandlerFor(prometheus.DefaultGatherer, opts))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Oracle DB Exporter " + Version + "</title></head><body><h1>Oracle DB Exporter " + Version + "</h1><p><a href='" + m.MetricsPath + "'>Metrics</a></p></body></html>"))
	})

	// start a ticker to cause rebirth
	if enableRestart {
		duration, err := time.ParseDuration(restartInterval)
		if err != nil {
			logger.Info("Could not parse RESTART_INTERVAL, so ignoring it")
		}
		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		go func() {
			<-ticker.C
			logger.Info("Restarting the process...")
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
			logger.Info("Could not parse FREE_INTERVAL, so ignoring it")
		}
		memTicker := time.NewTicker(duration)
		defer memTicker.Stop()

		go func() {
			for {
				<-memTicker.C
				logger.Info("attempting to free OS memory")
				debug.FreeOSMemory()
			}
		}()

	}

	// start the log exporter
	if m.LogDisable() == 1 {
		logger.Info("log.disable set to 1, so will not export the alert logs")
	} else {
		logger.Info(fmt.Sprintf("Exporting alert logs to %s", m.LogDestination()))
		logTicker := time.NewTicker(m.LogInterval())
		defer logTicker.Stop()

		go func() {
			for {
				<-logTicker.C
				logger.Debug("updating alert log")
				for _, db := range exporter.GetDBs() {
					alertlog.UpdateLog(m.LogDestination(), logger, db)
				}

			}
		}()
	}
	
	// start the main server thread
	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		logger.Error("Listening error", "error", err)
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
