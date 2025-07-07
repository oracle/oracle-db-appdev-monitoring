// Copyright (c) 2021, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package collector

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	_ "github.com/godror/godror"
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
func NewExporter(logger *slog.Logger, m *MetricsConfiguration) *Exporter {
	var databases []*Database
	wg := &sync.WaitGroup{}
	for dbname, dbconfig := range m.Databases {
		logger.Info("Initializing database", "database", dbname)
		database := NewDatabase(logger, dbname, dbconfig)
		databases = append(databases, database)
		wg.Add(1)
		go func() {
			defer wg.Done()
			database.WarmupConnectionPool(logger)
		}()
	}
	wg.Wait()
	e := &Exporter{
		mu: &sync.Mutex{},
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
		logger:               logger,
		MetricsConfiguration: m,
		databases:            databases,
		lastScraped:          map[string]*time.Time{},
	}
	e.metricsToScrape = e.DefaultMetrics()

	return e
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
	if e.ScrapeInterval() != 0 {
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
	for _, db := range e.databases {
		ch <- db.DBTypeMetric()
		ch <- db.UpMetric()
	}
}

// RunScheduledScrapes is only relevant for users of this package that want to set the scrape on a timer
// rather than letting it be per Collect call
func (e *Exporter) RunScheduledScrapes(ctx context.Context) {
	e.doScrape(time.Now())

	ticker := time.NewTicker(e.ScrapeInterval())
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
	for _, db := range e.databases {
		metricCh <- db.UpMetric()
	}
	close(metricCh)
	wg.Wait()
}

func (e *Exporter) scrapeDatabase(ch chan<- prometheus.Metric, errChan chan<- error, d *Database, tick *time.Time) int {
	// If ping fails, we will try again on the next iteration of metrics scraping
	if err := d.ping(e.logger); err != nil {
		e.logger.Error("Error pinging database", "error", err, "database", d.Name)
		errChan <- err
		return 1
	}
	e.logger.Debug("Successfully pinged Oracle database: "+maskDsn(d.Config.URL), "database", d.Name)

	metricsToScrape := 0
	for _, metric := range e.metricsToScrape.Metric {
		metric := metric //https://golang.org/doc/faq#closures_and_goroutines
		if !e.isScrapeMetric(tick, metric, d) {
			continue
		}
		metricsToScrape++
		go func() {
			e.logger.Debug("About to scrape metric",
				"Context", metric.Context,
				"MetricsDesc", fmt.Sprint(metric.MetricsDesc),
				"MetricsType", fmt.Sprint(metric.MetricsType),
				"MetricsBuckets", fmt.Sprint(metric.MetricsBuckets), // ignored unless histogram
				"Labels", fmt.Sprint(metric.Labels),
				"FieldToAppend", metric.FieldToAppend,
				"IgnoreZeroResult", metric.IgnoreZeroResult,
				"Request", metric.Request,
				"database", d.Name)

			if len(metric.Request) == 0 {
				errChan <- errors.New("scrape request not found")
				e.logger.Error("Error scraping for "+fmt.Sprint(metric.MetricsDesc)+". Did you forget to define request in your toml file?", "database", d.Name)
				return
			}

			if len(metric.MetricsDesc) == 0 {
				errChan <- errors.New("metricsdesc not found")
				e.logger.Error("Error scraping for query"+fmt.Sprint(metric.Request)+". Did you forget to define metricsdesc in your toml file?", "database", d.Name)
				return
			}

			for column, metricType := range metric.MetricsType {
				if metricType == "histogram" {
					_, ok := metric.MetricsBuckets[column]
					if !ok {
						errChan <- errors.New("metricsbuckets not found")
						e.logger.Error("Unable to find MetricsBuckets configuration key for metric. (metric="+column+")", "database", d.Name)
						return
					}
				}
			}

			scrapeStart := time.Now()
			scrapeError := e.ScrapeMetric(d, ch, metric)
			errChan <- scrapeError
			if scrapeError != nil {
				if shouldLogScrapeError(scrapeError, metric.IgnoreZeroResult) {
					e.logger.Error("Error scraping metric",
						"Context", metric.Context,
						"MetricsDesc", fmt.Sprint(metric.MetricsDesc),
						"duration", time.Since(scrapeStart),
						"error", scrapeError,
						"database", d.Name)
				}
				e.scrapeErrors.WithLabelValues(metric.Context).Inc()
			} else {
				e.logger.Debug("Successfully scraped metric",
					"Context", metric.Context,
					"MetricDesc", fmt.Sprint(metric.MetricsDesc),
					"duration", time.Since(scrapeStart),
					"database", d.Name)
			}
		}()
	}
	return metricsToScrape
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric, tick *time.Time) {
	e.totalScrapes.Inc()
	errChan := make(chan error, len(e.metricsToScrape.Metric)*len(e.databases))
	begun := time.Now()
	if e.checkIfMetricsChanged() {
		e.reloadMetrics()
	}

	// Scrape all databases
	asyncTasksCh := make(chan int)
	for _, db := range e.databases {
		db := db
		go func() {
			asyncTasksCh <- e.scrapeDatabase(ch, errChan, db, tick)
		}()
	}
	totalTasks := 0
	for _ = range e.databases {
		totalTasks += <-asyncTasksCh
	}

	e.afterScrape(begun, totalTasks, errChan)
}

func (e *Exporter) afterScrape(begun time.Time, tasks int, errChan chan error) {
	// Receive all scrape errors
	totalErrors := 0.0
	for i := 0; i < tasks; i++ {
		scrapeError := <-errChan
		if scrapeError != nil {
			totalErrors++
		}
	}
	e.duration.Set(time.Since(begun).Seconds())
	e.error.Set(float64(totalErrors))
}

// this is used by the log exporter to share the database connection
func (e *Exporter) GetDBs() []*Database {
	return e.databases
}

func (e *Exporter) checkIfMetricsChanged() bool {
	for i, _customMetrics := range e.CustomMetricsFiles() {
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
	if len(e.CustomMetricsFiles()) > 0 {
		for _, _customMetrics := range e.CustomMetricsFiles() {
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
func (e *Exporter) ScrapeMetric(d *Database, ch chan<- prometheus.Metric, m Metric) error {
	e.logger.Debug("Calling function ScrapeGenericValues()")
	queryTimeout := e.getQueryTimeout(m, d)
	return e.scrapeGenericValues(d, ch, m.Context, m.Labels, m.MetricsDesc,
		m.MetricsType, m.MetricsBuckets, m.FieldToAppend, m.IgnoreZeroResult,
		m.Request, queryTimeout)
}

// generic method for retrieving metrics.
func (e *Exporter) scrapeGenericValues(d *Database, ch chan<- prometheus.Metric, context string, labels []string,
	metricsDesc map[string]string, metricsType map[string]string, metricsBuckets map[string]map[string]string,
	fieldToAppend string, ignoreZeroResult bool, request string, queryTimeout time.Duration) error {
	metricsCount := 0
	constLabels := d.constLabels()
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
					labels,
					constLabels,
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
					nil, constLabels,
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
	err := e.generatePrometheusMetrics(d, genericParser, request, queryTimeout)
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
func (e *Exporter) generatePrometheusMetrics(d *Database, parse func(row map[string]string) error, query string, queryTimeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	rows, err := d.Session.QueryContext(ctx, query)

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
