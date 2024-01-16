// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/godror/godror"
	"github.com/prometheus/client_golang/prometheus"
)

// Exporter collects Oracle DB metrics. It implements prometheus.Collector.
type Exporter struct {
	config          *Config
	mu              *sync.Mutex
	metricsToScrape Metrics
	scrapeInterval  *time.Duration
	user            string
	password        string
	connectString   string
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    *prometheus.CounterVec
	scrapeResults   []prometheus.Metric
	up              prometheus.Gauge
	db              *sql.DB
	logger          log.Logger
}

// Config is the configuration of the exporter
type Config struct {
	User               string
	Password           string
	ConnectString      string
	MaxIdleConns       int
	MaxOpenConns       int
	CustomMetrics      string
	QueryTimeout       int
	DefaultMetricsFile string
}

// CreateDefaultConfig returns the default configuration of the Exporter
// it is to be of note that the DNS will be empty when
func CreateDefaultConfig() *Config {
	return &Config{
		MaxIdleConns:       0,
		MaxOpenConns:       10,
		CustomMetrics:      "",
		QueryTimeout:       5,
		DefaultMetricsFile: "",
	}
}

// Metric is an object description
type Metric struct {
	Context          string
	Labels           []string
	MetricsDesc      map[string]string
	MetricsType      map[string]string
	MetricsBuckets   map[string]map[string]string
	FieldToAppend    string
	Request          string
	IgnoreZeroResult bool
}

// Metrics is a container structure for prometheus metrics
type Metrics struct {
	Metric []Metric
}

var (
	additionalMetrics Metrics
	hashMap           = make(map[int][]byte)
	namespace         = "oracledb"
	exporterName      = "exporter"
)

func maskDsn(dsn string) string {
	parts := strings.Split(dsn, "@")
	if len(parts) > 1 {
		maskedURL := "***@" + parts[1]
		return maskedURL
	}
	return dsn
}

