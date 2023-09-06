# Unified Observability for Oracle Database 

This project aims to provide observability for the Oracle Database so that users can understand performance and diagnose issues easily across applications and database.  Over time, this project will provide not just metrics, but also logging and tracing support, and integration into popular frameworks like Spring Boot.  The project aims to deliver functionality to support both cloud and on-premises databases, including those running in Kubernetes and containers.

In the first production release, v1.0, this project provides a [Prometheus](https://prometheus.io/) exporter for Oracle Database that is based in part on a Prometheus exporter created by [iamseth](https://github.com/iamseth/oracledb_exporter) with various changes to comply with various Oracle standards and policies. 

Customers with an active support agreement for Oracle Database may open a Service Request in My Oracle Support for support with any issues using this exporter.  Community support is available through GitHub issues, etc., for other users. 

Contributions are welcome - please see [contributing](CONTRIBUTING.md).


### Table of Contents

[Roadmap](#roadmap)
[Standard metrics](#standard-metrics)  
[Installation](#installation)  
[Running](#running)  
[Grafana](#grafana)  
[Troubleshooting](#troubleshooting)  
[Operating principles](operating-principles.md)

# Roadmap

## Version 1.0

The first production release, v1.0, includes the following features: 

- A number of [standard metrics](#standard-metrics) are exposed,
- Users can define [custom metrics](#custom-metrics),
- Oracle regularly reviews third-party licenses and scans the code and images, including transitive/recursive dependencies for issues,
- Connection to Oracle can be a basic connection or use an Oracle Wallet and TLS - connection to Oracle Autonomous Database is supported,
- Metrics for Oracle Transactional Event Queues are also supported,
- A Grafana dashboard is provided for Transacational Event Queues, and
- A pre-built container image is provided, based on Oracle Linux, and optimized for size and security.

Note that this exporter uses a different Oracle Database driver which in turn uses code directly written by Oracle to access the database.  This driver does require an Oracle client.  In this initial release, the client is bundled into the container image, however we intend to make that optional in order to minimize the image size. 

The interfaces for this version have been kept as close as possible to those of earlier alpha releases in this repository to assist with migration.  However, it should be expected that there may be breaking changes in future releases.

## Plans

We always welcome input on features you would like to see supported.  Please open an issue in this repository with your suggestions. 

Currently, we plan to address the following key features:

- Implement multiple database support - allow the exporter to publish metrics for multiple database instances,
- Implement vault support - allow the exporter to obtain database connection information from a secure vault,
- Implement connection storm protection - prevent the exporter from repeatedly connecting when the credentials fail, to prevent a storm of connections causing accounts to be locked across a large number of databases,
- Provide the option to have the Oracle client outside of the container image, e.g., on a shared volume,
- Implement the ability to update the configuration dynamically, i.e., without a restart,
- Implement support for exporting logs, including audit logs for example, from the database,
- Implement support for tracing within the database, e.g., using an execution context ID provide by an external caller,
- Provide additional pre-built Grafana dashboards,
- Integration with Spring Observability, e.g., Micrometer,
- Provide additional documentation and samples, and
- Integrate with the Oracle Database Operator for Kubernetes.

# Standard metrics

The following metrics are exposed currently.

- oracledb_exporter_last_scrape_duration_seconds
- oracledb_exporter_last_scrape_error
- oracledb_exporter_scrapes_total
- oracledb_up
- oracledb_activity_execute_count
- oracledb_activity_parse_count_total
- oracledb_activity_user_commits
- oracledb_activity_user_rollbacks
- oracledb_sessions_activity
- oracledb_wait_time_application
- oracledb_wait_time_commit
- oracledb_wait_time_concurrency
- oracledb_wait_time_configuration
- oracledb_wait_time_network
- oracledb_wait_time_other
- oracledb_wait_time_scheduler
- oracledb_wait_time_system_io
- oracledb_wait_time_user_io
- oracledb_tablespace_bytes
- oracledb_tablespace_max_bytes
- oracledb_tablespace_free
- oracledb_tablespace_used_percent
- oracledb_process_count
- oracledb_resource_current_utilization
- oracledb_resource_limit_value

# Installation

There are a number of ways to run the exporter.  In this section you will find information on running the exporter:

- In a container runtime like [Docker, Podman, etc](#docker-podman-etc)
- In a test/demo environment using [Docker Compose](#testdemo-environment-with-docker-compose)
- In [Kubernetes](#kubernetes)
- As a [standalone binary](#standalone-binary)


## Docker, Podman, etc.

You can run the exporter in a local container using a conatiner image from [Oracle Container Registry](https://container-registry.oracle.com).  The container image is available in the "observability-exporter" repository in the "Database" category.  No authentication or license presentment/acceptance are required to pull this image from the registry.

### Oracle Database 

If you need an Oracle Database to test the exporter, you can use this command to start up an instance of [Oracle Database 23c Free](https://www.oracle.com/database/free/) which also requires no authentication or license presentment/acceptance to pull the image.

```bash
docker run --name free23c -d -p 1521:1521 -e ORACLE_PWD=Welcome12345 container-registry.oracle.com/database/free:latest
```

This will pull the image and start up the database with a listener on port 1521. It will also create a pluggable database (a database container) called "FREEPDB1" and will set the admin passwords to the password you specified on this command.

You can tail the logs to see when the database is ready to use:

```bash
docker logs -f free23c

(look for this message...)
#########################
DATABASE IS READY TO USE!
#########################
```

To obtain the IP address of the container, which you will need to connect to the database, use this command.  Note: depending on your platform and container runtime, you may be able to access the database at "localhost":

```bash
docker inspect free23c | grep IPA
            "SecondaryIPAddresses": null,
            "IPAddress": "172.17.0.2",
                    "IPAMConfig": null,
                    "IPAddress": "172.17.0.2",
```

### Exporter 

You need to give the exporter the connection details for the Oracle Database that you want it to run against.  You can use a simple connection, or a wallet. 

#### Simple connection

For a simple connection, you will provide the details using these variables: 

- `DB_USERNAME` is the database username, e.g., `pdbadmin`
- `DB_PASSWORD` is the password for that user, e.g., `Welcome12345`
- `DB_CONNECT_STRING` is the connection string, e.g., `free23c:1521/freepdb`

To run the exporter in a container and expose the port, use a command like this, with the appropriate values for the environment variables:

```bash
docker run -it --rm \
    -e DB_USERNAME=pdbadmin \
    -e DB_PASSWORD=Welcome12345 \
    -e DB_CONNECT_STRING=free23c:1521/freepdb \
    -p 9161:9161 \
    container-registry.oracle.com/database/observability-exporter:1.0.0
```

#### Using a wallet

For a wallet connection, you must first set up the wallet.  If you are using Oracle Autonomous Database, for example, you can download the wallet from the Oracle Cloud Infrastructure (OCI) console.  

1. Unzip the wallet into a new directory, e.g., called `wallet`.
1. Edit the `sqlnet.ora` file and set the `DIRECTORY` to `/wallet`.  This is the path inside the exporter container where you will provide the wallet.
1. Take a note of the TNS name from the `tnsnames.ora` that will be used to connect to the database, e.g., `devdb_tp`.

Now, you provide the connection details using these variables: 

- `DB_USERNAME` is the database username, e.g., `pdbadmin`
- `DB_PASSWORD` is the password for that user, e.g., `Welcome12345`
- `DB_CONNECT_STRING` is the connection string, e.g., `devdb_tp?TNS_ADMIN=/wallet`

To run the exporter in a container and expose the port, use a command like this, with the appropriate values for the environment variables, and mounting your `wallet` directory to provide access to the wallet:

```bash
docker run -it --rm \
    -e DB_USERNAME=pdbadmin \
    -e DB_PASSWORD=Welcome12345 \
    -e DB_CONNECT_STRING=devdb_tp \
    -v ./wallet:/wallet \
    -p 9161:9161 \
    container-registry.oracle.com/database/observability-exporter:1.0.0
```


## Test/demo environment with Docker Compose

If you would like to set up a test environment with the exporter, you can use the provided "Docker Compose" file in this repository which will start an Oracle Database instance, the exporter, Prometheus and Grafana.

```bash
cd docker-compose
docker-compose up -d
```

The containers will take a short time to start.  The first time, the Oracle container might take a few minutes to start while it creates the database instance, but this is a one-time operation, and subequent restarts will be much faster (a few seconds). 

Once the containers are all running, you can access the services using these URLs:

- [Exporter](http://localhost:9161/metrics)
- [Prometheus](http://localhost:9000) - try a query for "oracle".
- [Grafana](http://localhost:3000) - username is "admin" and password is "grafana".  Try creating a dashboard using one of the metrics from the exporter (use the Prometheus datasource and choose a metric with "oracle" in the name).

## Kubernetes

write me

## Standalone binary

write me


# END

## Binary Release

Pre-compiled versions for Linux 64 bit and Mac OSX 64 bit can be found under [releases](https://github.com/iamseth/oracledb_exporter/releases).

In order to run, you'll need the [Oracle Instant Client Basic](http://www.oracle.com/technetwork/database/features/instant-client/index-097480.html)
for your operating system. Only the basic version is required for execution.

# Running
Ensure that the environment variable DATA_SOURCE_NAME is set correctly before starting.
DATA_SOURCE_NAME should be in Oracle Database connection string format:  

```conn
    oracle://user:pass@server/service_name[?OPTION1=VALUE1[&OPTIONn=VALUEn]...]
```

For Example:

```bash
# export Oracle location:
export DATA_SOURCE_NAME=oracle://system:password@oracle-sid
# or using a complete url:
export DATA_SOURCE_NAME=oracle://user:password@myhost:1521/service
# 19c client for primary/standby configuration
export DATA_SOURCE_NAME=oracle://user:password@primaryhost:1521,standbyhost:1521/service
# 19c client for primary/standby configuration with options
export DATA_SOURCE_NAME=oracle://user:password@primaryhost:1521,standbyhost:1521/service?connect_timeout=5&transport_connect_timeout=3&retry_count=3
# 19c client for ASM instance connection (requires SYSDBA)
export DATA_SOURCE_NAME=oracle://user:password@primaryhost:1521,standbyhost:1521/+ASM?as=sysdba
# Then run the exporter
/path/to/binary/oracledb_exporter --log.level error --web.listen-address 0.0.0.0:9161
```
## Default-metrics requirement
Make sure to grant `SYS` privilege on `SELECT` statement for the monitoring user, on the following tables.
```
dba_tablespace_usage_metrics
dba_tablespaces
v$system_wait_class
v$asm_diskgroup_stat
v$datafile
v$sysstat
v$process
v$waitclassmetric
v$session
v$resource_limit
```

# Integration with System D

Create **oracledb_exporter** user with disabled login and **oracledb_exporter** group\
mkdir /etc/oracledb_exporter\
chown root:oracledb_exporter /etc/oracledb_exporter  
chmod 775 /etc/oracledb_exporter  
Put config files to **/etc/oracledb_exporter**  
Put binary to **/usr/local/bin**

Create file **/etc/systemd/system/oracledb_exporter.service** with the following content:

```bash
[Unit]
Description=Service for oracle telemetry client
After=network.target
[Service]
Type=oneshot
#!!! Set your values and uncomment
#User=oracledb_exporter
#Group=oracledb_exporter
#Environment="DATA_SOURCE_NAME=dbsnmp/Bercut01@//primaryhost:1521,standbyhost:1521/myservice?transport_connect_timeout=5&retry_count=3"
#Environment="LD_LIBRARY_PATH=/u01/app/oracle/product/19.0.0/dbhome_1/lib"
#Environment="ORACLE_HOME=/u01/app/oracle/product/19.0.0/dbhome_1"
#Environment="CUSTOM_METRICS=/etc/oracledb_exporter/custom-metrics.toml"
ExecStart=/usr/local/bin/oracledb_exporter  \
  --default.metrics "/etc/oracledb_exporter/default-metrics.toml"  \
  --log.level error --web.listen-address 0.0.0.0:9161
[Install]
WantedBy=multi-user.target
```

Then tell System D to read files:

    systemctl daemon-reload

Start this new service:

    systemctl start oracledb_exporter

Check service status:

    systemctl status oracledb_exporter

## Usage

```bash
Usage of oracledb_exporter:
  --log.format value
       	If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.
  --log.level value
       	Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal].
  --custom.metrics string
        File that may contain various custom metrics in a TOML file.
  --default.metrics string
        Default TOML file metrics.
  --web.systemd-socket
        Use systemd socket activation listeners instead of port listeners (Linux only).
  --web.listen-address string
       	Address to listen on for web interface and telemetry. (default ":9161")
  --web.telemetry-path string
       	Path under which to expose metrics. (default "/metrics")
  --database.maxIdleConns string
        Number of maximum idle connections in the connection pool. (default "0")
  --database.maxOpenConns string
        Number of maximum open connections in the connection pool. (default "10")
  --web.config.file
        Path to configuration file that can enable TLS or authentication.
```

# Default metrics

This exporter comes with a set of default metrics defined in **default-metrics.toml**. You can modify this file or
provide a different one using `default.metrics` option.

# Custom metrics

> NOTE: Do not put a `;` at the end of your SQL queries as this will **NOT** work.

This exporter does not have the metrics you want? You can provide new one using TOML file. To specify this file to the
exporter, you can:

- Use `--custom.metrics` flag followed by the TOML file
- Export CUSTOM_METRICS variable environment (`export CUSTOM_METRICS=my-custom-metrics.toml`)

This file must contain the following elements:

- One or several metric section (`[[metric]]`)
- For each section a context, a request and a map between a field of your request and a comment.

Here's a simple example:

```
[[metric]]
context = "test"
request = "SELECT 1 as value_1, 2 as value_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }
```

This file produce the following entries in the exporter:

```
# HELP oracledb_test_value_1 Simple example returning always 1.
# TYPE oracledb_test_value_1 gauge
oracledb_test_value_1 1
# HELP oracledb_test_value_2 Same but returning always 2.
# TYPE oracledb_test_value_2 gauge
oracledb_test_value_2 2
```

You can also provide labels using labels field. Here's an example providing two metrics, with and without labels:

```
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

This TOML file produce the following result:

```
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

```
[[metric]]
context = "context_with_labels"
labels = [ "label_1", "label_2" ]
request = "SELECT 1 as value_1, 2 as value_2, 'First label' as label_1, 'Second label' as label_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1 as counter.", value_2 = "Same but returning always 2 as gauge." }
# Can be counter or gauge (default)
metricstype = { value_1 = "counter" }
```

This TOML file will produce the following result:

```
# HELP oracledb_test_value_1 Simple test example returning always 1 as counter.
# TYPE oracledb_test_value_1 counter
oracledb_test_value_1 1
# HELP oracledb_test_value_2 Same test but returning always 2 as gauge.
# TYPE oracledb_test_value_2 gauge
oracledb_test_value_2 2
```

You can find [here](./custom-metrics-example/custom-metrics.toml) a working example of custom metrics for slow queries, big queries and top 100 tables.

# Customize metrics in a docker image

If you run the exporter as a docker image and want to customize the metrics, you can use the following example:

```Dockerfile
FROM iamseth/oracledb_exporter:latest

COPY custom-metrics.toml /

ENTRYPOINT ["/oracledb_exporter", "--custom.metrics", "/custom-metrics.toml"]
```

# Using a multiple host data source name

> NOTE: This has been tested with v0.2.6a and will most probably work on versions above.

> NOTE: While `user/password@//database1.example.com:1521,database3.example.com:1521/DBPRIM` works with SQLPlus, it doesn't seem to work with oracledb-exporter v0.2.6a.

In some cases, one might want to scrape metrics from the currently available database when having a active-passive replication setup.

This will try to connect to any available database to scrape for the metrics. With some replication options, the secondary database is not available when replicating. This allows the scraper to automatically fall back in case of the primary one failing.

This example allows to achieve this:

### Files & Folder:

- tns_admin folder: `/path/to/tns_admin`
- tnsnames.ora file: `/path/to/tns_admin/tnsnames.ora`

Example of a tnsnames.ora file:

```
database =
(DESCRIPTION =
  (ADDRESS_LIST =
    (ADDRESS = (PROTOCOL = TCP)(HOST = database1.example.com)(PORT = 1521))
    (ADDRESS = (PROTOCOL = TCP)(HOST = database2.example.com)(PORT = 1521))
  )
  (CONNECT_DATA =
    (SERVICE_NAME = DBPRIM)
  )
)
```

### Environment Variables

- `TNS_ENTRY`: Name of the entry to use (`database` in the example file above)
- `TNS_ADMIN`: Path you choose for the tns admin folder (`/path/to/tns_admin` in the example file above)
- `DATA_SOURCE_NAME`: Datasource pointing to the `TNS_ENTRY` (`user:password@database` in the example file above)

# TLS connection to database

First, set the following variables:

    export WALLET_PATH=/wallet/path/to/use
    export TNS_ENTRY=tns_entry
    export DB_USERNAME=db_username
    export TNS_ADMIN=/tns/admin/path/to/use

Create the wallet and set the credential:

    mkstore -wrl $WALLET_PATH -create
    mkstore -wrl $WALLET_PATH -createCredential $TNS_ENTRY $DB_USERNAME

Then, update sqlnet.ora:

    echo "
    WALLET_LOCATION = (SOURCE = (METHOD = FILE) (METHOD_DATA = (DIRECTORY = $WALLET_PATH )))
    SQLNET.WALLET_OVERRIDE = TRUE
    SSL_CLIENT_AUTHENTICATION = FALSE
    " >> $TNS_ADMIN/sqlnet.ora

To use the wallet, use the wallet_location parameter. You may need to disable ssl verification with the
ssl_server_dn_match parameter.

Here a complete example of string connection:

    DATA_SOURCE_NAME=oracle://username:password@server:port/service?ssl_server_dn_match=false&wallet_location=wallet_path

For more details, have a look at the following location: https://github.com/iamseth/oracledb_exporter/issues/84

# Integration with Grafana

An example Grafana dashboard is available [here](https://grafana.com/grafana/dashboards/3333-oracledb/).

# Build

## Docker build

To build Ubuntu and Alpine image, run the following command:

    make docker

You can also build only Ubuntu image:

    make ubuntu-image

Or Alpine:

    make alpine-image

## Building Binaries

Run build:

```sh
    make go-build
```

will output binaries and archive inside the `dist` folder for the building operating system.

## Import into your Golang Application

The `oracledb_exporter` can also be imported into your Go based applications. The [Grafana Agent](https://github.com/grafana/agent/) uses this pattern to implement the [OracleDB integration](https://grafana.com/docs/grafana-cloud/data-configuration/integrations/integration-reference/integration-oracledb/). Feel free to modify the code to fit your application's use case.

Here is a small snippet of an example usage of the exporter in code:

```go
 promLogConfig := &promlog.Config{}
 // create your own config
 logger := promlog.New(promLogConfig)

 // replace with your connection string
 connectionString := "oracle://username:password@localhost:1521/orcl.localnet"
 oeExporter, err := oe.NewExporter(logger, &oe.Config{
  DSN:          connectionString,
  MaxIdleConns: 0,
  MaxOpenConns: 10,
  QueryTimeout: 5,
 })

 if err != nil {
  panic(err)
 }

 metricChan := make(chan prometheus.Metric, len(oeExporter.DefaultMetrics().Metric))
 oeExporter.Collect(metricChan)

 // alternatively its possible to run scrapes on an interval
 // and Collect() calls will only return updated data once
 // that intervaled scrape is run
 // please note this is a blocking call so feel free to run
 // in a separate goroutine
 // oeExporter.RunScheduledScrapes(context.Background(), time.Minute)

 for r := range metricChan {
  // Write to the client of your choice
  // or spin up a promhttp.Server to serve these metrics
  r.Write(&dto.Metric{})
 }

```

# FAQ/Troubleshooting

## Unable to convert current value to float (metric=par,metri...in.go:285

Oracle is trying to send a value that we cannot convert to float. This could be anything like 'UNLIMITED' or 'UNDEFINED' or 'WHATEVER'.

In this case, you must handle this problem by testing it in the SQL request. Here an example available in default metrics:

```toml
[[metric]]
context = "resource"
labels = [ "resource_name" ]
metricsdesc = { current_utilization= "Generic counter metric from v$resource_limit view in Oracle (current value).", limit_value="Generic counter metric from v$resource_limit view in Oracle (UNLIMITED: -1)." }
request="SELECT resource_name,current_utilization,CASE WHEN TRIM(limit_value) LIKE 'UNLIMITED' THEN '-1' ELSE TRIM(limit_value) END as limit_value FROM v$resource_limit"
```

If the value of limite_value is 'UNLIMITED', the request send back the value -1.

You can increase the log level (`--log.level debug`) in order to get the statement generating this error.

## error while loading shared libraries: libclntsh.so.xx.x: cannot open shared object file: No such file or directory

This exporter use libs from Oracle in order to connect to Oracle Database. If you are running the binary version, you
must install the Oracle binaries somewhere on your machine and **you must install the good version number**. If the
error talk about the version 18.3, you **must** install 18.3 binary version. If it's 12.2, you **must** install 12.2.

An alternative is to run this exporter using a Docker container. This way, you don't have to worry about Oracle binaries
version as they are embedded in the container.

Here an example to run this exporter (to scrap metrics from system/oracle@//host:1521/service-or-sid) and bind the exporter port (9161) to the global machine:

`docker run -it --rm -p 9161:9161 -e DATA_SOURCE_NAME=oracle://system/oracle@//host:1521/service-or-sid iamseth/oracledb_exporter:0.2.6a`

## Error scraping for wait_time

If you experience an error `Error scraping for wait_time: sql: Scan error on column index 1: converting driver.Value type string (",01") to a float64: invalid syntax source="main.go:144"` you may need to set the NLS_LANG variable.

```bash

export NLS_LANG=AMERICAN_AMERICA.WE8ISO8859P1
export DATA_SOURCE_NAME=system/oracle@myhost
/path/to/binary --log.level error --web.listen-address :9161
```

If using Docker, set the same variable using the -e flag.

## An Oracle instance generates a lot of trace files being monitored by exporter

As being said, Oracle instance may (and probably does) generate a lot of trace files alongside its alert log file, one trace file per scraping event. The trace file contains the following lines

```
...
*** MODULE NAME:(prometheus_oracle_exporter-amd64@hostname)
...
kgxgncin: clsssinit: CLSS init failed with status 3
kgxgncin: clsssinit: return status 3 (0 SKGXN not av) from CLSS
```

The root cause is Oracle's reaction of quering ASM-related views without ASM used. The current workaround proposed is to setup a regular task to cleanup these trace files from the filesystem, as example

```
$ find $ORACLE_BASE/diag/rdbms -name '*.tr[cm]' -mtime +14 -delete
```

## TLS and basic authentication

Apache Exporter supports TLS and basic authentication. This enables better
control of the various HTTP endpoints.

To use TLS and/or basic authentication, you need to pass a configuration file
using the `--web.config` parameter. The format of the file is described
[in the exporter-toolkit repository](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).

Note that the TLS and basic authentication settings affect all HTTP endpoints:
/metrics for scraping, /probe for probing, and the web UI.


## Multi-target support

This exporter supports the multi-target pattern. This allows running a single instance of this exporter for multiple Oracle targets.

To use the multi-target functionality, send a http request to the endpoint `/scrape?target=foo:1521` where target is set to the DSN of the Oracle instance to scrape metrics from.
