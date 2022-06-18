# Unified App Dev Monitoring with Oracle Database

This v1 (preview) distribution contains scripts and code for exporting metrics, logs, and traces from any Oracle Database to provide converged observability for data-centric applications. 

Metrics from the application layer, Kubernetes, and Oracle Database can be combined to provide unified observability to developers within a single Grafana console. 

All three exporters (metrics, log, and trace) can be configured in the same file and each is explanined in the corresponding doc pages:

Metrics Exporter

Log Exporter

Trace Exporter

The old version of the metrics exporter can be found in the branch and the new metrics exporter implementation is backward compatible such that the same configuration can be used.

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

Ensure the environment variable DATA_SOURCE_NAME is set correctly before starting.
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

The only other required environment variable is DEFAULT_METRICS value which is set to the location of the config file.

### Security and Other

The exporters are built on the Spring Boot framework and thereby inherit all of the capabilities present there, including

Enabling HTTPS: https://docs.spring.io/spring-cloud-skipper/docs/1.0.0.BUILD-SNAPSHOT/reference/html/configuration-security-enabling-https.html

Basic Auth: https://docs.spring.io/spring-security/reference/servlet/authentication/passwords/basic.html

OAuth2 https://spring.io/guides/tutorials/spring-boot-oauth2/

The reader is referred to this material to configure security and other aspects as appropriate.


#### Environment Variables

- `TNS_ENTRY`: Name of the entry to use (`database` in the example file above)
- `TNS_ADMIN`: Path you choose for the tns admin folder (`/path/to/tns_admin` in the example file above)
- `DATA_SOURCE_NAME`: Datasource pointing to the `TNS_ENTRY` (`user/password@database` in the example file above)
