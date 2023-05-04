# Unified Observability with Oracle Database

This distribution contains scripts and code for exporting metrics, logs, and traces from any Oracle Database to provide converged observability for data-centric applications. 

Metrics from the application layer, Kubernetes, and Oracle Database can be combined to provide unified observability to developers within a single Grafana console. 

All three exporters (metrics, log, and trace) can be configured in the same file and each is explained in the corresponding doc pages:


[Metrics Exporter][Metrics Exporter]

[Log Exporter][Log Exporter]

[Trace Exporter][Trace Exporter]

The old version of the metrics exporter can be found in the [old implementation branch][old implementation branch] and the new metrics exporter implementation is backward compatible such that the same configuration for both database connection and metrics definition can be used.

Users are encouraged to open issues and enhancements requests against this repos (https://github.com/oracle/oracle-db-appdev-monitoring/issues ) and are encouraged to ask any questions.  We will actively work on them as we will the development of the exporters.

### Pull

The image can be pulled via `docker pull container-registry.oracle.com/database/observability-exporter:0.1.0`

### Build

Build without running tests using the following.

`mvn clean package -DskipTests`

Tests use a live database and require `DATA_SOURCE_NAME` environment variable be set (see section on Running) and can be run using the following.

`mvn clean package`

Docker image can be build using the following.

`./build.sh`

Docker image can be pushed to $DOCKER_REGISTRY using the following.

`./push.sh`

### Run

Ensure the environment variable DATA_SOURCE_NAME (and TNS_ADMIN if appropriate) is set correctly before starting.

For Example:

```bash
export DATA_SOURCE_NAME="%USER%/$(dbpassword)@%PDB_NAME%_tp"
```

Kubernetes Secrets, etc. an of course be used to store password.

OCI Vault support for storing/accessing password values is built into exporters and is enabled by simply setting the OCI_REGION and VAULT_SECRET_OCID variables.

For Example:

```bash
export OCI_REGION="us-ashburn-1"
export VAULT_SECRET_OCID="ocid..."
```

The only other required environment variable is DEFAULT_METRICS value which is set to the location of the config file.

For Example:

```bash
export DEFAULT_METRICS="/msdataworkshop/observability/db-metrics-%EXPORTER_NAME%-exporter-metrics.toml"
```

Run using Java:

`java -jar target/observability-exporter-0.1.0.jar`

Run using Docker

`docker container run observability-exporter-0.1.0`

Run within Kubernetes:

See example yaml in examples directory

## Contributing

This project welcomes contributions from the community. Before submitting a pull request, please [review our contribution guide](./CONTRIBUTING.md)

## Security

Please consult the [security guide](./SECURITY.md) for our responsible security vulnerability disclosure process

The exporters are built on the Spring Boot framework and thereby inherit all of the capabilities present there, including

Enabling HTTPS: https://docs.spring.io/spring-cloud-skipper/docs/1.0.0.BUILD-SNAPSHOT/reference/html/configuration-security-enabling-https.html

Basic Auth: https://docs.spring.io/spring-security/reference/servlet/authentication/passwords/basic.html

OAuth2 https://spring.io/guides/tutorials/spring-boot-oauth2/

The reader is referred to this material to configure security and other aspects as appropriate.

## Support for Multiple Databases

This feature can be used in two ways:

1. By accessing the `/scrape` endpoint and passing a `target` parameter that is the full dsn value. 
   For example: `http://localhost:9161/scrape?target`
2. By accessing the `/scrape` endpoint and passing a `name` parameter that is the name of the datasource 
   as defined in the yaml file located at the environment variable `MULTI_DATASOURCE_CONFIG`
    The yaml file takes the following form and an example can be found under examples/multidatasource_config.yaml :
```    
    dataSourceName :
       serviceName :
       userName :
       password :
       TNS_ADMIN :
       # if present, OCI Vault is used for password rather than "password" attribute
       passwordOCID :
       # the following is applicable only if OCI config file is used rather than instance principals authentication (generally only the case in development)
       ociConfigFile :
       ociRegion :
       ociProfile :
```   
The feature is currently only applicable to metrics, not tracing or logging.
The feature currently uses the same global `DEFAULT_METRICS` config for every datasource.

[Metrics Exporter]: Metrics.md
[Log Exporter]: Logs.md
[Trace Exporter]: Tracing.md
[old implementation branch]: https://github.com/oracle/oracle-db-appdev-monitoring/tree/old-go-implementation

## License

Copyright (c) 2021 Oracle and/or its affiliates.

Released under the Universal Permissive License v1.0 as shown at
<https://oss.oracle.com/licenses/upl/>.
