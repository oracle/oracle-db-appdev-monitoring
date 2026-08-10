// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/collector"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/credentials"
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
		return nil, fmt.Errorf("prometheus gatherer is required")
	}
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = *cfg.Timeout
	}
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithTimeout(timeout),
		otlpmetricgrpc.WithHeaders(copyAttributes(cfg.Headers)),
	}
	endpointURL, err := url.ParseRequestURI(cfg.Endpoint)
	if err != nil || endpointURL.Host == "" || (endpointURL.Scheme != "http" && endpointURL.Scheme != "https") {
		return nil, fmt.Errorf("OTLP endpoint must be an http:// or https:// URL")
	}
	opts = append(opts, otlpmetricgrpc.WithEndpointURL(cfg.Endpoint))
	if cfg.TLS != nil {
		tlsConfig, err := clientTLSConfig(cfg.TLS)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
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

func clientTLSConfig(cfg *collector.OTLPTLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
	if cfg.MinVersion == "TLS1.3" {
		tlsConfig.MinVersion = tls.VersionTLS13
	}
	if cfg.CAFile != "" {
		certificate, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read OTLP CA file: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(certificate) {
			return nil, fmt.Errorf("parse OTLP CA file %q", cfg.CAFile)
		}
		tlsConfig.RootCAs = roots
	}
	if cfg.CertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load OTLP client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfig, nil
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
