---
title: Roadmap
sidebar_position: 1
---

# Exporter Roadmap

Planned and upcoming features for the exporter.

We welcome input on community-driven features you'd like to see supported. Please open an issue in this repository with your suggestions.

Currently, we plan to address the following key features:

- Provide default Oracle Exadata metrics
- Implement connection storm protection: prevent the exporter from repeatedly connecting when the credentials fail, to prevent a storm of connections causing accounts to be locked across a large number of databases
- Provide the option to have the Oracle client outside of the container image, e.g., on a shared volume,
- Implement the ability to update the configuration dynamically, i.e., without a restart
- Implement support for tracing within the database, e.g., using an execution context ID provide by an external caller
- Provide additional pre-built Grafana dashboards,
- Integration with Spring Observability, e.g., Micrometer
- Provide additional documentation and samples
