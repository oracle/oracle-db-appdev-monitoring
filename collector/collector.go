// Copyright (c) 2021, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package collector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/godror/godror"
	"github.com/godror/godror/dsn"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	hashMap      = make(map[int][]byte)
	namespace    = "oracledb"
	exporterName = "exporter"
)

// ScrapResult is container structure for error handling
type ScrapeResult struct {
	Err         error
	Metric      Metric
	ScrapeStart time.Time
}

func maskDsn(dsn string) string {
	parts := strings.Split(dsn, "@")
	if len(parts) > 1 {
		maskedURL := "***@" + parts[1]
		return maskedURL
	}
	return dsn
}

// NewExporter creates a new Exporter instance
func NewExporter(logger *slog.Logger, cfg *Config) (*Exporter, error) {
	e := &Exporter{
		mu:            &sync.Mutex{},
		user:          cfg.User,
		password:      cfg.Password,
		connectString: cfg.ConnectString,
		configDir:     cfg.ConfigDir,
		externalAuth:  cfg.ExternalAuth,
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_scrape_duration_seconds",
			Help:      "Duration of the last scrape of metrics from Oracle DB.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "scrapes_total",
			Help:      "Total number of times Oracle DB was scraped for metrics.",
		}),
		scrapeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "scrape_errors_total",
			Help:      "Total number of times an error occured scraping a Oracle database.",
		}, []string{"collector"}),
		error: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_scrape_error",
			Help:      "Whether the last scrape of metrics from Oracle DB resulted in an error (1 for error, 0 for success).",
		}),
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Whether the Oracle database server is up.",
		}),
		dbtypeGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "dbtype",
			Help:      "Type of database the exporter is connected to (0=non-CDB, 1=CDB, >1=PDB).",
		}),
		logger: logger,
		config: cfg,
	}
	e.metricsToScrape = e.DefaultMetrics()
	err := e.connect()
	return e, err
}

// Describe describes all the metrics exported by the Oracle DB exporter.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	// We cannot know in advance what metrics the exporter will generate
	// So we use the poor man's describe method: Run a collect
	// and send the descriptors of all the collected metrics. The problem
	// here is that we need to connect to the Oracle DB. If it is currently
	// unavailable, the descriptors will be incomplete. Since this is a
	// stand-alone exporter and not used as a library within other code
	// implementing additional metrics, the worst that can happen is that we
	// don't detect inconsistent metrics created by this exporter
	// itself. Also, a change in the monitored Oracle instance may change the
	// exported metrics during the runtime of the exporter.

	metricCh := make(chan prometheus.Metric)
	doneCh := make(chan struct{})

	go func() {
		for m := range metricCh {
			ch <- m.Desc()
		}
		close(doneCh)
	}()

	e.Collect(metricCh)
	close(metricCh)
	<-doneCh
}

// Collect implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	// they are running scheduled scrapes we should only scrape new data
	// on the interval
	if e.scrapeInterval != nil && *e.scrapeInterval != 0 {
		// read access must be checked
		e.mu.Lock()
		for _, r := range e.scrapeResults {
			ch <- r
		}
		e.mu.Unlock()
		return
	}

	// otherwise do a normal scrape per request
	e.mu.Lock() // ensure no simultaneous scrapes
	defer e.mu.Unlock()
	e.scrape(ch, nil)
	ch <- e.duration
	ch <- e.totalScrapes
	ch <- e.error
	e.scrapeErrors.Collect(ch)
	ch <- e.up
	ch <- e.dbtypeGauge
}

// RunScheduledScrapes is only relevant for users of this package that want to set the scrape on a timer
// rather than letting it be per Collect call
func (e *Exporter) RunScheduledScrapes(ctx context.Context, si time.Duration) {
	e.scrapeInterval = &si

	e.doScrape(time.Now())

	ticker := time.NewTicker(si)
	defer ticker.Stop()

	for {
		select {
		case tick := <-ticker.C:

			e.doScrape(tick)
		case <-ctx.Done():
			return
		}
	}
}

func (e *Exporter) doScrape(tick time.Time) {
	e.mu.Lock() // ensure no simultaneous scrapes
	e.scheduledScrape(&tick)
	e.lastTick = &tick
	e.mu.Unlock()
}

