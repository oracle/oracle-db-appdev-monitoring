// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/collector"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestTLSCredentialsConfiguresClientTLS(t *testing.T) {
	config, err := clientTLSConfig(&collector.OTLPTLSConfig{ServerName: "collector.example", MinVersion: "TLS1.3"})
	if err != nil {
		t.Fatalf("client TLS config: %v", err)
	}
	if config.ServerName != "collector.example" {
		t.Fatalf("expected configured server name, got %q", config.ServerName)
	}
	if config.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS 1.3 minimum version, got %d", config.MinVersion)
	}
}

type metricsService struct {
	collectormetricspb.UnimplementedMetricsServiceServer
	requests chan *collectormetricspb.ExportMetricsServiceRequest
	headers  chan metadata.MD
}

func (s *metricsService) Export(ctx context.Context, request *collectormetricspb.ExportMetricsServiceRequest) (*collectormetricspb.ExportMetricsServiceResponse, error) {
	requestHeaders, _ := metadata.FromIncomingContext(ctx)
	s.requests <- request
	s.headers <- requestHeaders
	return &collectormetricspb.ExportMetricsServiceResponse{}, nil
}

type staticGatherer struct {
	families []*dto.MetricFamily
}

func (g staticGatherer) Gather() ([]*dto.MetricFamily, error) { return g.families, nil }

var _ prometheus.Gatherer = staticGatherer{}

func TestProducerConvertsGaugeCounterAndHistogram(t *testing.T) {
	nameGauge, nameCounter, nameHistogram := "gauge", "counter", "histogram"
	help := "test metric"
	labelName, labelValue := "database", "db1"
	gaugeValue, counterValue, sum := 2.5, 7.0, 12.0
	count, firstBucket, secondBucket := uint64(5), uint64(2), uint64(5)
	upperOne, upperTwo := 1.0, 2.0
	families := []*dto.MetricFamily{
		{Name: &nameGauge, Help: &help, Type: dto.MetricType_GAUGE.Enum(), Metric: []*dto.Metric{{Label: []*dto.LabelPair{{Name: &labelName, Value: &labelValue}}, Gauge: &dto.Gauge{Value: &gaugeValue}}}},
		{Name: &nameCounter, Help: &help, Type: dto.MetricType_COUNTER.Enum(), Metric: []*dto.Metric{{Counter: &dto.Counter{Value: &counterValue}}}},
		{Name: &nameHistogram, Help: &help, Type: dto.MetricType_HISTOGRAM.Enum(), Metric: []*dto.Metric{{Histogram: &dto.Histogram{SampleCount: &count, SampleSum: &sum, Bucket: []*dto.Bucket{{CumulativeCount: &firstBucket, UpperBound: &upperOne}, {CumulativeCount: &secondBucket, UpperBound: &upperTwo}}}}}},
	}

	scopes, err := (Producer{gatherer: staticGatherer{families: families}}).Produce(context.Background())
	if err != nil {
		t.Fatalf("produce: %v", err)
	}
	metrics := scopes[0].Metrics
	if len(metrics) != 3 {
		t.Fatalf("expected three converted metrics, got %d", len(metrics))
	}
	gauge := metrics[0].Data.(metricdata.Gauge[float64])
	if got := gauge.DataPoints[0].Value; got != gaugeValue {
		t.Fatalf("expected gauge value %v, got %v", gaugeValue, got)
	}
	value, ok := gauge.DataPoints[0].Attributes.Value("database")
	if !ok || value.AsString() != labelValue {
		got := ""
		if ok {
			got = value.AsString()
		}
		t.Fatalf("expected label value %q, got %q", labelValue, got)
	}
	counter := metrics[1].Data.(metricdata.Sum[float64])
	if got := counter.DataPoints[0].Value; got != counterValue {
		t.Fatalf("expected counter value %v, got %v", counterValue, got)
	}
	if !counter.IsMonotonic {
		t.Fatal("expected counter sum to be monotonic")
	}
	histogram := metrics[2].Data.(metricdata.Histogram[float64]).DataPoints[0]
	if got := histogram.BucketCounts; len(got) != 3 || got[0] != 2 || got[1] != 3 || got[2] != 0 {
		t.Fatalf("unexpected histogram bucket counts: %#v", got)
	}
	if got := histogram.Bounds; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("unexpected histogram bounds: %#v", got)
	}
}

func TestPipelineExportsConfiguredHeadersAndResources(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	service := &metricsService{requests: make(chan *collectormetricspb.ExportMetricsServiceRequest, 1), headers: make(chan metadata.MD, 1)}
	collectormetricspb.RegisterMetricsServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })

	timeout := time.Second
	pipeline, err := New(context.Background(), &collector.OTLPConfig{
		Endpoint: "http://" + listener.Addr().String(), Timeout: &timeout,
		Headers:            map[string]string{"Authorization": "Bearer token"},
		ResourceAttributes: map[string]string{"service.name": "custom-exporter", "deployment.environment": "test"},
	}, "1.2.3", time.Hour, staticGatherer{})
	if err != nil {
		t.Fatalf("new pipeline: %v", err)
	}
	t.Cleanup(func() { _ = pipeline.Shutdown(context.Background()) })
	if err := pipeline.ForceFlush(context.Background()); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	select {
	case headers := <-service.headers:
		if got := headers.Get("authorization"); len(got) != 1 || got[0] != "Bearer token" {
			t.Fatalf("expected authorization metadata, got %#v", headers)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OTLP request")
	}
	select {
	case request := <-service.requests:
		attributes := map[string]string{}
		for _, attribute := range request.ResourceMetrics[0].Resource.Attributes {
			attributes[attribute.GetKey()] = attribute.GetValue().GetStringValue()
		}
		if got := attributes["service.name"]; got != "custom-exporter" {
			t.Fatalf("expected configured service name, got %q", got)
		}
		if got := attributes["service.version"]; got != "1.2.3" {
			t.Fatalf("expected default service version, got %q", got)
		}
		if got := attributes["deployment.environment"]; got != "test" {
			t.Fatalf("expected configured deployment environment, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OTLP request")
	}
}
