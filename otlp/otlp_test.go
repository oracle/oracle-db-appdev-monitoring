// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/collector"
	dto "github.com/prometheus/client_model/go"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

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

func TestConvertFamiliesConvertsGaugeCounterAndHistogram(t *testing.T) {
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

	metrics := convertFamilies(families)
	if len(metrics) != 3 {
		t.Fatalf("expected three converted metrics, got %d", len(metrics))
	}
	if got := metrics[0].GetGauge().DataPoints[0].GetAsDouble(); got != gaugeValue {
		t.Fatalf("expected gauge value %v, got %v", gaugeValue, got)
	}
	if got := metrics[0].GetGauge().DataPoints[0].Attributes[0].GetValue().GetStringValue(); got != labelValue {
		t.Fatalf("expected label value %q, got %q", labelValue, got)
	}
	if got := metrics[1].GetSum().DataPoints[0].GetAsDouble(); got != counterValue {
		t.Fatalf("expected counter value %v, got %v", counterValue, got)
	}
	if !metrics[1].GetSum().GetIsMonotonic() {
		t.Fatal("expected counter sum to be monotonic")
	}
	histogram := metrics[2].GetHistogram().DataPoints[0]
	if got := histogram.GetBucketCounts(); len(got) != 3 || got[0] != 2 || got[1] != 3 || got[2] != 0 {
		t.Fatalf("unexpected histogram bucket counts: %#v", got)
	}
	if got := histogram.GetExplicitBounds(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("unexpected histogram bounds: %#v", got)
	}
}

func TestNewAppliesDefaultAndConfiguredResourceAttributes(t *testing.T) {
	timeout := time.Second
	publisher, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), &collector.OTLPConfig{
		Endpoint: "127.0.0.1:4317",
		Insecure: true,
		Timeout:  &timeout,
		ResourceAttributes: map[string]string{
			"service.name":           "custom-exporter",
			"deployment.environment": "test",
		},
	}, "1.2.3")
	if err != nil {
		t.Fatalf("expected publisher to be created, got %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })

	attributes := map[string]string{}
	for _, attribute := range publisher.resource.Attributes {
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
}

func TestPublisherExportsConfiguredHeadersAndResources(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	service := &metricsService{requests: make(chan *collectormetricspb.ExportMetricsServiceRequest, 1), headers: make(chan metadata.MD, 1)}
	collectormetricspb.RegisterMetricsServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	timeout := time.Second
	publisher, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), &collector.OTLPConfig{
		Endpoint: listener.Addr().String(),
		Insecure: true,
		Timeout:  &timeout,
		Headers:  map[string]string{"Authorization": "Bearer token"},
	}, "1.2.3")
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	publisher.export(nil)

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
		attributes := request.ResourceMetrics[0].Resource.Attributes
		if len(attributes) != 2 {
			t.Fatalf("expected default resource attributes, got %#v", attributes)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OTLP request")
	}
}