func (e *Exporter) scheduledScrape(tick *time.Time) {
	metricCh := make(chan prometheus.Metric, 5)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.scrapeResults = []prometheus.Metric{}
		for {
			scrapeResult, more := <-metricCh
			if more {
				e.scrapeResults = append(e.scrapeResults, scrapeResult)
				continue
			}
			return
		}
	}()
	e.scrape(metricCh, tick)

	// report metadata metrics
	metricCh <- e.duration
	metricCh <- e.totalScrapes
	metricCh <- e.error
	e.scrapeErrors.Collect(metricCh)
	metricCh <- e.up
	close(metricCh)
	wg.Wait()
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric, tick *time.Time) {
	e.totalScrapes.Inc()
	errChan := make(chan error, len(e.metricsToScrape.Metric))
	begun := time.Now()

	if connectionError := e.db.Ping(); connectionError != nil {
		e.logger.Debug("connection error", "error", connectionError)
		if strings.Contains(connectionError.Error(), "sql: database is closed") {
			e.logger.Info("Reconnecting to DB")
			connectionError = e.connect()
			if connectionError != nil {
				e.logger.Error("Error reconnecting to DB", "error", connectionError)
			}
		}
	}

	if pingError := e.db.Ping(); pingError != nil {
		e.logger.Error("Error pinging oracle",
			"error", pingError)
		e.up.Set(0)
		e.error.Set(1)
		e.duration.Set(time.Since(begun).Seconds())
		return
	}

	e.dbtypeGauge.Set(float64(e.dbtype))

	e.logger.Debug("Successfully pinged Oracle database: " + maskDsn(e.connectString))
	e.up.Set(1)

	if e.checkIfMetricsChanged() {
		e.reloadMetrics()
	}

	for _, metric := range e.metricsToScrape.Metric {
		metric := metric //https://golang.org/doc/faq#closures_and_goroutines

		go func() {
			e.logger.Debug("About to scrape metric",
				"Context", metric.Context,
				"MetricsDesc", fmt.Sprint(metric.MetricsDesc),
				"MetricsType", fmt.Sprint(metric.MetricsType),
				"MetricsBuckets", fmt.Sprint(metric.MetricsBuckets), // ignored unless histogram
				"Labels", fmt.Sprint(metric.Labels),
				"FieldToAppend", metric.FieldToAppend,
				"IgnoreZeroResult", metric.IgnoreZeroResult,
				"Request", metric.Request)

			if len(metric.Request) == 0 {
				errChan <- errors.New("scrape request not found")
				e.logger.Error("Error scraping for " + fmt.Sprint(metric.MetricsDesc) + ". Did you forget to define request in your toml file?")
				return
			}

			if len(metric.MetricsDesc) == 0 {
				errChan <- errors.New("metricsdesc not found")
				e.logger.Error("Error scraping for query" + fmt.Sprint(metric.Request) + ". Did you forget to define metricsdesc in your toml file?")
				return
			}

			for column, metricType := range metric.MetricsType {
				if metricType == "histogram" {
					_, ok := metric.MetricsBuckets[column]
					if !ok {
						errChan <- errors.New("metricsbuckets not found")
						e.logger.Error("Unable to find MetricsBuckets configuration key for metric. (metric=" + column + ")")
						return
					}
				}
			}

			scrapeStart := time.Now()
			scrapeError := e.ScrapeMetric(e.db, ch, metric, tick)
			// Always send the scrapeError, nil or non-nil
			errChan <- scrapeError
			if scrapeError != nil {
				if shouldLogScrapeError(scrapeError, metric.IgnoreZeroResult) {
					e.logger.Error("Error scraping metric",
						"Context", metric.Context,
						"MetricsDesc", fmt.Sprint(metric.MetricsDesc),
						"duration", time.Since(scrapeStart),
						"error", scrapeError)
				}
				e.scrapeErrors.WithLabelValues(metric.Context).Inc()
			} else {
				e.logger.Debug("Successfully scraped metric",
					"Context", metric.Context,
					"MetricDesc", fmt.Sprint(metric.MetricsDesc),
					"duration", time.Since(scrapeStart))
			}
		}()
	}

	e.afterScrape(begun, len(e.metricsToScrape.Metric), errChan)
}

func (e *Exporter) afterScrape(begun time.Time, countMetrics int, errChan chan error) {
	// Receive all scrape errors
	totalErrors := 0.0
	for i := 0; i < countMetrics; i++ {
		scrapeError := <-errChan
		if scrapeError != nil {
			totalErrors++
		}
	}
	close(errChan)

	e.duration.Set(time.Since(begun).Seconds())
	e.error.Set(totalErrors)
}

