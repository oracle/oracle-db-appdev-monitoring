---
title: Installation
sidebar_position: 1
---

# Installation

In this section you will find information on running the exporter.

- In a container runtime like [Docker, Podman, etc](#docker-podman-etc)
- In a test/demo environment using [Docker Compose](#docker-compose)
- In [Kubernetes](./kubernetes.md)
- As a [standalone binary](#standalone-binary)

## Database Permissions

For the built-in default metrics, the exporter database database user must have the `SELECT_CATALOG_ROLE` privilege and/or `SELECT` permission on the following objects:

```
dba_tablespace_usage_metrics
dba_tablespaces
gv$system_wait_class
gv$asm_diskgroup_stat
gv$datafile
gv$sysstat
gv$process
gv$waitclassmetric
gv$session
gv$resource_limit
gv$parameter
gv$database
gv$sqlstats
gv$sysmetric
v$diag_alert_ext (for alert logs only)
```

## Docker, Podman, etc

You can run the exporter in a local container using a container image from [Oracle Container Registry](https://container-registry.oracle.com).  The container image is available in the "observability-exporter" repository in the "Database" category.  No authentication or license presentment/acceptance are required to pull this image from the registry.

### Oracle AI Database Free

If you need an Oracle AI Database to test the exporter, you can use this command to start up an instance of [Oracle AI Database Free](https://www.oracle.com/database/free/) which also requires no authentication or license presentment/acceptance to pull the image.

```bash
docker run --name free23ai \
    -d \
    -p 1521:1521 \
    -e ORACLE_PASSWORD=Welcome12345 \
    gvenzl/oracle-free:23.9-slim-faststart
```

This will pull the image and start up the database with a listener on port 1521. It will also create a pluggable database (a database container) called "FREEPDB1" and will set the admin passwords to the password you specified on this command.

You can tail the logs to see when the database is ready to use:

```bash
docker logs -f free23ai

(look for this message...)
#########################
DATABASE IS READY TO USE!
#########################
```

To obtain the IP address of the container, which you will need to connect to the database, use this command.  Note: depending on your platform and container runtime, you may be able to access the database at "localhost":

```bash
docker inspect free23ai | grep IPA
    "SecondaryIPAddresses": null,
    "IPAddress": "172.17.0.2",
            "IPAMConfig": null,
            "IPAddress": "172.17.0.2",
```

### Exporter

You need to give the exporter the connection details for the Oracle AI Database that you want it to run against.  You can use a simple connection, or a wallet.

### Simple connection

For a simple connection, you will provide the details using these variables:

- `DB_USERNAME` is the database username, e.g., `pdbadmin`
- `DB_PASSWORD` is the password for that user, e.g., `Welcome12345`
- `DB_CONNECT_STRING` is the connection string, e.g., `free23ai:1521/freepdb`
- `DB_ROLE` (Optional) can be set to `SYSDBA`, `SYSOPER`, `SYSBACKUP`, `SYSDG`, `SYSKM`, `SYSRAC` or `SYSASM` if you want to connect with one of those roles, however Oracle recommends that you connect with the lowest possible privileges and roles necessary for the exporter to run.

To run the exporter in a container and expose the port, use a command like this, with the appropriate values for the environment variables:

```bash
docker run -it --rm \
    -e DB_USERNAME=pdbadmin \
    -e DB_PASSWORD=Welcome12345 \
    -e DB_CONNECT_STRING=free23ai:1521/freepdb \
    -p 9161:9161 \
    container-registry.oracle.com/database/observability-exporter:2.2.0
```

## Standalone Binary

Pre-compiled versions for Linux, ARM and Darwin 64-bit can be found under [releases](https://github.com/oracle/oracle-db-appdev-monitoring/releases).

In order to run, you'll need the [Oracle Instant Client Basic](http://www.oracle.com/technetwork/database/features/instant-client/index-097480.html) for your operating system. Only the basic version is required for the exporter.

> NOTE: If you are running the Standalone binary on a Mac ARM platform you must set the variable `DYLD_LIBRARY_PATH` to the location of where the instant client installed. For example `export DYLD_LIBRARY_PATH=/lib/oracle/instantclient_23_3`.

The following command line arguments (flags) can be passed to the exporter (the --help flag will show the table below).

```bash
Usage of oracledb_exporter:
      --config.file="example-config.yaml"
                                 File with metrics exporter configuration.  (env: CONFIG_FILE)
      --web.telemetry-path="/metrics"
                                 Path under which to expose metrics. (env: TELEMETRY_PATH)
      --default.metrics="default-metrics.toml"
                                 File with default metrics in a TOML file. (env: DEFAULT_METRICS)
      --custom.metrics=""        Comma separated list of file(s) that contain various custom metrics in a TOML format. (env: CUSTOM_METRICS)
      --query.timeout=5          Query timeout (in seconds). (env: QUERY_TIMEOUT)
      --database.maxIdleConns=0  Number of maximum idle connections in the connection pool. (env: DATABASE_MAXIDLECONNS)
      --database.maxOpenConns=10
                                 Number of maximum open connections in the connection pool. (env: DATABASE_MAXOPENCONNS)
      --database.poolIncrement=-1
                                 Connection increment when the connection pool reaches max capacity. (env: DATABASE_POOLINCREMENT)
      --database.poolMaxConnections=-1
                                 Maximum number of connections in the connection pool. (env: DATABASE_POOLMAXCONNECTIONS)
      --database.poolMinConnections=-1
                                 Minimum number of connections in the connection pool. (env: DATABASE_POOLMINCONNECTIONS)
      --scrape.interval=0s       Interval between each scrape. Default is to scrape on collect requests.
      --log.disable=0            Set to 1 to disable alert logs
      --log.interval=15s         Interval between log updates (e.g. 5s).
      --log.destination="/log/alert.log"
                                 File to output the alert log to. (env: LOG_DESTINATION)
      --web.listen-address=:9161 ...
                                 Addresses on which to expose metrics and web interface. Repeatable for multiple addresses. Examples: `:9100` or `[::1]:9100` for http, `vsock://:9100` for vsock
      --web.config.file=""       Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md
      --log.level=info           Only log messages with the given severity or above. One of: [debug, info, warn, error]
      --log.format=logfmt        Output format of log messages. One of: [logfmt, json]
      --[no-]version             Show application version.
```

You may provide the connection details using these variables:

- `DB_USERNAME` is the database username, e.g., `pdbadmin`
- `DB_PASSWORD` is the password for that user, e.g., `Welcome12345`
- `DB_CONNECT_STRING` is the connection string, e.g., `localhost:1521/freepdb1`
- `DB_ROLE` (Optional) can be set to `SYSDBA` or `SYSOPER` if you want to connect with one of those roles, however Oracle recommends that you connect with the lowest possible privileges and roles necessary for the exporter to run.
- `ORACLE_HOME` is the location of the Oracle Instant Client, e.g., `/lib/oracle/21/client64/lib`.
- `TNS_ADMIN` is the location of your (unzipped) wallet.  The `DIRECTORY` set in the `sqlnet.ora` file must match the path that it will be mounted on inside the container.

The following example puts the logfile in the current location with the filename `alert.log` and loads the default matrics file (`default-metrics,toml`) from the current location.

If you prefer to provide configuration via a [config file](../configuration/config-file.md), you may do so with the `--config.file` argument. The use of a config file over command line arguments is preferred. If a config file is not provided, the "default" database connection is managed by command line arguments.

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
  ## How often to scrape metrics. If not provided, metrics will be scraped on request.
  # scrapeInterval: 15s
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

### Docker Compose

If you would like to set up a test environment with the exporter, you can use the provided "Docker Compose" file in this repository which will start an Oracle AI Database instance, the exporter, Prometheus and Grafana.

```bash
cd docker-compose
docker-compose up -d
```

The containers will take a short time to start.  The first time, the Oracle container might take a few minutes to start while it creates the database instance, but this is a one-time operation, and subequent restarts will be much faster (a few seconds).

Once the containers are all running, you can access the services using these URLs:

- [Exporter](http://localhost:9161/metrics)
- [Prometheus](http://localhost:9090) - try a query for "oracle".
- [Grafana](http://localhost:3000) - username is "admin" and password is "grafana".  An Oracle AI Database dashboard is provisioned and configured to use data from the exporter.