// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/collector"
	dto "github.com/prometheus/client_model/go"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const instrumentationScope = "github.com/oracle/oracle-db-appdev-monitoring"

// Publisher sends completed Prometheus metric snapshots over OTLP/gRPC.
type Publisher struct {
	client   collectormetricspb.MetricsServiceClient
	conn     *grpc.ClientConn
	logger   *slog.Logger
	timeout  time.Duration
	headers  map[string]string
	resource *resourcepb.Resource
	queue    chan []*dto.MetricFamily
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a publisher and starts its bounded asynchronous export worker.
func New(logger *slog.Logger, cfg *collector.OTLPConfig, version string) (*Publisher, error) {
	if cfg == nil {
		return nil, fmt.Errorf("OTLP configuration is required")
	}
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = *cfg.Timeout
	}
	var transportCredentials credentials.TransportCredentials
	if cfg.Insecure {
		transportCredentials = insecure.NewCredentials()
	} else {
		transportCredentials = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	conn, err := grpc.NewClient(cfg.Endpoint, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return nil, fmt.Errorf("create OTLP gRPC client: %w", err)
	}

	resourceAttributes := copyAttributes(cfg.ResourceAttributes)
	if _, ok := resourceAttributes["service.name"]; !ok {
		resourceAttributes["service.name"] = "oracledb_exporter"
	}
	if _, ok := resourceAttributes["service.version"]; !ok {
		resourceAttributes["service.version"] = version
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Publisher{
		client:   collectormetricspb.NewMetricsServiceClient(conn),
		conn:     conn,
		logger:   logger,
		timeout:  timeout,
		headers:  headers(cfg.Headers),
		resource: &resourcepb.Resource{Attributes: keyValues(resourceAttributes)},
		queue:    make(chan []*dto.MetricFamily, 1),
		cancel:   cancel,
	}
	p.wg.Add(1)
	go p.run(ctx)
	return p, nil
}

// Publish queues a completed metric snapshot. It never blocks scraping.
func (p *Publisher) Publish(families []*dto.MetricFamily) {
	select {
	case p.queue <- families:
	default:
		p.logger.Warn("dropping completed OTLP metric snapshot because an export is still in progress")
	}
}

// Close stops the worker and closes the gRPC connection.
func (p *Publisher) Close() error {
	p.cancel()
	p.wg.Wait()
	return p.conn.Close()
}

func (p *Publisher) run(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case families := <-p.queue:
			p.export(families)
		}
	}
}

func (p *Publisher) export(families []*dto.MetricFamily) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	if len(p.headers) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, metadata.New(p.headers))
	}
	request := &collectormetricspb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: p.resource,
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Scope:   &commonpb.InstrumentationScope{Name: instrumentationScope},
			Metrics: convertFamilies(families),
		}},
	}}}
	if _, err := p.client.Export(ctx, request); err != nil {
		p.logger.Error("unable to export metrics over OTLP", "error", err)
	}
}

func copyAttributes(attributes map[string]string) map[string]string {
	copy := make(map[string]string, len(attributes))
	for key, value := range attributes {
		copy[key] = value
	}
	return copy
}

func headers(configured map[string]string) map[string]string {
	values := make(map[string]string, len(configured))
	for key, value := range configured {
		values[strings.ToLower(key)] = value
	}
	return values
}

func keyValues(attributes map[string]string) []*commonpb.KeyValue {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]*commonpb.KeyValue, 0, len(keys))
	for _, key := range keys {
		values = append(values, &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: attributes[key]}}})
	}
	return values
}

func convertFamilies(families []*dto.MetricFamily) []*metricspb.Metric {
	metrics := make([]*metricspb.Metric, 0, len(families))
	for _, family := range families {
		if metric := convertFamily(family); metric != nil {
			metrics = append(metrics, metric)
		}
	}
	return metrics
}

func convertFamily(family *dto.MetricFamily) *metricspb.Metric {
	if family == nil || family.Name == nil {
		return nil
	}
	metric := &metricspb.Metric{Name: family.GetName(), Description: family.GetHelp()}
	switch family.GetType() {
	case dto.MetricType_GAUGE, dto.MetricType_UNTYPED:
		points := make([]*metricspb.NumberDataPoint, 0, len(family.Metric))
		for _, sample := range family.Metric {
			value := sample.GetGauge().GetValue()
			if family.GetType() == dto.MetricType_UNTYPED {
				value = sample.GetUntyped().GetValue()
			}
			points = append(points, numberPoint(sample, value))
		}
		metric.Data = &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: points}}
	case dto.MetricType_COUNTER:
		points := make([]*metricspb.NumberDataPoint, 0, len(family.Metric))
		for _, sample := range family.Metric {
			points = append(points, numberPoint(sample, sample.GetCounter().GetValue()))
		}
		metric.Data = &metricspb.Metric_Sum{Sum: &metricspb.Sum{DataPoints: points, AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, IsMonotonic: true}}
	case dto.MetricType_HISTOGRAM:
		points := make([]*metricspb.HistogramDataPoint, 0, len(family.Metric))
		for _, sample := range family.Metric {
			points = append(points, histogramPoint(sample))
		}
		metric.Data = &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{DataPoints: points, AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE}}
	default:
		return nil
	}
	return metric
}

func numberPoint(sample *dto.Metric, value float64) *metricspb.NumberDataPoint {
	return &metricspb.NumberDataPoint{Attributes: labels(sample), TimeUnixNano: sampleTimestamp(sample), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}
}

func histogramPoint(sample *dto.Metric) *metricspb.HistogramDataPoint {
	histogram := sample.GetHistogram()
	sum := histogram.GetSampleSum()
	point := &metricspb.HistogramDataPoint{Attributes: labels(sample), TimeUnixNano: sampleTimestamp(sample), Count: histogram.GetSampleCount(), Sum: &sum}
	var previous uint64
	for _, bucket := range histogram.Bucket {
		if math.IsInf(bucket.GetUpperBound(), 1) {
			continue
		}
		point.ExplicitBounds = append(point.ExplicitBounds, bucket.GetUpperBound())
		point.BucketCounts = append(point.BucketCounts, bucket.GetCumulativeCount()-previous)
		previous = bucket.GetCumulativeCount()
	}
	point.BucketCounts = append(point.BucketCounts, point.Count-previous)
	return point
}

func labels(sample *dto.Metric) []*commonpb.KeyValue {
	attributes := make(map[string]string, len(sample.Label))
	for _, pair := range sample.Label {
		attributes[pair.GetName()] = pair.GetValue()
	}
	return keyValues(attributes)
}

func sampleTimestamp(sample *dto.Metric) uint64 {
	if sample.TimestampMs != nil {
		return uint64(sample.GetTimestampMs()) * uint64(time.Millisecond)
	}
	return uint64(time.Now().UnixNano())
}
