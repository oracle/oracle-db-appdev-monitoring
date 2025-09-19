---
title: Custom Metrics
sidebar_position: 2
---

# Custom Metrics

The exporter allows definition of arbitrary custom metrics in one or more TOML or YAML files.

To specify custom metrics files
exporter, use the `metrics` configuration in the [config file](./config-file.md):

```yaml
metrics:
  ## How often to scrape metrics. If not provided, metrics will be scraped on request.
  # scrapeInterval: 15s
  ## Path to default metrics file.
  default: default-metrics.toml
  ## Paths to any custom metrics files (TOML or YAML)
  custom:
    - custom-metrics-example/custom-metrics.toml
```

You may also use `--custom.metrics` flag followed by a comma separated list of TOML or YAML files, or export `CUSTOM_METRICS` variable environment (`export CUSTOM_METRICS=my-custom-metrics.toml,my-other-custom-metrics.toml`)

### Metric Schema

Metrics files must contain a series of `[[metric]]` definitions, in TOML, or the equivalent definition in a YAML file. Each metric definition must follow the exporter's metric schema:

| Field Name       | Description                                                                                                                                                                                                                                                                         | Type                              | Required | Default                           |
|------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------|----------|-----------------------------------|
| context          | Metric context, used to build metric FQN                                                                                                                                                                                                                                            | String                            | Yes      |                                   |
| labels           | Metric labels, which must match column names in the query. Any column that is not a label will be parsed as a metric                                                                                                                                                                | Array of Strings                  | No       |                                   |
| metricsdesc      | Mapping between field(s) in the request and comment(s)                                                                                                                                                                                                                              | Dictionary of Strings             | Yes      |                                   |
| metricstype      | Mapping between field(s) in the request and [Prometheus metric types](https://prometheus.io/docs/concepts/metric_types/)                                                                                                                                                            | Dictionary of Strings             | No       |                                   |
| metricsbuckets   | Split [histogram](https://prometheus.io/docs/concepts/metric_types/#histogram) metric types into buckets based on value ([example](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/custom-metrics-example/metric-histogram-example.toml))                           | Dictionary of String dictionaries | No       |                                   |
| fieldtoappend    | Field from the request to append to the metric FQN. This field will **not** be included in the metric labels.                                                                                                                                                                       | String                            | No       |                                   |
| request          | Oracle database query to run for metrics scraping                                                                                                                                                                                                                                   | String                            | Yes      |                                   |
| ignorezeroresult | Whether or not an error will be printed if the request does not return any results                                                                                                                                                                                                  | Boolean                           | No       | false                             |
| querytimeout     | Oracle Database query timeout duration, e.g., 300ms, 0.5h                                                                                                                                                                                                                           | String duration                   | No       | Value of query.timeout in seconds |
| scrapeinterval   | Custom metric scrape interval, used if scrape.interval is provided, otherwise metrics are always scraped on request.                                                                                                                                                                | String duration                   | No       |                                   |
| databases        | Array of databases the metric will be scraped from, using the database name from the exporter config file. If not present, the metric is scraped from all databases. If the databases array is empty (`databases = []`) the metric will not be scraped, effectively being disabled. | Array of Strings                  | No       |                                   |

### Example Metric Definition

Here's a simple example of a metric definition:

```toml
[[metric]]
context = "test"
request = "SELECT 1 as value_1, 2 as value_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }
```

> NOTE: Do not add a semicolon (`;`) at the end of the SQL queries.

This file produce the following entries in the exporter:

```text
# HELP oracledb_test_value_1 Simple example returning always 1.
# TYPE oracledb_test_value_1 gauge
oracledb_test_value_1 1
# HELP oracledb_test_value_2 Same but returning always 2.
# TYPE oracledb_test_value_2 gauge
oracledb_test_value_2 2
```

You can also provide labels using `labels` field. Here's an example providing two metrics, with and without labels:

```toml
[[metric]]
context = "context_no_label"
request = "SELECT 1 as value_1, 2 as value_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }

[[metric]]
context = "context_with_labels"
labels = [ "label_1", "label_2" ]
request = "SELECT 1 as value_1, 2 as value_2, 'First label' as label_1, 'Second label' as label_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }
```

This TOML file produces the following result:

```text
# HELP oracledb_context_no_label_value_1 Simple example returning always 1.
# TYPE oracledb_context_no_label_value_1 gauge
oracledb_context_no_label_value_1 1
# HELP oracledb_context_no_label_value_2 Same but returning always 2.
# TYPE oracledb_context_no_label_value_2 gauge
oracledb_context_no_label_value_2 2
# HELP oracledb_context_with_labels_value_1 Simple example returning always 1.
# TYPE oracledb_context_with_labels_value_1 gauge
oracledb_context_with_labels_value_1{label_1="First label",label_2="Second label"} 1
# HELP oracledb_context_with_labels_value_2 Same but returning always 2.
# TYPE oracledb_context_with_labels_value_2 gauge
oracledb_context_with_labels_value_2{label_1="First label",label_2="Second label"} 2
```

Last, you can set metric type using **metricstype** field.

```toml
[[metric]]
context = "context_with_labels"
labels = [ "label_1", "label_2" ]
request = "SELECT 1 as value_1, 2 as value_2, 'First label' as label_1, 'Second label' as label_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1 as counter.", value_2 = "Same but returning always 2 as gauge." }
# Can be counter or gauge (default)
metricstype = { value_1 = "counter" }
```

This TOML file will produce the following result:

```text
# HELP oracledb_test_value_1 Simple test example returning always 1 as counter.
# TYPE oracledb_test_value_1 counter
oracledb_test_value_1 1
# HELP oracledb_test_value_2 Same test but returning always 2 as gauge.
# TYPE oracledb_test_value_2 gauge
oracledb_test_value_2 2
```

You can find [working examples](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/custom-metrics-example/custom-metrics.toml) of custom metrics for slow queries, big queries and top 100 tables.
An example of [custom metrics for Transacational Event Queues](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/custom-metrics-example/txeventq-metrics.toml) is also provided.

### Customize metrics in a container image

If you run the exporter as a container image and want to include your custom metrics in the image itself, you can use the following example `Dockerfile` to create a new image:

```Dockerfile
FROM container-registry.oracle.com/database/observability-exporter:2.0.3
COPY custom-metrics.toml /
ENTRYPOINT ["/oracledb_exporter", "--custom.metrics", "/custom-metrics.toml"]
```