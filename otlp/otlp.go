// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"context"
	"fmt"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/collector"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

const instrumentationScope = "github.com/oracle/oracle-db-appdev-monitoring"

// Producer translates collector snapshots into OpenTelemetry SDK metric data.
type Producer struct {
	gatherer prometheus.Gatherer
}

// Implements metric.Producer
var producer metric.Producer = &Producer{}

// Produce gathers the current collector snapshot for the SDK reader.
func (p Producer) Produce(context.Context) ([]metricdata.ScopeMetrics, error) {
	families, err := p.gatherer.Gather()
	if err != nil {
		return nil, fmt.Errorf("gather collector metrics: %w", err)
	}
	return []metricdata.ScopeMetrics{{
		Scope:   instrumentation.Scope{Name: instrumentationScope},
		Metrics: convertFamilies(families),
	}}, nil
}

// Pipeline wires the collector producer to the OpenTelemetry SDK push pipeline.
type Pipeline struct {
	provider *metric.MeterProvider
}

// New creates an SDK metric pipeline with the official OTLP/gRPC exporter.
func New(ctx context.Context, cfg *collector.OTLPConfig, version string, interval time.Duration, gatherer prometheus.Gatherer) (*Pipeline, error) {
	if cfg == nil {
		return nil, fmt.Errorf("OTLP configuration is required")
	}
	if gatherer == nil {
		return nil, fmt.Errorf("Prometheus gatherer is required")
	}
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = *cfg.Timeout
	}
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		otlpmetricgrpc.WithTimeout(timeout),
		otlpmetricgrpc.WithHeaders(copyAttributes(cfg.Headers)),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	resourceAttributes := copyAttributes(cfg.ResourceAttributes)
	if _, ok := resourceAttributes["service.name"]; !ok {
		resourceAttributes["service.name"] = "oracledb_exporter"
	}
	if _, ok := resourceAttributes["service.version"]; !ok {
		resourceAttributes["service.version"] = version
	}

	// Periodically reads metrics from the OTLP exporter
	reader := metric.NewPeriodicReader(exporter,
		metric.WithInterval(interval),
		metric.WithTimeout(timeout),
		metric.WithProducer(Producer{gatherer: gatherer}),
	)
	return &Pipeline{provider: metric.NewMeterProvider(
		metric.WithResource(resource.NewWithAttributes("", attributes(resourceAttributes)...)),
		metric.WithReader(reader),
	)}, nil
}

// Shutdown flushes the SDK reader and releases the exporter connection.
func (p *Pipeline) Shutdown(ctx context.Context) error {
	return p.provider.Shutdown(ctx)
}

// ForceFlush collects and exports a snapshot immediately.
func (p *Pipeline) ForceFlush(ctx context.Context) error {
	return p.provider.ForceFlush(ctx)
}

func copyAttributes(attributes map[string]string) map[string]string {
	copy := make(map[string]string, len(attributes))
	for key, value := range attributes {
		copy[key] = value
	}
	return copy
}
