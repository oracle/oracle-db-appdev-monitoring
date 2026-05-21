---
title: Exporter Configuration
sidebar_position: 1
---

# Exporter Configuration

Configure the exporter with a YAML configuration file, specified with the `--config.file` argument or the `CONFIG_FILE` environment variable.

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
    ## Maximum lifetime for a pooled connection before it is recycled
    connMaxLifetime: 30m
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
  # Log level: debug, info, warn, or error
  level: info
  # Log output format: logfmt or json
  format: logfmt
  # Path of log file
  destination: /opt/alert.log
  # Interval of log updates
  interval: 15s
  ## Set disable to 1 to disable logging
  # disable: 0

# Optionally configure the Prometheus web server
#web:
#  listenAddresses: [':9161']
#  systemdSocket: true|false
#  configFile: /path/to/webconfigfile
#  readHeaderTimeout: 10s
#  readTimeout: 30s
#  idleTimeout: 120s
```

From the exporter configuration file, you may optionally load database credentials from [OCI Vault](./oci-vault.md), [Azure Vault](./azure-vault.md), or [HashiCorp Vault](./hashicorp-vault.md).

### Logging configuration

The optional `log` section configures alert log export and exporter process logging.

- `level`: Process log level. Accepted values are `debug`, `info`, `warn`, and `error`. Defaults to `info`.
- `format`: Process log output format. Accepted values are `logfmt` and `json`. Defaults to `logfmt`.
- `destination`: Base alert log file path. Defaults to `/log/alert.log`.
- `interval`: Interval between alert log updates. Defaults to `15s`.
- `disable`: Disable alert log export when set to `1`. Defaults to `0`.
- `perDatabaseFiles`: Write alert logs to per-database files. Defaults to `false`.

### Web server configuration

The optional `web` section configures the Prometheus Exporter Toolkit web server used by the exporter. These settings are passed directly to the toolkit, so you can use the same web server configuration patterns that are used by other Prometheus exporters.

```yaml
web:
  listenAddresses: [':9161']
  systemdSocket: false
  configFile: /etc/metrics-exporter/web-config.yml
  readHeaderTimeout: 10s
  readTimeout: 30s
  idleTimeout: 120s
```

The `web` properties are:

- `listenAddresses`: One or more addresses for the exporter HTTP server to bind to. For example, `[':9161']` listens on port `9161` on all interfaces.
- `systemdSocket`: Enables systemd socket activation. When set to `true`, systemd provides the listening socket.
- `configFile`: Path to a Prometheus Exporter Toolkit web configuration file. Configure TLS, basic authentication, and other supported web server features in this file.
- `readHeaderTimeout`: Maximum time to read request headers. Defaults to `10s`.
- `readTimeout`: Maximum time to read the full HTTP request. Defaults to `30s`.
- `idleTimeout`: Maximum time to wait for the next request on a keep-alive connection. Defaults to `120s`.

Configure web server security settings such as TLS and basic authentication through `web.configFile`. Those settings should be defined in the Prometheus Exporter Toolkit configuration file, not as exporter-specific properties in the main exporter config file.

For the supported web configuration file format, see the [Prometheus Exporter Toolkit web configuration documentation](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).

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
FROM container-registry.oracle.com/database/observability-exporter:2.3.1
COPY my-exporter-config.yaml /
ENTRYPOINT ["/oracledb_exporter", "--config.file", "/my-exporter-config.yaml"]
```
