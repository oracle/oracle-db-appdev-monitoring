# [Oracle Database Metrics Exporter](https://oracle.github.io/oracle-db-appdev-monitoring/)

View the documentation here: [Oracle Database Metrics Exporter](https://oracle.github.io/oracle-db-appdev-monitoring/)

This project aims to provide observability for the Oracle Database so that users can understand performance and diagnose issues easily across applications and database.  Over time, this project will provide not just metrics, but also logging and tracing support, and integration into popular frameworks like Spring Boot.  The project aims to deliver functionality to support both cloud and on-premises databases, including those running in Kubernetes and containers.

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

## Contributing

This project welcomes contributions from the community. Before submitting a pull request, please [review our contribution guide](./CONTRIBUTING.md)

## Security

Please consult the [security guide](./SECURITY.md) for our responsible security vulnerability disclosure process

## License

Copyright (c) 2016, 2025, Oracle and/or its affiliates.

Released under the Universal Permissive License v1.0 as shown at
<https://oss.oracle.com/licenses/upl/>
and the MIT License (MIT)
