---
title: Development
sidebar_position: 3
---

# Development

The exporter is a Go program using the Prometheus SDK. 

External contributions are welcome, see [CONTRIBUTING](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/CONTRIBUTING.md) for details.

The exporter initialization is as follows:

- Parse flags options and configuration properties
- Load the default toml file (`default-metrics.toml`) and store each metric in a `Metric` struct
- Load the custom toml file (if a custom toml file is given)
- Create an `Exporter` object
- Register exporter in prometheus library
- Launching a web server to handle incoming requests
- Attempt connection to any configured Oracle Database servers

These operations are mainly done in the `main` function.

After this initialization phase, the exporter will wait for the arrival of a request.

Each time, it will iterate over the content of the `metricsToScrape` structure (in the function scrape `func (e * Export) scrape (ch chan <- prometheus.Metric)`).

For each element (of `Metric` type), a call to the `ScrapeMetric` function will be made which will itself make a call to the `ScrapeGenericValues` function.

The `ScrapeGenericValues` function will read the information from the `Metric` structure and, depending on the parameters, will generate the metrics to return. In particular, it will use the `GeneratePrometheusMetrics` function which will make SQL calls to the database.

### Docker/container build

To build a container image, run the following command:

```bash
make docker
```

For ARM:

```bash
make docker-arm
```

### Building Binaries

Run build:

```bash
make go-build
```

This will create binaries and archives inside the `dist` folder for the building operating system.
