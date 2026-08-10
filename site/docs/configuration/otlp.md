---
title: OTLP Metric Publishing
sidebar_position: 7
---

# OTLP Metric Publishing

The exporter can periodically publish its collected metrics to an OTLP/gRPC collector. OTLP export is optional: when the `otlp` section is not configured, metrics are available only from the Prometheus `/metrics` endpoint.

OTLP publishing exports the gauges, counters, histograms, and untyped metrics exposed by the Prometheus `/metrics` endpoint. It requires a positive `metrics.scrapeInterval`. The OTLP reader and scheduled scraper use that same interval, but run on independent schedules.

## Basic OTLP/gRPC exporter configuration

Use an `http://` endpoint for plaintext gRPC, or an `https://` endpoint for TLS. The scheme is required.

```yaml
metrics:
  scrapeInterval: 15s

otlp:
  endpoint: https://otel-collector.example.com:4317
  timeout: 10s # default 10s
  headers: # optional, specify any request headers
    Authorization: "Bearer ${OTLP_TOKEN}"
  resourceAttributes: # optional, additional OTLP resource attributes
    deployment.environment: production
```

The exporter sends static `headers` as OTLP gRPC metadata on each export. As with the rest of the exporter configuration, `${VARIABLE}` values are expanded from the environment.

`resourceAttributes` adds OpenTelemetry resource attributes to every exported metric. `service.name` defaults to `oracledb_exporter` and `service.version` defaults to the exporter version; explicitly set either attribute to override its default.

## TLS, custom CA, and mTLS

An `https://` endpoint uses TLS. Add the optional `tls` section to customize certificate verification or use mutual TLS:

```yaml
metrics:
  scrapeInterval: 30s

otlp:
  endpoint: https://otel-collector.internal:4317
  tls:
    caFile: /etc/oracledb-exporter/collector-ca.pem
    certFile: /etc/oracledb-exporter/client.pem
    keyFile: /etc/oracledb-exporter/client-key.pem
    serverName: otel-collector.internal
    minVersion: TLS1.3
```

`caFile` adds a PEM-encoded CA certificate to the system trust store for this connection. Configure `certFile` and `keyFile` together to present a client certificate. `serverName` overrides the TLS server name used for certificate verification. `minVersion` may be `TLS1.2` (the default) or `TLS1.3`.

For temporary diagnostics only, `insecureSkipVerify: true` disables TLS certificate verification. Do not use it for a production collector.

## Configuration reference

| Property | Required | Default | Description |
| --- | --- | --- | --- |
| `otlp.endpoint` | Yes | — | OTLP/gRPC endpoint URL. Must use `http://` or `https://`. |
| `otlp.timeout` | No | `10s` | Maximum time allowed for an OTLP export. Must be positive. |
| `otlp.headers` | No | — | Static string headers sent as gRPC metadata. |
| `otlp.resourceAttributes` | No | — | Static OpenTelemetry resource attributes. |
| `otlp.tls.caFile` | No | — | PEM CA certificate file to trust in addition to the system trust store. |
| `otlp.tls.certFile` | No | — | Client certificate file for mTLS; requires `keyFile`. |
| `otlp.tls.keyFile` | No | — | Client private-key file for mTLS; requires `certFile`. |
| `otlp.tls.serverName` | No | endpoint host | TLS server name used to verify the collector certificate. |
| `otlp.tls.minVersion` | No | `TLS1.2` | Minimum TLS version: `TLS1.2` or `TLS1.3`. |
| `otlp.tls.insecureSkipVerify` | No | `false` | Disables TLS certificate verification. Use only for temporary diagnostics. |

## Validation rules

- `metrics.scrapeInterval` and `otlp.timeout` must be positive durations.
- `otlp.endpoint` must be an absolute `http://` or `https://` URL.
- `http://` selects plaintext and cannot be combined with `otlp.tls`.
- `certFile` and `keyFile` must be configured together.
