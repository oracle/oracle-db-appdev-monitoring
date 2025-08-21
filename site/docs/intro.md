---
sidebar_position: 1
---

# OpenTelemetry Metrics for Oracle Database

This project aims to provide observability for the Oracle Database so that users can understand performance and diagnose issues easily across applications and database.  Over time, this project will provide not just metrics, but also logging and tracing support, and integration into popular frameworks like Spring Boot.  The project aims to deliver functionality to support both cloud and on-premises databases, including those running in Kubernetes and containers.

Contributions are welcome - please see [contributing](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/CONTRIBUTING.md).

![Oracle Database Dashboard](/img/exporter-running-against-basedb.png)

## Main Features

The exporter supports the following main features

- Exports Oracle Database metrics in standard OTEL/Prometheus format
- Works with on-prem, in the cloud, and in Kubernetes, with single instance, clustered, or Autonomous Oracle Database instances
- Authenticate with plaintext, TLS, and Oracle Wallet
- Secure credentials with Oracle Cloud Infrastructure (OCI) Vault or Azure Vault
- Load metrics from one or more databases using a single exporter instance
- Export the Prometheus Alert Log in JSON format for easy ingest by log aggregators
- Pre-buit AMD64 and ARM64 images provided
- Standard, default metrics included "out of the box"
- Easily define custom metrics using YAML or TOML
- Define the scrape interval, database query timeout, and other parameters on a per-metric, per-database level
- Customize the database connection pool using go-sql, Oracle Database connection pools, and works with Database Resident Connection Pools
- Includes a sample [Grafana dashboards](https://github.com/oracle/oracle-db-appdev-monitoring/tree/main/docker-compose/grafana) for inspiration or customization

From the v1.0 release onwards, this project provides a [Prometheus](https://prometheus.io/) exporter for Oracle Database based in part on a Prometheus exporter created by [Seth Miller](https://github.com/iamseth/oracledb_exporter). This project includes changes to comply with various Oracle standards and policies, as well as new features.

> Seth has archived his exporter as of Feb 13, 2025 and added a note encouraging people to check out ours instead.  We wanted to extend a huge "Thank You!" to Seth for the work he did on that exporter, and his contributions to the Oracle and open source communities!
