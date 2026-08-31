---
title: OTLP Metric Publishing
sidebar_position: 7
---

# OTLP Metric Publishing

The exporter can periodically publish its collected metrics to an OpenTelemetry Protocol (OTLP) backend over gRPC. OTLP publishing is optional and works alongside the Prometheus `/metrics` endpoint; enabling it does not disable Prometheus collection.

OTLP publishing exports the gauges, counters, histograms, and untyped metrics exposed by the Prometheus `/metrics` endpoint. It requires a positive `metrics.scrapeInterval`. The OTLP reader and scheduled scraper use that same interval, but run on independent schedules.

![Oracle AI Database metrics flow through Prometheus pull and OTLP push](/img/otlp-flow.svg)

## Basic configuration

To enable OTLP push-based metrics, configure the `otlp` object in your metrics exporter config file. The `metrics.scrapeInterval` property must be set, which configures the interval at which metrics are interally collected.

```yaml
metrics:
  scrapeInterval: 15s # Metrics will be collected for OTLP push every 15s

otlp:
  # Collector endpoint
  endpoint: https://otel-collector.example.com:4317
  # gRPC timeout
  timeout: 10s
```

`resourceAttributes` adds OpenTelemetry resource attributes to every exported metric. `service.name` defaults to `oracledb_exporter` and `service.version` defaults to the exporter version; explicitly set either attribute to override its default.

```yaml
otlp:
  # Collector endpoint
  endpoint: https://otel-collector.example.com:4317
  # gRPC timeout
  timeout: 10s
  resourceAttributes:
    deployment.environment: production
    service.namespace: database-observability
```


## Authentication and resource attributes

Use `headers` for static authentication or tenant metadata. Configuration values such as `${OTLP_TOKEN}` are expanded from the environment:

```yaml
metrics:
  scrapeInterval: 15s

otlp:
  endpoint: https://otel-collector.example.com:4317
  timeout: 10s
  headers:
    Authorization: "Bearer ${OTLP_TOKEN}"
```

The exporter sends static `headers` as OTLP gRPC metadata on each export. As with the rest of the exporter configuration, `${VARIABLE}` values are expanded from the environment.


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

## Troubleshooting

### No metrics arrive

- Confirm that the collector has an OTLP **gRPC** receiver. This exporter does not send OTLP/HTTP.
- Confirm that the endpoint includes `http://` or `https://` and uses the collector's gRPC port, commonly `4317`.
- When using containers, do not use `localhost` unless the collector runs in the same container. Use a shared network and the collector service name.
- Wait at least one `metrics.scrapeInterval`, then check the exporter and collector logs.
- Query the exporter's `/metrics` endpoint to confirm that database collection itself is succeeding.

### Authentication or TLS fails

- Check that header environment variables are present in the exporter process. Missing variables expand to empty strings.
- Use `https://` for TLS. An `http://` endpoint cannot be combined with the `tls` section.
- Make sure `caFile`, `certFile`, and `keyFile` paths are readable inside the exporter container or process environment.
- Set `serverName` only when the name used to verify the collector certificate differs from the endpoint host.
- Reserve `insecureSkipVerify` for short-lived diagnostics; it disables certificate verification.

### Exports time out

Increase `otlp.timeout` only after checking collector reachability and responsiveness. A slow export may delay the next periodic export, but it does not stop scheduled database scraping or remove metrics from `/metrics`.

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

`metrics.scrapeInterval` is not part of the `otlp` section, but is required to enable scheduled collection and OTLP publishing.

## Validation rules

- `metrics.scrapeInterval` and `otlp.timeout` must be positive durations.
- `otlp.endpoint` must be an absolute `http://` or `https://` URL.
- `http://` selects plaintext and cannot be combined with `otlp.tls`.
- `certFile` and `keyFile` must be configured together.
