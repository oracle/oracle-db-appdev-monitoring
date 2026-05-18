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

For the built-in default metrics, the exporter database database user must have the `SELECT_CATALOG_ROLE` privilege and/or the following object permissions:

```
grant select on sys.dba_tablespace_usage_metrics to exporteruser;
grant select on sys.dba_tablespaces to exporteruser;
grant select on sys.dba_temp_free_space to exporteruser;
grant select on sys.gv_$instance to exporteruser;
grant select on sys.gv_$system_wait_class to exporteruser;
grant select on sys.gv_$asm_diskgroup_stat to exporteruser;
grant select on sys.gv_$datafile to exporteruser;
grant select on sys.gv_$sysstat to exporteruser;
grant select on sys.gv_$process to exporteruser;
grant select on sys.gv_$waitclassmetric to exporteruser;
grant select on sys.gv_$session to exporteruser;
grant select on sys.gv_$resource_limit to exporteruser;
grant select on sys.gv_$parameter to exporteruser;
grant select on sys.gv_$database to exporteruser;
grant select on sys.gv_$sqlstats to exporteruser;
grant select on sys.gv_$con_sysmetric to exporteruser;
grant select on sys.v_$diag_alert_ext to exporteruser; -- for alert logs only
```

Additional permissions may be required for custom metrics, depending on the tables and views used.

## Docker, Podman, etc

You can run the exporter in a local container using a container image from [Oracle Container Registry](https://container-registry.oracle.com).  The container image is available in the "observability-exporter" repository in the "Database" category.  No authentication or license presentment/acceptance are required to pull this image from the registry.

### Oracle AI Database Free

If you need an Oracle AI Database to test the exporter, you can use this command to start up an instance of [Oracle AI Database Free](https://www.oracle.com/database/free/) which also requires no authentication or license presentment/acceptance to pull the image.

```bash
docker run --name free23ai \
    -d \
    -p 1521:1521 \
    -e ORACLE_PASSWORD='<your-password>' \
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
- `DB_PASSWORD` is the password for that user, e.g., `<your-password>`
- `DB_CONNECT_STRING` is the connection string, e.g., `free23ai:1521/freepdb`
- `DB_ROLE` (Optional) can be set to `SYSDBA`, `SYSOPER`, `SYSBACKUP`, `SYSDG`, `SYSKM`, `SYSRAC` or `SYSASM` if you want to connect with one of those roles, however Oracle recommends that you connect with the lowest possible privileges and roles necessary for the exporter to run.

To run the exporter in a container and expose the port, use a command like this, with the appropriate values for the environment variables:

```bash
docker run -it --rm \
    -e DB_USERNAME=pdbadmin \
    -e DB_PASSWORD='<your-password>' \
    -e DB_CONNECT_STRING=free23ai:1521/freepdb \
    -p 9161:9161 \
    container-registry.oracle.com/database/observability-exporter:2.3.1
```

## Standalone Binary

Pre-compiled versions for Linux, ARM and Darwin 64-bit can be found under [releases](https://github.com/oracle/oracle-db-appdev-monitoring/releases).

In order to run, you'll need the [Oracle Instant Client Basic](http://www.oracle.com/technetwork/database/features/instant-client/index-097480.html) for your operating system. Only the basic version is required for the exporter.

> NOTE: If you are running the Standalone binary on a Mac ARM platform you must set the variable `DYLD_LIBRARY_PATH` to the location of where the instant client installed. For example `export DYLD_LIBRARY_PATH=/lib/oracle/instantclient_23_3`.

The exporter requires a YAML configuration file. Pass it with `--config.file`, or set the `CONFIG_FILE` environment variable.

```bash
Usage of oracledb_exporter:
  --config.file string
        File with metrics exporter configuration. (env: CONFIG_FILE)
```

You may reference environment variables from the configuration file. For a simple connection, commonly used variables include:

- `DB_USERNAME` is the database username, e.g., `pdbadmin`
- `DB_PASSWORD` is the password for that user, e.g., `<your-password>`
- `DB_CONNECT_STRING` is the connection string, e.g., `localhost:1521/freepdb1`
- `DB_ROLE` (Optional) can be set to `SYSDBA` or `SYSOPER` if you want to connect with one of those roles, however Oracle recommends that you connect with the lowest possible privileges and roles necessary for the exporter to run.
- `ORACLE_HOME` is the location of the Oracle Instant Client, e.g., `/lib/oracle/21/client64/lib`.
- `TNS_ADMIN` is the location of your (unzipped) wallet.  The `DIRECTORY` set in the `sqlnet.ora` file must match the path that it will be mounted on inside the container.

All exporter settings other than selecting the configuration file are configured in YAML. The following example puts the logfile in the current location with the filename `alert.log` and loads the default metrics file (`default-metrics.toml`) from the current location.

HTTP server request timeouts are configured in the exporter config file under `web.readHeaderTimeout`, `web.readTimeout`, and `web.idleTimeout`.

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

# Optionally configure prometheus webserver
#web:
#  listenAddresses: [':9161']
#  systemdSocket: true|false
#  configFile: /path/to/webconfigfile
```

### Docker Compose

If you would like to set up a test environment with the exporter, you can use the provided "Docker Compose" file in this repository which will start an Oracle AI Database instance, the exporter, Prometheus and Grafana.

```bash
DB_PASSWORD='<choose-a-local-demo-password>' make docker-compose-up
```

The containers will take a short time to start.  The first time, the Oracle container might take a few minutes to start while it creates the database instance, but this is a one-time operation, and subequent restarts will be much faster (a few seconds).

> Warning: This stack is intended for local testing only.  Set `DB_PASSWORD` explicitly before startup, and keep the sample database ports bound to `127.0.0.1` rather than exposing them on a shared or public host.

Once the containers are all running, you can access the services using these URLs:

- [Exporter](http://localhost:9161/metrics)
- [Prometheus](http://localhost:9090) - try a query for "oracle".
- [Grafana](http://localhost:3000) - Log in with `admin:admin` and then reset the Grafana Admin password.

When you're done, shut down the docker compose environment:

```bash
make docker-compose-down
```