// NewExporter creates a new Exporter instance
func NewExporter(logger log.Logger, cfg *Config) (*Exporter, error) {
	e := &Exporter{
		mu:            &sync.Mutex{},
		user:          cfg.User,
		password:      cfg.Password,
		connectString: cfg.ConnectString,
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
	e.scrape(ch)
	ch <- e.duration
	ch <- e.totalScrapes
	ch <- e.error
	e.scrapeErrors.Collect(ch)
	ch <- e.up
}

// RunScheduledScrapes is only relevant for users of this package that want to set the scrape on a timer
// rather than letting it be per Collect call
func (e *Exporter) RunScheduledScrapes(ctx context.Context, si time.Duration) {
	e.scrapeInterval = &si
	ticker := time.NewTicker(si)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.mu.Lock() // ensure no simultaneous scrapes
			e.scheduledScrape()
			e.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (e *Exporter) scheduledScrape() {
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
	e.scrape(metricCh)

	// report metadata metrics
	metricCh <- e.duration
	metricCh <- e.totalScrapes
	metricCh <- e.error
	e.scrapeErrors.Collect(metricCh)
	metricCh <- e.up

	close(metricCh)
	wg.Wait()
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) {
	e.totalScrapes.Inc()
	var err error
	defer func(begun time.Time) {
		e.duration.Set(time.Since(begun).Seconds())
		if err == nil {
			e.error.Set(0)
		} else {
			e.error.Set(1)
		}
	}(time.Now())

	if err = e.db.Ping(); err != nil {
		level.Debug(e.logger).Log("msg", "error = "+err.Error())
		if strings.Contains(err.Error(), "sql: database is closed") {
			level.Info(e.logger).Log("msg", "Reconnecting to DB")
			err = e.connect()
			if err != nil {
				level.Error(e.logger).Log("msg", "Error reconnecting to DB", err)
			}
		}
	}

	if err = e.db.Ping(); err != nil {
		level.Error(e.logger).Log("msg", "Error pinging oracle",
			"error", err)
		e.up.Set(0)
		return
	}

	level.Debug(e.logger).Log("msg", "Successfully pinged Oracle database: "+maskDsn(e.connectString))
	e.up.Set(1)

	if e.checkIfMetricsChanged() {
		e.reloadMetrics()
	}

	wg := sync.WaitGroup{}

	for _, metric := range e.metricsToScrape.Metric {
		wg.Add(1)
		metric := metric //https://golang.org/doc/faq#closures_and_goroutines

		go func() {
			defer wg.Done()

			level.Debug(e.logger).Log("msg", "About to scrape metric",
				"Context", metric.Context,
				"MetricsDesc", fmt.Sprint(metric.MetricsDesc),
				"MetricsType", fmt.Sprint(metric.MetricsType),
				"MetricsBuckets", fmt.Sprint(metric.MetricsBuckets), // ignored unless histogram
				"Labels", fmt.Sprint(metric.Labels),
				"FieldToAppend", metric.FieldToAppend,
				"IgnoreZeroResult", metric.IgnoreZeroResult,
				"Request", metric.Request)

			if len(metric.Request) == 0 {
				level.Error(e.logger).Log("msg", "Error scraping for "+fmt.Sprint(metric.MetricsDesc)+". Did you forget to define request in your toml file?")
				return
			}

			if len(metric.MetricsDesc) == 0 {
				level.Error(e.logger).Log("msg", "Error scraping for query"+fmt.Sprint(metric.Request)+". Did you forget to define metricsdesc  in your toml file?")
				return
			}

			for column, metricType := range metric.MetricsType {
				if metricType == "histogram" {
					_, ok := metric.MetricsBuckets[column]
					if !ok {
						level.Error(e.logger).Log("msg", "Unable to find MetricsBuckets configuration key for metric. (metric="+column+")")
						return
					}
				}
			}

			scrapeStart := time.Now()
			if err = e.ScrapeMetric(e.db, ch, metric); err != nil {
				level.Error(e.logger).Log("msg", "Error scraping metric",
					"Context", metric.Context,
					"MetricsDesc", fmt.Sprint(metric.MetricsDesc),
					"time", time.Since(scrapeStart),
					"error", err)
				e.scrapeErrors.WithLabelValues(metric.Context).Inc()
			} else {
				level.Debug(e.logger).Log("msg", "Successfully scraped metric",
					"Context", metric.Context,
					"MetricDesc", fmt.Sprint(metric.MetricsDesc),
					"time", time.Since(scrapeStart))
			}
		}()
	}
	wg.Wait()
}

func (e *Exporter) connect() error {
	level.Debug(e.logger).Log("msg", "Launching connection to "+maskDsn(e.connectString))

	var P godror.ConnectionParams
	P.Username, P.Password, P.ConnectString = e.user, godror.NewPassword(e.password), e.connectString

	level.Debug(e.logger).Log("msg", "connection properties: "+fmt.Sprint(P))

	db := sql.OpenDB(godror.NewConnector(P))
	// if err != nil {
	// 	level.Error(e.logger).Log("Error while connecting to", e.dsn)
	// 	return err
	// }
	level.Debug(e.logger).Log("msg", "disable go connection pooling, to allow use of oracle's instead")
	db.SetMaxIdleConns(0)
	db.SetMaxOpenConns(0)
	db.SetConnMaxLifetime(0)
	level.Debug(e.logger).Log("msg", "Successfully configured connection to "+maskDsn(e.connectString))
	e.db = db
	return nil
}

func (e *Exporter) checkIfMetricsChanged() bool {
	for i, _customMetrics := range strings.Split(e.config.CustomMetrics, ",") {
		if len(_customMetrics) == 0 {
			continue
		}
		level.Debug(e.logger).Log("msg", "Checking modifications in following metrics definition file:"+_customMetrics)
		h := sha256.New()
		if err := hashFile(h, _customMetrics); err != nil {
			level.Error(e.logger).Log("msg", "Unable to get file hash", "error", err)
			return false
		}
		// If any of files has been changed reload metrics
		if !bytes.Equal(hashMap[i], h.Sum(nil)) {
			level.Info(e.logger).Log("msg", _customMetrics+" has been changed. Reloading metrics...")
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
			if _, err := toml.DecodeFile(_customMetrics, &additionalMetrics); err != nil {
				level.Error(e.logger).Log(err)
				panic(errors.New("Error while loading " + _customMetrics))
			} else {
				level.Info(e.logger).Log("msg", "Successfully loaded custom metrics from "+_customMetrics)
			}
			e.metricsToScrape.Metric = append(e.metricsToScrape.Metric, additionalMetrics.Metric...)
		}
	} else {
		level.Debug(e.logger).Log("msg", "No custom metrics defined.")
	}
}

// ScrapeMetric is an interface method to call scrapeGenericValues using Metric struct values
func (e *Exporter) ScrapeMetric(db *sql.DB, ch chan<- prometheus.Metric, metricDefinition Metric) error {
	level.Debug(e.logger).Log("msg", "Calling function ScrapeGenericValues()")
	return e.scrapeGenericValues(db, ch, metricDefinition.Context, metricDefinition.Labels,
		metricDefinition.MetricsDesc, metricDefinition.MetricsType, metricDefinition.MetricsBuckets,
		metricDefinition.FieldToAppend, metricDefinition.IgnoreZeroResult,
		metricDefinition.Request)
}

// generic method for retrieving metrics.
func (e *Exporter) scrapeGenericValues(db *sql.DB, ch chan<- prometheus.Metric, context string, labels []string,
	metricsDesc map[string]string, metricsType map[string]string, metricsBuckets map[string]map[string]string, fieldToAppend string, ignoreZeroResult bool, request string) error {
	metricsCount := 0
	genericParser := func(row map[string]string) error {
		// Construct labels value
		labelsValues := []string{}
		for _, label := range labels {
			labelsValues = append(labelsValues, row[label])
		}
		// Construct Prometheus values to sent back
		for metric, metricHelp := range metricsDesc {
			value, err := strconv.ParseFloat(strings.TrimSpace(row[metric]), 64)
			// If not a float, skip current metric
			if err != nil {
				level.Error(e.logger).Log("msg", "Unable to convert current value to float (metric="+metric+
					",metricHelp="+metricHelp+",value=<"+row[metric]+">)")
				continue
			}
			level.Debug(e.logger).Log("msg", "Query result",
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
						level.Error(e.logger).Log("msg", "Unable to convert count value to int (metric="+metric+
							",metricHelp="+metricHelp+",value=<"+row["count"]+">)")
						continue
					}
					buckets := make(map[float64]uint64)
					for field, le := range metricsBuckets[metric] {
						lelimit, err := strconv.ParseFloat(strings.TrimSpace(le), 64)
						if err != nil {
							level.Error(e.logger).Log("msg", "Unable to convert bucket limit value to float (metric="+metric+
								",metricHelp="+metricHelp+",bucketlimit=<"+le+">)")
							continue
						}
						counter, err := strconv.ParseUint(strings.TrimSpace(row[field]), 10, 64)
						if err != nil {
							level.Error(e.logger).Log("msg", "Unable to convert ", field, " value to int (metric="+metric+
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
						level.Error(e.logger).Log("msg", "Unable to convert count value to int (metric="+metric+
							",metricHelp="+metricHelp+",value=<"+row["count"]+">)")
						continue
					}
					buckets := make(map[float64]uint64)
					for field, le := range metricsBuckets[metric] {
						lelimit, err := strconv.ParseFloat(strings.TrimSpace(le), 64)
						if err != nil {
							level.Error(e.logger).Log("msg", "Unable to convert bucket limit value to float (metric="+metric+
								",metricHelp="+metricHelp+",bucketlimit=<"+le+">)")
							continue
						}
						counter, err := strconv.ParseUint(strings.TrimSpace(row[field]), 10, 64)
						if err != nil {
							level.Error(e.logger).Log("msg", "Unable to convert ", field, " value to int (metric="+metric+
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
	level.Debug(e.logger).Log("msg", "Calling function GeneratePrometheusMetrics()")
	err := e.generatePrometheusMetrics(db, genericParser, request)
	level.Debug(e.logger).Log("msg", "ScrapeGenericValues() - metricsCount: "+strconv.Itoa(metricsCount))
	if err != nil {
		return err
	}
	if !ignoreZeroResult && metricsCount == 0 {
		return errors.New("no metrics found while parsing, query returned no rows")
	}
	return err
}

// inspired by https://kylewbanks.com/blog/query-result-to-map-in-golang
// Parse SQL result and call parsing function to each row
func (e *Exporter) generatePrometheusMetrics(db *sql.DB, parse func(row map[string]string) error, query string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.QueryTimeout)*time.Second)
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

func (e *Exporter) logError(s string) {
	_ = level.Error(e.logger).Log(s)
}

func (e *Exporter) logDebug(s string) {
	_ = level.Debug(e.logger).Log(s)
}
