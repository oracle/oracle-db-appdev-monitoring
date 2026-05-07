// Copyright (c) 2021, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/client_golang/prometheus"
	cversion "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"

	// Required for debugging
	// _ "net/http/pprof"

	"github.com/oracle/oracle-db-appdev-monitoring/alertlog"
	"github.com/oracle/oracle-db-appdev-monitoring/collector"
)

var (
	// Version will be set at build time.
	Version = "0.0.0.dev"
)

func syncBuildVersion() {
	if version.Version == "" {
		version.Version = Version
	}
}

func parseConfigFile(args []string, getenv func(string) string, output io.Writer) (string, error) {
	flags := flag.NewFlagSet("oracledb_exporter", flag.ContinueOnError)
	var flagOutput bytes.Buffer
	flags.SetOutput(&flagOutput)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage of oracledb_exporter:\n")
		fmt.Fprintf(flags.Output(), "  --config.file string\n")
		fmt.Fprintf(flags.Output(), "        File with metrics exporter configuration. (env: CONFIG_FILE)\n")
	}

	configFile := flags.String("config.file", getenv("CONFIG_FILE"), "File with metrics exporter configuration. (env: CONFIG_FILE)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) && output != nil {
			_, _ = output.Write(flagOutput.Bytes())
		}
		return "", err
	}
	if flags.NArg() > 0 {
		return "", fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*configFile) == "" {
		return "", errors.New("config file is required; set --config.file or CONFIG_FILE")
	}
	return *configFile, nil
}

func promslogConfig(levelValue, formatValue string) (*promslog.Config, error) {
	level := promslog.NewLevel()
	if err := level.Set(levelValue); err != nil {
		return nil, err
	}
	format := promslog.NewFormat()
	if err := format.Set(formatValue); err != nil {
		return nil, err
	}
	return &promslog.Config{
		Level:  level,
		Format: format,
	}, nil
}

func landingPageHTML(metricsPath string) string {
	escapedVersion := html.EscapeString(Version)
	escapedMetricsPath := html.EscapeString(metricsPath)
	return "<html><head><title>Oracle DB Exporter " + escapedVersion + "</title></head><body><h1>Oracle DB Exporter " + escapedVersion + "</h1><p><a href='" + escapedMetricsPath + "'>Metrics</a></p></body></html>"
}

func landingPageHandler(metricsPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(landingPageHTML(metricsPath)))
	}
}

func main() {
	syncBuildVersion()

	configFile, err := parseConfigFile(os.Args[1:], os.Getenv, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	bootstrapLogConfig, _ := promslogConfig("info", "logfmt")
	bootstrapLogger := promslog.New(bootstrapLogConfig)
	config := &collector.Config{ConfigFile: configFile}
	m, err := collector.LoadMetricsConfiguration(bootstrapLogger, config)
	if err != nil {
		bootstrapLogger.Error("unable to load metrics configuration file", "error", err)
		os.Exit(1)
	}

	promLogConfig, err := promslogConfig(m.Logging.Level, m.Logging.Format)
	if err != nil {
		bootstrapLogger.Error("invalid logging configuration", "error", err)
		os.Exit(1)
	}
	logger := promslog.New(promLogConfig)

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
	http.HandleFunc("/", landingPageHandler(m.MetricsPath))

	// start a ticker to cause rebirth
	if enableRestart {
		duration, err := time.ParseDuration(restartInterval)
		if err != nil {
			logger.Info("Could not parse RESTART_INTERVAL, so ignoring it")
		} else if duration <= 0 {
			logger.Info("RESTART_INTERVAL must be greater than zero, so ignoring it")
		} else {
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
	}

	// start a ticker to free OS memory
	if enableFree {
		duration, err := time.ParseDuration(freeOSMemInterval)
		if err != nil {
			logger.Info("Could not parse FREE_INTERVAL, so ignoring it")
		} else if duration <= 0 {
			logger.Info("FREE_INTERVAL must be greater than zero, so ignoring it")
		} else {
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

	}

	// start the log exporter
	if m.LogDisable() == 1 {
		logger.Info("log.disable set to 1, so will not export the alert logs")
	} else {
		if m.LogPerDatabaseFiles() {
			logger.Info(fmt.Sprintf("Exporting an alert log file per database to %s", filepath.Dir(m.LogDestination())))
		} else {
			logger.Info(fmt.Sprintf("Exporting alert logs to %s", m.LogDestination()))
		}
		logTicker := time.NewTicker(m.LogInterval())
		defer logTicker.Stop()

		go func() {
			for {
				<-logTicker.C
				logger.Debug("updating alert log")
				for _, db := range exporter.GetDBs() {
					alertlog.UpdateLog(m.LogDestination(), m.LogPerDatabaseFiles(), logger, db)
				}

			}
		}()
	}

	server := &http.Server{
		ReadHeaderTimeout: m.Web.GetReadHeaderTimeout(),
		ReadTimeout:       m.Web.GetReadTimeout(),
		IdleTimeout:       m.Web.GetIdleTimeout(),
	}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- web.ListenAndServe(server, m.Web.Flags(), logger)
	}()

	go exporter.InitializeDatabases()

	// start the main server thread
	if err := <-serverErr; err != nil {
		logger.Error("Listening error", "error", err)
		os.Exit(1)
	}

}