func (e *Exporter) connect() error {
	e.logger.Debug("Launching connection to " + maskDsn(e.connectString))

	var P godror.ConnectionParams
	// If password is not specified, externalAuth will be true and we'll ignore user input
	e.externalAuth = e.password == ""
	e.logger.Debug(fmt.Sprintf("external authentication set to %t", e.externalAuth))
	msg := "Using Username/Password Authentication."
	if e.externalAuth {
		msg = "Database Password not specified; will attempt to use external authentication (ignoring user input)."
		e.user = ""
	}
	e.logger.Info(msg)
	externalAuth := sql.NullBool{
		Bool:  e.externalAuth,
		Valid: true,
	}
	P.Username, P.Password, P.ConnectString, P.ExternalAuth = e.user, godror.NewPassword(e.password), e.connectString, externalAuth

	if e.config.PoolIncrement > 0 {
		e.logger.Debug(fmt.Sprintf("set pool increment to %d", e.config.PoolIncrement))
		P.PoolParams.SessionIncrement = e.config.PoolIncrement
	}
	if e.config.PoolMaxConnections > 0 {
		e.logger.Debug(fmt.Sprintf("set pool max connections to %d", e.config.PoolMaxConnections))
		P.PoolParams.MaxSessions = e.config.PoolMaxConnections
	}
	if e.config.PoolMinConnections > 0 {
		e.logger.Debug(fmt.Sprintf("set pool min connections to %d", e.config.PoolMinConnections))
		P.PoolParams.MinSessions = e.config.PoolMinConnections
	}

	P.PoolParams.WaitTimeout = time.Second * 5

	// if TNS_ADMIN env var is set, set ConfigDir to that location
	P.ConfigDir = e.configDir

	switch e.config.DbRole {
	case "SYSDBA":
		P.AdminRole = dsn.SysDBA
	case "SYSOPER":
		P.AdminRole = dsn.SysOPER
	case "SYSBACKUP":
		P.AdminRole = dsn.SysBACKUP
	case "SYSDG":
		P.AdminRole = dsn.SysDG
	case "SYSKM":
		P.AdminRole = dsn.SysKM
	case "SYSRAC":
		P.AdminRole = dsn.SysRAC
	case "SYSASM":
		P.AdminRole = dsn.SysASM
	default:
		P.AdminRole = dsn.NoRole
	}

	// note that this just configures the connection, it does not actually connect until later
	// when we call db.Ping()
	db := sql.OpenDB(godror.NewConnector(P))
	e.logger.Debug(fmt.Sprintf("set max idle connections to %d", e.config.MaxIdleConns))
	db.SetMaxIdleConns(e.config.MaxIdleConns)
	e.logger.Debug(fmt.Sprintf("set max open connections to %d", e.config.MaxOpenConns))
	db.SetMaxOpenConns(e.config.MaxOpenConns)
	db.SetConnMaxLifetime(0)
	e.logger.Debug(fmt.Sprintf("Successfully configured connection to %d", maskDsn(e.connectString)))
	e.db = db

	if _, err := db.Exec(`
			begin
	       		dbms_application_info.set_client_info('oracledb_exporter');
			end;`); err != nil {
		e.logger.Info("Could not set CLIENT_INFO.")
	}

	var result int
	if err := db.QueryRow("select sys_context('USERENV', 'CON_ID') from dual").Scan(&result); err != nil {
		e.logger.Info("dbtype err", "error", err)
	}
	e.dbtype = result

	var sysdba string
	if err := db.QueryRow("select sys_context('USERENV', 'ISDBA') from dual").Scan(&sysdba); err != nil {
		e.logger.Error("error checking my database role", "error", err)
	}
	e.logger.Info("Connected as SYSDBA? " + sysdba)

	return nil
}

// this is used by the log exporter to share the database connection
func (e *Exporter) GetDB() *sql.DB {
	return e.db
}

func (e *Exporter) checkIfMetricsChanged() bool {
	for i, _customMetrics := range strings.Split(e.config.CustomMetrics, ",") {
		if len(_customMetrics) == 0 {
			continue
		}
		e.logger.Debug("Checking modifications in following metrics definition file:" + _customMetrics)
		h := sha256.New()
		if err := hashFile(h, _customMetrics); err != nil {
			e.logger.Error("Unable to get file hash", "error", err)
			return false
		}
		// If any of files has been changed reload metrics
		if !bytes.Equal(hashMap[i], h.Sum(nil)) {
			e.logger.Info(_customMetrics + " has been changed. Reloading metrics...")
			hashMap[i] = h.Sum(nil)
			return true
		}
	}
	return false
}

