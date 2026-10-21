---
title: Grafana Dashboards
sidebar_position: 4
---

# Grafana Dashboards

Sample Grafana dashboards are included with the exporter.

A sample Grafana dashboard definition is provided [in this directory](https://github.com/oracle/oracle-db-appdev-monitoring/tree/main/docker-compose/grafana/dashboards).  You can import these dashboards into your Grafana instance, and set it to use the Prometheus datasource that you have defined for the Prometheus instance that is collecting metrics from the exporter.

The dashboard shows some basic information, as shown below:

![Oracle AI Database Dashboard](/img/oracledb-dashboard.png)

## Collect metrics with Grafana Alloy

[Grafana Alloy](https://grafana.com/docs/alloy/latest/reference/components/prometheus/prometheus.exporter.oracledb/) includes the `prometheus.exporter.oracledb` component. Use it when you want Alloy to run the exporter and forward the scraped Oracle AI Database metrics to a Prometheus-compatible remote-write endpoint.

```alloy
prometheus.exporter.oracledb "oracle" {
  database {
    name              = "primary"
    connection_string = "db.example.com:1521/ORCL"
    username          = "<DB_USERNAME>"
    password          = "<DB_PASSWORD>"
  }
}

prometheus.scrape "oracle" {
  targets    = prometheus.exporter.oracledb.oracle.targets
  forward_to = [prometheus.remote_write.default.receiver]
}

prometheus.remote_write "default" {
  endpoint {
    url = "<PROMETHEUS_REMOTE_WRITE_URL>"
  }
}
```

Alloy must have Oracle Instant Client Basic available on the host or in its container image. When using Oracle Wallet authentication, also configure `TNS_ADMIN` to point to the wallet directory. See the [Grafana Alloy component reference](https://grafana.com/docs/alloy/latest/reference/components/prometheus/prometheus.exporter.oracledb/) for all available options, including custom metrics and multiple database targets.
