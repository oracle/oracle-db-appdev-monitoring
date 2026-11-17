// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/querylog"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// LogTransport owns a concurrency-safe OTLP client shared by log outputs.
// Close it only after all of its outputs have stopped delivering.
type LogTransport struct {
	conn     *grpc.ClientConn
	client   collectorlogspb.LogsServiceClient
	timeout  time.Duration
	headers  map[string]string
	resource *resourcepb.Resource
}

// LogOutput sends acknowledged batches directly through the OTLP protocol.
// There is no volatile SDK queue between delivery and checkpoint persistence.
type LogOutput struct {
	*LogTransport
	timestampColumn string
	ownsTransport   bool
}

func NewLogOutput(cfg *config.OTLPConfig, version, timestampColumn string) (*LogOutput, error) {
	transport, err := NewLogTransport(cfg, version)
	if err != nil {
		return nil, err
	}
	output := transport.Output(timestampColumn)
	output.ownsTransport = true
	return output, nil
}

// Output creates a stream-specific output borrowing this transport.
func (t *LogTransport) Output(timestampColumn string) *LogOutput {
	return &LogOutput{LogTransport: t, timestampColumn: timestampColumn}
}

func NewLogTransport(cfg *config.OTLPConfig, version string) (*LogTransport, error) {
	if cfg == nil {
		return nil, errors.New("OTLP log transport is required")
	}
	u, err := url.ParseRequestURI(cfg.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("OTLP endpoint must be an http:// or https:// URL")
	}
	creds := insecure.NewCredentials()
	if u.Scheme == "https" {
		tlsCfg := cfg.TLS
		if tlsCfg == nil {
			tlsCfg = &config.OTLPTLSConfig{}
		}
		tlsConfig, err := clientTLSConfig(tlsCfg)
		if err != nil {
			return nil, err
		}
		creds = credentials.NewTLS(tlsConfig)
	} else if cfg.TLS != nil {
		return nil, errors.New("OTLP TLS settings require https")
	}
	conn, err := grpc.NewClient(u.Host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	timeout := 10 * time.Second
	if cfg.Timeout != nil {
		timeout = *cfg.Timeout
	}
	attrs := copyAttributes(cfg.ResourceAttributes)
	if _, ok := attrs["service.name"]; !ok {
		attrs["service.name"] = "oracledb_exporter"
	}
	if _, ok := attrs["service.version"]; !ok {
		attrs["service.version"] = version
	}
	resource := &resourcepb.Resource{}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		resource.Attributes = append(resource.Attributes, kv(k, attrs[k]))
	}
	return &LogTransport{conn: conn, client: collectorlogspb.NewLogsServiceClient(conn), timeout: timeout, headers: copyAttributes(cfg.Headers), resource: resource}, nil
}
func kv(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}
func (o *LogOutput) Deliver(ctx context.Context, rows []querylog.Row) error {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.New(o.headers))
	records := make([]*logspb.LogRecord, 0, len(rows))
	observed := uint64(time.Now().UnixNano())
	for _, row := range rows {
		timestamp, err := row.Timestamp(o.timestampColumn)
		if err != nil {
			return err
		}
		if timestamp.Before(time.Unix(0, 0)) {
			return errors.New("OTLP cannot represent event times before the Unix epoch; configure initialLookback")
		}
		body, err := rowBody(row)
		if err != nil {
			return err
		}
		records = append(records, &logspb.LogRecord{
			TimeUnixNano: uint64(timestamp.UnixNano()), ObservedTimeUnixNano: observed,
			Body: body,
		})
	}
	return o.export(ctx, records)
}

// export splits resource-exhausted batches to fit the receiver, retaining the
// original delivery deadline. A single record cannot be subdivided.
func (o *LogOutput) export(ctx context.Context, records []*logspb.LogRecord) error {
	response, err := o.client.Export(ctx, &collectorlogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: o.resource, ScopeLogs: []*logspb.ScopeLogs{{Scope: &commonpb.InstrumentationScope{Name: instrumentationScope}, LogRecords: records}}}}})
	if status.Code(err) == codes.ResourceExhausted && len(records) > 1 {
		middle := len(records) / 2
		if err := o.export(ctx, records[:middle]); err != nil {
			return err
		}
		return o.export(ctx, records[middle:])
	}
	if err != nil {
		return err
	}
	if response.PartialSuccess != nil && response.PartialSuccess.RejectedLogRecords > 0 {
		return fmt.Errorf("%w: receiver rejected %d records; delivery will retry (accepted rows may replay)", querylog.ErrRejected, response.PartialSuccess.RejectedLogRecords)
	}
	return nil
}
func (t *LogTransport) Close() error { return t.conn.Close() }

func (o *LogOutput) Close() error {
	if o.ownsTransport {
		return o.LogTransport.Close()
	}
	return nil
}

// rowBody encodes SQL values directly into an OTLP structured body. It does not
// assign column semantics or copy them into severity, attributes, or resources.
func rowBody(row querylog.Row) (*commonpb.AnyValue, error) {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]*commonpb.KeyValue, 0, len(keys))
	for _, key := range keys {
		v := row[key]
		// Drivers may return precise numeric wrappers such as godror.Number.
		// Use their standard SQL representation without importing a driver.
		if valuer, ok := v.(driver.Valuer); ok {
			var err error
			v, err = valuer.Value()
			if err != nil {
				return nil, fmt.Errorf("column %q: %w", key, err)
			}
		}
		value := &commonpb.AnyValue{}
		switch v := v.(type) {
		case nil:
		case string:
			value.Value = &commonpb.AnyValue_StringValue{StringValue: v}
		case bool:
			value.Value = &commonpb.AnyValue_BoolValue{BoolValue: v}
		case int64:
			value.Value = &commonpb.AnyValue_IntValue{IntValue: v}
		case uint64:
			if v <= 1<<63-1 {
				value.Value = &commonpb.AnyValue_IntValue{IntValue: int64(v)}
			} else {
				value.Value = &commonpb.AnyValue_StringValue{StringValue: strconv.FormatUint(v, 10)}
			}
		case float64:
			value.Value = &commonpb.AnyValue_DoubleValue{DoubleValue: v}
		case []byte:
			value.Value = &commonpb.AnyValue_BytesValue{BytesValue: v}
		case time.Time:
			value.Value = &commonpb.AnyValue_StringValue{StringValue: v.Format(time.RFC3339Nano)}
		default:
			return nil, fmt.Errorf("column %q has unsupported OTLP value type %T; cast it in SQL", key, v)
		}
		values = append(values, &commonpb.KeyValue{Key: key, Value: value})
	}
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: values}}}, nil
}