func hashFile(h hash.Hash, fn string) error {
	f, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	return nil
}

func (e *Exporter) reloadMetrics() {
	// Truncate metricsToScrape
	e.metricsToScrape.Metric = []Metric{}

	// Load default metrics
	defaultMetrics := e.DefaultMetrics()
	e.metricsToScrape.Metric = defaultMetrics.Metric

	// If custom metrics, load it
	if strings.Compare(e.config.CustomMetrics, "") != 0 {
		for _, _customMetrics := range strings.Split(e.config.CustomMetrics, ",") {
			metrics := &Metrics{}
			if _, err := toml.DecodeFile(_customMetrics, metrics); err != nil {
				e.logger.Error("failed to load custom metrics", "error", err)
				panic(errors.New("Error while loading " + _customMetrics))
			} else {
				e.logger.Info("Successfully loaded custom metrics from " + _customMetrics)
			}
			e.metricsToScrape.Metric = append(e.metricsToScrape.Metric, metrics.Metric...)
		}
	} else {
		e.logger.Debug("No custom metrics defined.")
	}
}

// ScrapeMetric is an interface method to call scrapeGenericValues using Metric struct values
func (e *Exporter) ScrapeMetric(db *sql.DB, ch chan<- prometheus.Metric, m Metric, tick *time.Time) error {
	e.logger.Debug("Calling function ScrapeGenericValues()")
	if e.isScrapeMetric(tick, m) {
		queryTimeout := e.getQueryTimeout(m)
		return e.scrapeGenericValues(db, ch, m.Context, m.Labels, m.MetricsDesc,
			m.MetricsType, m.MetricsBuckets, m.FieldToAppend, m.IgnoreZeroResult,
			m.Request, queryTimeout)
	}
	return nil
}

// generic method for retrieving metrics.
func (e *Exporter) scrapeGenericValues(db *sql.DB, ch chan<- prometheus.Metric, context string, labels []string,
	metricsDesc map[string]string, metricsType map[string]string, metricsBuckets map[string]map[string]string,
	fieldToAppend string, ignoreZeroResult bool, request string, queryTimeout time.Duration) error {
	metricsCount := 0
	genericParser := func(row map[string]string) error {
		// Construct labels value
		labelsValues := []string{}
		for _, label := range labels {
			labelsValues = append(labelsValues, row[label])
		}
		// Construct Prometheus values to sent back
		for metric, metricHelp := range metricsDesc {
			value, ok := e.parseFloat(metric, metricHelp, row)
			if !ok {
				// Skip invalid metric values
				continue
			}
			e.logger.Debug("Query result",
				"value", value)
			// If metric do not use a field content in metric's name
			if strings.Compare(fieldToAppend, "") == 0 {
				desc := prometheus.NewDesc(
					prometheus.BuildFQName(namespace, context, metric),
					metricHelp,
					labels, nil,
				)
				if metricsType[strings.ToLower(metric)] == "histogram" {
					count, err := strconv.ParseUint(strings.TrimSpace(row["count"]), 10, 64)
					if err != nil {
						e.logger.Error("Unable to convert count value to int (metric=" + metric +
							",metricHelp=" + metricHelp + ",value=<" + row["count"] + ">)")
						continue
					}
					buckets := make(map[float64]uint64)
					for field, le := range metricsBuckets[metric] {
						lelimit, err := strconv.ParseFloat(strings.TrimSpace(le), 64)
						if err != nil {
							e.logger.Error("Unable to convert bucket limit value to float (metric=" + metric +
								",metricHelp=" + metricHelp + ",bucketlimit=<" + le + ">)")
							continue
						}
						counter, err := strconv.ParseUint(strings.TrimSpace(row[field]), 10, 64)
						if err != nil {
							e.logger.Error("Unable to convert ", field, " value to int (metric="+metric+
								",metricHelp="+metricHelp+",value=<"+row[field]+">)")
							continue
						}
						buckets[lelimit] = counter
					}
					ch <- prometheus.MustNewConstHistogram(desc, count, value, buckets, labelsValues...)
				} else {
					ch <- prometheus.MustNewConstMetric(desc, getMetricType(metric, metricsType), value, labelsValues...)
				}
				// If no labels, use metric name
			} else {
				desc := prometheus.NewDesc(
					prometheus.BuildFQName(namespace, context, cleanName(row[fieldToAppend])),
					metricHelp,
					nil, nil,
				)
				if metricsType[strings.ToLower(metric)] == "histogram" {
					count, err := strconv.ParseUint(strings.TrimSpace(row["count"]), 10, 64)
					if err != nil {
						e.logger.Error("Unable to convert count value to int (metric=" + metric +
							",metricHelp=" + metricHelp + ",value=<" + row["count"] + ">)")
						continue
					}
					buckets := make(map[float64]uint64)
					for field, le := range metricsBuckets[metric] {
						lelimit, err := strconv.ParseFloat(strings.TrimSpace(le), 64)
						if err != nil {
							e.logger.Error("Unable to convert bucket limit value to float (metric=" + metric +
								",metricHelp=" + metricHelp + ",bucketlimit=<" + le + ">)")
							continue
						}
						counter, err := strconv.ParseUint(strings.TrimSpace(row[field]), 10, 64)
						if err != nil {
							e.logger.Error("Unable to convert ", field, " value to int (metric="+metric+
								",metricHelp="+metricHelp+",value=<"+row[field]+">)")
							continue
						}
						buckets[lelimit] = counter
					}
					ch <- prometheus.MustNewConstHistogram(desc, count, value, buckets)
				} else {
					ch <- prometheus.MustNewConstMetric(desc, getMetricType(metric, metricsType), value)
				}
			}
			metricsCount++
		}
		return nil
	}
	e.logger.Debug("Calling function GeneratePrometheusMetrics()")
	err := e.generatePrometheusMetrics(db, genericParser, request, queryTimeout)
	e.logger.Debug("ScrapeGenericValues() - metricsCount: " + strconv.Itoa(metricsCount))
	if err != nil {
		return err
	}
	if !ignoreZeroResult && metricsCount == 0 {
		// a zero result error is returned for caller error identification.
		// https://github.com/oracle/oracle-db-appdev-monitoring/issues/168
		return newZeroResultError()
	}
	return err
}

