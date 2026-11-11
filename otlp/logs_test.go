// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/querylog"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type logService struct {
	collectorlogspb.UnimplementedLogsServiceServer
	request  *collectorlogspb.ExportLogsServiceRequest
	headers  metadata.MD
	reject   bool
	fail     bool
	block    bool
	received atomic.Int64
}

func (s *logService) Export(ctx context.Context, r *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	for _, resource := range r.ResourceLogs {
		for _, scope := range resource.ScopeLogs {
			s.received.Add(int64(len(scope.LogRecords)))
		}
	}
	s.request = r
	s.headers, _ = metadata.FromIncomingContext(ctx)
	if s.fail {
		return nil, status.Error(codes.Unavailable, "unavailable")
	}
	out := &collectorlogspb.ExportLogsServiceResponse{}
	if s.reject {
		out.PartialSuccess = &collectorlogspb.ExportLogsPartialSuccess{RejectedLogRecords: 1, ErrorMessage: "rejected"}
	}
	return out, nil
}
func newLogService(t *testing.T, s *logService) *LogOutput {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	collectorlogspb.RegisterLogsServiceServer(server, s)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	timeout := time.Second
	output, err := NewLogOutput(&config.OTLPConfig{Endpoint: "http://" + listener.Addr().String(), Timeout: &timeout, Headers: map[string]string{"authorization": "Bearer token"}, ResourceAttributes: map[string]string{"service.name": "test"}}, "test-version", "TS")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	return output
}
func TestLogOutputAcknowledgementAndStructuredRow(t *testing.T) {
	service := &logService{}
	output := newLogService(t, service)
	ts := time.Date(2026, 11, 10, 0, 0, 0, 123456789, time.UTC)
	if err := output.Deliver(context.Background(), []querylog.Row{{"TS": ts, "MESSAGE_TEXT": "hello\n", "SEVERITY": "ERROR", "COUNT": int64(42), "RATIO": float64(1.25), "ACTIVE": true, "BINARY": []byte{0, 255}, "NULL_VALUE": nil}}); err != nil {
		t.Fatal(err)
	}
	request := service.request
	resource := request.ResourceLogs[0]
	record := resource.ScopeLogs[0].LogRecords[0]
	if record.TimeUnixNano != uint64(ts.UnixNano()) || record.Body.GetKvlistValue() == nil || record.SeverityText != "" || record.SeverityNumber != 0 || len(record.Attributes) != 0 || record.ObservedTimeUnixNano == 0 {
		t.Fatalf("bad mapping: %+v", record)
	}
	values := map[string]*commonpb.AnyValue{}
	for _, field := range record.Body.GetKvlistValue().Values {
		values[field.Key] = field.Value
	}
	if len(values) != 8 || values["TS"].GetStringValue() != ts.Format(time.RFC3339Nano) || values["MESSAGE_TEXT"].GetStringValue() != "hello\n" || values["SEVERITY"].GetStringValue() != "ERROR" || values["COUNT"].GetIntValue() != 42 || values["RATIO"].GetDoubleValue() != 1.25 || !values["ACTIVE"].GetBoolValue() || !bytes.Equal(values["BINARY"].GetBytesValue(), []byte{0, 255}) || values["NULL_VALUE"].Value != nil {
		t.Fatalf("row columns changed: %+v", values)
	}
	if service.headers.Get("authorization")[0] != "Bearer token" {
		t.Fatal("missing headers")
	}
	for _, attr := range resource.Resource.Attributes {
		if attr.Key == "processId" || attr.Key == "recordId" {
			t.Fatal("high-cardinality resource")
		}
	}
}
func TestLogOutputRejectionAndFailure(t *testing.T) {
	for _, test := range []struct {
		name     string
		service  *logService
		rejected bool
	}{{name: "rejected", service: &logService{reject: true}, rejected: true}, {name: "unavailable", service: &logService{fail: true}}, {name: "timeout", service: &logService{block: true}}} {
		t.Run(test.name, func(t *testing.T) {
			output := newLogService(t, test.service)
			output.timeout = 50 * time.Millisecond
			err := output.Deliver(context.Background(), []querylog.Row{{"TS": time.Now()}})
			if err == nil {
				t.Fatal("failure acknowledged")
			}
			if errors.Is(err, querylog.ErrRejected) != test.rejected {
				t.Fatal(err)
			}
		})
	}
}

