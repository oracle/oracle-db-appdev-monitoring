---
sidebar_position: 1
---

# OpenTelemetry Metrics for Oracle AI Database

The Oracle AI Database Metrics Exporter collects metrics that help you monitor the health, performance, and availability of Oracle AI Database instances from anywhere.

Expose metrics in Prometheus format on the default `/metrics` endpoint, [publish them using OTLP/gRPC](./configuration/otlp.md) to an OpenTelemetry backend, or use both delivery paths at the same time.

The metrics supports monitoring multiple database instances, querying default and custom metrics, and ensures that if one database goes offline for maintenance, the rest stay monitored.

Over time, this project will provide not just metrics, but also logging and tracing support, and integration into popular frameworks like Spring Boot. The project aims to deliver functionality to support both cloud and on-premises databases, including those running in Kubernetes and containers.

Contributions are welcome - please see [contributing](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/CONTRIBUTING.md).

![Oracle AI Database Dashboard](/img/exporter-running-against-basedb.png)

## Main Features

The exporter supports the following main features

- Expose Oracle AI Database metrics in Prometheus format on the `/metrics` endpoint
- Publish metrics to an OpenTelemetry backend using OTLP/gRPC
- Works with on-prem, in the cloud, and in Kubernetes, with single instance, clustered, or Autonomous Oracle AI Database instances
- Authenticate with plaintext, TLS, and Oracle Wallet
- Secure credentials with Oracle Cloud Infrastructure (OCI) Vault, Azure Vault, or HashiCorp Vault
- Load metrics from one or more databases using a single exporter instance
- Export the Prometheus Alert Log in JSON format for easy ingest by log aggregators
- Pre-built AMD64 and ARM64 images provided
- Standard, default metrics included "out of the box"
- Easily define custom metrics using YAML or TOML
- Rich [metric customization](./configuration/custom-metrics.md) defining scrape interval, database query timeout, and other parameters on a per-metric, per-database level
- Customize the database connection pool using go-sql, Oracle AI Database connection pools, and works with Database Resident Connection Pools
- Includes a sample [Grafana dashboards](https://github.com/oracle/oracle-db-appdev-monitoring/tree/main/docker-compose/grafana) for inspiration or customization
- Available as a [Grafana Alloy](https://grafana.com/docs/alloy/latest/reference/components/prometheus/prometheus.exporter.oracledb/) component

## Supported Oracle AI Database Versions

The Oracle AI Database Metrics Exporter natively supports Oracle AI Database versions 19c and 26ai.

## Acknowledgements

From the v1.0 release onwards, this project provides a [Prometheus](https://prometheus.io/) exporter for Oracle AI Database based in part on a Prometheus exporter created by [Seth Miller](https://github.com/iamseth/oracledb_exporter). This project includes changes to comply with various Oracle standards and policies, as well as new features.

> Seth has archived his exporter as of Feb 13, 2025 and added a note encouraging people to check out ours instead.  We wanted to extend a huge "Thank You!" to Seth for the work he did on that exporter, and his contributions to the Oracle and open source communities!
