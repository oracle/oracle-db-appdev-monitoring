# Unified App Dev Monitoring with Oracle Database

This v1 (preview) distribution contains scripts and code for exporting metrics, logs, and traces from any Oracle Database to provide converged observability for data-centric applications. 

Metrics from the application layer, Kubernetes, and Oracle Database can be combined to provide unified observability to developers within a single Grafana console. 

Each 

### Build

Build without running tests using the following.

`mvn clean package -DskipTests`

Tests use a live database and require `DATA_SOURCE_NAME` environment variable be set (see section on Running) and can be run using the following.

`mvn clean package`

Docker image can be build using the following.

`./build.sh`

Docker image can be pushed to $DOCKER_REGISTRY using the following.

`./push.sh`

### Running

Ensure  the environment variable DATA_SOURCE_NAME is set correctly before starting.
DATA_SOURCE_NAME should be in Oracle EZCONNECT format:  
<https://docs.oracle.com/en/database/oracle/oracle-database/19/netag/configuring-naming-methods.html#GUID-B0437826-43C1-49EC-A94D-B650B6A4A6EE>  
19c Oracle Client supports enhanced EZCONNECT, you are able to failover to standby DB or gather some heavy metrics from active standby DB and specify some additional parameters. Within 19c client you are able to connect 12c primary/standby DB too :)

For Example:

```bash
# export Oracle location:
export DATA_SOURCE_NAME=system/password@oracle-sid
# or using a complete url:
export DATA_SOURCE_NAME=user/password@//myhost:1521/service
# 19c client for primary/standby configuration
export DATA_SOURCE_NAME=user/password@//primaryhost:1521,standbyhost:1521/service
# 19c client for primary/standby configuration with options
export DATA_SOURCE_NAME=user/password@//primaryhost:1521,standbyhost:1521/service?connect_timeout=5&transport_connect_timeout=3&retry_count=3
# 19c client for ASM instance connection (requires SYSDBA)
export DATA_SOURCE_NAME=user/password@//primaryhost:1521,standbyhost:1521/+ASM?as=sysdba
# Then run the exporter
/path/to/binary/oracle-db-monitoring-exporter --log.level error --web.listen-address 0.0.0.0:9161
```

### Security and Other

The exporters are built on the Spring Boot framework and thereby inherit all of the capabilities present there, including

Enabling HTTPS: https://docs.spring.io/spring-cloud-skipper/docs/1.0.0.BUILD-SNAPSHOT/reference/html/configuration-security-enabling-https.html

Basic Auth: https://docs.spring.io/spring-security/reference/servlet/authentication/passwords/basic.html

OAuth2 https://spring.io/guides/tutorials/spring-boot-oauth2/

The reader is referred to this material to configure security and other aspects as appropriate.

### Usage

```bash
Usage of oracle-db-monitoring-exporter:
  --log.format value
        If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.
  --log.level value
        Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal].
  --custom.metrics string
        File that may contain various custom metrics in a TOML file.
  --default.metrics string
        Default TOML file metrics.
  --web.listen-address string
        Address to listen on for web interface and telemetry. (default ":9161")
  --web.telemetry-path string
        Path under which to expose metrics. (default "/metrics")
  --database.maxIdleConns string
        Number of maximum idle connections in the connection pool. (default "0")
  --database.maxOpenConns string
        Number of maximum open connections in the connection pool. (default "10")
  --web.secured-metrics  boolean
        Expose metrics using https server. (default "false")
  --web.ssl-server-cert string
        Path to the PEM encoded certificate file.
  --web.ssl-server-key string
        Path to the PEM encoded key file.
```

#### Default metrics

This exporter comes with a set of default metrics defined in **default-metrics.toml**. You can modify this file or
provide a different one using ``default.metrics`` option.

#### Custom metrics

> NOTE: Do not put a `;` at the end of your SQL queries as this will **NOT** work.

This exporter does not have the metrics you want? You can provide new one using TOML file. To specify this file to the
exporter, you can:

- Use ``--custom.metrics`` flag followed by the TOML file
- Export CUSTOM_METRICS variable environment (``export CUSTOM_METRICS=my-custom-metrics.toml``)

This file must contain the following elements:

- One or several metric section (``[[metric]]``)
- For each section a context, a request and a map between a field of your request and a comment.

Here's a simple example:

```toml
[[metric]]
context = "test"
request = "SELECT 1 as value_1, 2 as value_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }
```

This file produce the following entries in the exporter:

```text
# HELP oracledb_test_value_1 Simple example returning always 1.
# TYPE oracledb_test_value_1 gauge
oracledb_test_value_1 1
# HELP oracledb_test_value_2 Same but returning always 2.
# TYPE oracledb_test_value_2 gauge
oracledb_test_value_2 2
```

You can also provide labels using labels field. Here's an example providing two metrics, with and without labels:

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

This TOML file produce the following result:

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

#### Environment Variables

- `TNS_ENTRY`: Name of the entry to use (`database` in the example file above)
- `TNS_ADMIN`: Path you choose for the tns admin folder (`/path/to/tns_admin` in the example file above)
- `DATA_SOURCE_NAME`: Datasource pointing to the `TNS_ENTRY` (`user/password@database` in the example file above)