// Precise numbers returned by drivers such as godror implement driver.Valuer.
type sqlNumber string

func (n sqlNumber) Value() (driver.Value, error) { return string(n), nil }

func TestOTLPBodyPreservesDriverNumbers(t *testing.T) {
	precise := "999999999999999999999999.123456789"
	body, err := rowBody(querylog.Row{"decimal": sqlNumber(precise), "unsigned": uint64(1<<64 - 1)})
	if err != nil {
		t.Fatal(err)
	}
	values := body.GetKvlistValue().Values
	if len(values) != 2 || values[0].Key != "decimal" || values[0].Value.GetStringValue() != precise || values[1].Key != "unsigned" || values[1].Value.GetStringValue() != "18446744073709551615" {
		t.Fatalf("numeric precision lost: %+v", values)
	}
}

func TestLogOutputSplitsBatchesRejectedForSize(t *testing.T) {
	for _, tc := range []struct {
		name                string
		count, messageBytes int
		wantError           bool
	}{
		{name: "large batch", count: 256, messageBytes: 20000},
		{name: "oversized single record", count: 1, messageBytes: 5 << 20, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &logService{}
			output := newLogService(t, service)
			output.timeout = 10 * time.Second
			rows := make([]querylog.Row, tc.count)
			message := strings.Repeat("x", tc.messageBytes)
			for i := range rows {
				rows[i] = querylog.Row{"TS": time.Now(), "MESSAGE": message}
			}
			err := output.Deliver(context.Background(), rows)
			if tc.wantError {
				if status.Code(err) != codes.ResourceExhausted || service.received.Load() != 0 {
					t.Fatalf("oversized record: error %v, received %d", err, service.received.Load())
				}
			} else if err != nil || service.received.Load() != int64(tc.count) {
				t.Fatalf("batch delivery: error %v, received %d", err, service.received.Load())
			}
		})
	}
}

// Both requests must reach the receiver before either returns, proving that
// stream-specific outputs share the client without serializing deliveries.
type concurrentLogService struct {
	collectorlogspb.UnimplementedLogsServiceServer
	timestamps chan uint64
	release    chan struct{}
}

func (s *concurrentLogService) Export(ctx context.Context, r *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	s.timestamps <- r.ResourceLogs[0].ScopeLogs[0].LogRecords[0].TimeUnixNano
	select {
	case <-s.release:
		return &collectorlogspb.ExportLogsServiceResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestLogTransportConcurrentOutputsAndOwnership(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &concurrentLogService{timestamps: make(chan uint64, 2), release: make(chan struct{})}
	server := grpc.NewServer()
	collectorlogspb.RegisterLogsServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	transport, err := NewLogTransport(&config.OTLPConfig{Endpoint: "http://" + listener.Addr().String()}, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transport.Close() })
	first, second := transport.Output("FIRST"), transport.Output("SECOND")
	if first.client != second.client || first.conn != second.conn {
		t.Fatal("outputs do not share client and connection")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	// Closing a borrowed output must not close another collector's transport.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := time.Now()
	results := make(chan error, 2)
	go func() { results <- first.Deliver(ctx, []querylog.Row{{"FIRST": ts}}) }()
	go func() { results <- second.Deliver(ctx, []querylog.Row{{"SECOND": ts.Add(time.Second)}}) }()
	received := map[uint64]bool{}
	for i := 0; i < 2; i++ {
		select {
		case timestamp := <-service.timestamps:
			received[timestamp] = true
		case <-ctx.Done():
			t.Fatal("concurrent exports did not reach receiver")
		}
	}
	close(service.release)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if !received[uint64(ts.UnixNano())] || !received[uint64(ts.Add(time.Second).UnixNano())] {
		t.Fatal("stream timestamp settings were mixed")
	}
	if err := transport.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Deliver(ctx, []querylog.Row{{"SECOND": ts}}); err == nil {
		t.Fatal("owner close did not shut down transport")
	}
}
