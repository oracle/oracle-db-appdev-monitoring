---
title: Exporter Configuration
sidebar_position: 1
---

# Exporter Configuration

The recommended way to configure the exporter is with the `--config.file` argument, specifying a YAML configuration file.

The configuration file contains the following options:

```yaml
# Example Oracle AI Database Metrics Exporter Configuration file.
# Environment variables of the form ${VAR_NAME} will be expanded.
# If you include a config value that contains a '$' character, escape that '$' with another '$', e.g.,
# "$test$pwd" => "$$test$$pwd"
# Otherwise, the value will be expanded as an environment variable.

# Example Oracle AI Database Metrics Exporter Configuration file.
# Environment variables of the form ${VAR_NAME} will be expanded.

databases:
  ## Path on which metrics will be served
  # metricsPath: /metrics
  ## Database connection information for the "default" database.
  default:
    ## Database username
    username: ${DB_USERNAME}
    ## Database password
    password: ${DB_PASSWORD}
    ## Database password file
    ## If specified, will load the database password from a file.
    # passwordFile: ${DB_PASSWORD_FILE}
    ## Database connection url
    url: localhost:1521/freepdb1

    ## Metrics query timeout for this database, in seconds
    queryTimeout: 5

    ## Rely on Oracle AI Database External Authentication by network or OS
    # externalAuth: false
    ## Database role
    # role: SYSDBA
    ## Path to Oracle AI Database wallet, if using wallet
    # tnsAdmin: /path/to/database/wallet

    ### Connection settings:
    ### Either the go-sql or Oracle AI Database connection pool may be used.
    ### To use the Oracle AI Database connection pool over the go-sql connection pool,
    ### set maxIdleConns to zero and configure the pool* settings.

    ### Connection pooling settings for the go-sql connection pool
    ## Max open connections for this database using go-sql connection pool
    maxOpenConns: 10
    ## Max idle connections for this database using go-sql connection pool
    maxIdleConns: 10

    ### Connection pooling settings for the Oracle AI Database connection pool
    ## Oracle AI Database connection pool increment.
    # poolIncrement: 1
    ## Oracle AI Database Connection pool maximum size
    # poolMaxConnections: 15
    ## Oracle AI Database Connection pool minimum size
    # poolMinConnections: 15

    ## Arbitrary labels to add to each metric scraped from this database
    # labels:
    #   label_name1: label_value1
    #   label_name2: label_value2

metrics:
  ## The name of the database label applied to each metric. "database" by default.
  # databaseLabel: database
  ## How often to scrape metrics. If not provided, metrics will be scraped on request.
  # scrapeInterval: 15s
  ## How long to wait before attempting to reconnect to an invalid database (login or locked user).
  ## Defaults to 5 minutes.
  # connectionBackoff: 5m
  ## Path to default metrics file.
  default: default-metrics.toml
  ## Paths to any custom metrics files
  custom:
    - custom-metrics-example/custom-metrics.toml

log:
  # Path of log file
  destination: /opt/alert.log
  # Interval of log updates
  interval: 15s
  ## Set disable to 1 to disable logging
  # disable: 0

# Optionally configure prometheus webserver
#web:
#  listenAddresses: [':9161']
#  systemdSocket: true|false
#  configFile: /path/to/webconfigfile
```

From the exporter configuration file, you may optionally load database credentials from [OCI Vault](./oci-vault.md), [Azure Vault](./azure-vault.md), or [HashiCorp Vault](./hashicorp-vault.md)..

### Scrape on request vs. Scrape on interval

The metrics exporter has two scraping modes: scrape on request, and scrape on interval. By default, the metrics exporter scrapes metrics on request, when the `/metrics` endpoint is invoked.

To scrape metrics on a given interval, set the `metrics.scrapeInterval` property to a valid interval:

```yaml
metrics:
  # Metrics will be scraped every 30s.
  scrapeInterval: 30s
```

An individual metric may have its own scrape interval separate from the exporter's scrape interval. See the [metric schema](custom-metrics.md#metric-schema) for details on configuring per-metric scrape intervals.

### Config file in a container image

To add your custom config file to a container image, you can layer the base exporter image and include that config:

```Dockerfile
FROM container-registry.oracle.com/database/observability-exporter:2.2.1
COPY my-exporter-config.yaml /
ENTRYPOINT ["/oracledb_exporter", "--config.file", "/my-exporter-config.yaml"]
```