// inspired by https://kylewbanks.com/blog/query-result-to-map-in-golang
// Parse SQL result and call parsing function to each row
func (e *Exporter) generatePrometheusMetrics(db *sql.DB, parse func(row map[string]string) error, query string, queryTimeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)

	if ctx.Err() == context.DeadlineExceeded {
		return errors.New("Oracle query timed out")
	}

	if err != nil {
		return err
	}
	cols, err := rows.Columns()
	defer rows.Close()

	for rows.Next() {
		// Create a slice of interface{}'s to represent each column,
		// and a second slice to contain pointers to each item in the columns slice.
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		// Scan the result into the column pointers...
		if err := rows.Scan(columnPointers...); err != nil {
			return err
		}

		// Create our map, and retrieve the value for each column from the pointers slice,
		// storing it in the map with the name of the column as the key.
		m := make(map[string]string)
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			m[strings.ToLower(colName)] = fmt.Sprintf("%v", *val)
		}
		// Call function to parse row
		if err := parse(m); err != nil {
			return err
		}
	}
	return nil
}

func getMetricType(metricType string, metricsType map[string]string) prometheus.ValueType {
	var strToPromType = map[string]prometheus.ValueType{
		"gauge":     prometheus.GaugeValue,
		"counter":   prometheus.CounterValue,
		"histogram": prometheus.UntypedValue,
	}

	strType, ok := metricsType[strings.ToLower(metricType)]
	if !ok {
		return prometheus.GaugeValue
	}
	valueType, ok := strToPromType[strings.ToLower(strType)]
	if !ok {
		panic(errors.New("Error while getting prometheus type " + strings.ToLower(strType)))
	}
	return valueType
}

func cleanName(s string) string {
	s = strings.Replace(s, " ", "_", -1) // Remove spaces
	s = strings.Replace(s, "(", "", -1)  // Remove open parenthesis
	s = strings.Replace(s, ")", "", -1)  // Remove close parenthesis
	s = strings.Replace(s, "/", "", -1)  // Remove forward slashes
	s = strings.Replace(s, "*", "", -1)  // Remove asterisks
	s = strings.ToLower(s)
	return s
}
