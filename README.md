# Unified App Dev Monitoring with Oracle Database

This distribution contains scripts and code for exporting metrics and logs from the Oracle Database, to provide converged app-dev monitoring for data-centric applications. Metrics from the application layer, Kubernetes, and Oracle Database will be combined to provide unified observability to developers. The project uses Prometheus for metrics and Loki for logs, and uses Grafana as the single pane-of-glass dashboard.

v1 (preview) - contains export of key database metrics to Prometheus and suggested Grafana dashboard

The following metrics are exposed currently by default.

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

## Table of Contents

- [Unified App Dev Monitoring with Oracle Database](#unified-app-dev-monitoring-with-oracle-database)
  - [Table of Contents](#table-of-contents)
  - [Directory Structure](#directory-structure)
  - [Prerequisite Components Setup with Docker Swarm](#prerequisite-components-setup-with-docker-swarm)
    - [Dockerfile and Images for Docker Swarm](#dockerfile-and-images-for-docker-swarm)
      - [Oracle Database Docker Image Building](#oracle-database-docker-image-building)
      - [Oracle Database Exporter Docker Image Building](#oracle-database-exporter-docker-image-building)
      - [Prometheus Docker Image Building](#prometheus-docker-image-building)
      - [Grafana Docker Image Building](#grafana-docker-image-building)
    - [Docker Compose with Docker Swarm](#docker-compose-with-docker-swarm)
      - [Oracle Database Part in Compose File](#oracle-database-part-in-compose-file)
      - [Exporter Part in Compose File](#exporter-part-in-compose-file)
      - [Docker Volume in Compose File](#docker-volume-in-compose-file)
      - [Prometheus Part in Compose File](#prometheus-part-in-compose-file)
      - [Grafana Part in Compose File](#grafana-part-in-compose-file)
  - [Monitor Startup and Components Modification](#monitor-startup-and-components-modification)
    - [Startup and Run](#startup-and-run)
      - [i. Start the Monitor](#i-start-the-monitor)
      - [ii. View Logging](#ii-view-logging)
      - [iii. Stop/Remove the Monitor Program](#iii-stopremove-the-monitor-program)
    - [Exporter Metrics modification and refresh](#exporter-metrics-modification-and-refresh)
    - [Prometheus storage/alert rule modification and refresh](#prometheus-storagealert-rule-modification-and-refresh)
    - [Grafana Setup and Refresh](#grafana-setup-and-refresh)
  - [Oracle Database Monitoring Exporter](#oracle-database-monitoring-exporter)
    - [Description](#description)
    - [Installation](#installation)
    - [Running](#running)
    - [Usage](#usage)
      - [Default metrics](#default-metrics)
      - [Custom metrics](#custom-metrics)
      - [Customize metrics in a docker image](#customize-metrics-in-a-docker-image)
      - [Using a multiple host data source name](#using-a-multiple-host-data-source-name)
      - [Files & Folder](#files--folder)
      - [Environment Variables](#environment-variables)
      - [TLS connection to database](#tls-connection-to-database)
    - [FAQ/Troubleshooting](#faqtroubleshooting)
      - [Unable to convert current value to float (metric=par,metri...in.go:285](#unable-to-convert-current-value-to-float-metricparmetriingo285)
      - [Error scraping for wait_time](#error-scraping-for-wait_time)
      - [An Oracle instance generates trace files](#an-oracle-instance-generates-trace-files)
  - [Data Storage](#data-storage)

## Directory Structure

```text
.
├── Makefile                          
├── docker-compose.yml                # aggregate all services
├── README.md                         # this!
│                               
├── docker_vol/                       
│   ├── graf_app_vol/        
│   │   ├── dashboard_concurrency.json
│   │   ├── dashboard_io.json      
│   │   ├── dashboard_query.json
│   │   └── dashboard_sys.json
│   │
│   └── prom_app_vol/                        
│       ├── myrules.yml               # rules of prometheus metrics and alerts
│       ├── config.yml                # connection configuration
│       └── web.yml                   # authentication configuration
│
│
├── oracledb/                         # local Oracle Database(19c) container
│   ├── Dockerfile
│   └── oracledb_entrypoint.sh        # docker secret setup scripts
│
│
├── oracle-db-monitoring-exporter/    # customized basic exporter program
│
│
├── exporter/                         # query and format metrics
│   ├── Dockerfile
│   ├── auth_config.yml               # http auth config of the exporter        
│   └── default-metrics.toml          # queries to collect metrics
│
│
├── prometheus/                       # time-series metrics storage
│   ├── Dockerfile              
│   └── prom_entrypoint.sh            # docker secret setup scripts
│
│
└── grafana/                          # monitor dashboard
    ├── Dockerfile              
    ├── dashboards/              
    │   └── all.yml                   # config of dashboard
    │
    └── datasources
        └── all.yml                   # specify prometheus as the datasource
```

---

## Prerequisite Components Setup with Docker Swarm  

### Dockerfile and Images for Docker Swarm

In order to protect user's sensitive config info and data, a Docker Secret is used which requires Docker Swarm mode.

```sh
docker swarm init
# use `docker info` to check status of swarm mode
```

> For more details about docker swarm, please visit [docker swarm init official documentation](https://docs.docker.com/engine/reference/commandline/swarm_init/).

Each component in [Docker Swarm](https://docs.docker.com/engine/swarm/) mode is a [Docker Service](https://docs.docker.com/engine/reference/commandline/service/). A group of docker services is called [Docker Stack](https://docs.docker.com/engine/reference/commandline/stack/).

Hence, we are going to use following command to start the monitor.

``` sh
docker stack deploy --compose-file {yaml_compose_file} {stack_name}
```

This is different from `docker-compose` which can build the image during setup as Docker Swarm requires a pre-built image for each service(container).

#### Oracle Database Docker Image Building

- Files involved
  - `./oracledb/Dockerfile`
  - `./oracledb/oracledb_entrypoint.sh`

```sh
cd exporter
docker build --tag {oracledb_image_name} .        
# or
docker build --tag {oracledb_image_name}:{image_tag} .
# examples
docker build --tag oracledb_monitor_oracledb .
docker build --tag oracledb_monitor_oracledb:1.0 .
```

> For more details about docker build, please visit the [official documentation](https://docs.docker.com/engine/reference/commandline/build/)

#### Oracle Database Exporter Docker Image Building

An additional http authentication feature is provided to enhance connection security, therefore, it is necessary to build a basic exporter image from `oracle-db-monitoring-exporter` and then a customized image with configuration files.

- Files involved:
  - `./oracle-db-monitoring-exporter/*`
  - `./exporter/Dockerfile`
  - `./exporter/auth_config.yml`
  - `./exporter/default-metrics.toml`
  - `./exporter/localhost.cert` (you need to create your own version)
  - `./exporter/localhost.key` (you need to create your own version)

a. Base Image

This is a required step.

```sh
# Base Image

cd oracle-db-monitoring-exporter
make oraclelinux-image
# This will build three base images and we are going to use either 
# "oracle-db-monitoring-exporter:0.3.0-oraclelinux" or "oracle-db-monitoring-exporter:oraclelinux"
```

b. Before creating the customized image, it is necessary to setup the authentication username and password for the exporter and encrypt it with a docker secret. Then, specify the docker secret names in `exporter/auth_config.yml`

```sh
echo "{exp_auth_username}" | docker secret create {secret_name} -
echo "{exp_auth_password}" | docker secret create {secret_name} -

# examples
echo "mntmgr" | docker secret create auth.username -
echo "P@55w0rd" | docker secret create auth.password -  
```

```yaml
# auth_config.yml
username: auth.username
password: auth.password
```

c. Ensure metric queries are finished and saved in `exporter/default-metrics.toml`.

d. Generate ssl key and ssl certificate for https transportation.

```sh
cd exporter
openssl req \
  -x509 \
  -newkey rsa:4096 \
  -nodes \
  -keyout localhost.key \
  -out localhost.crt
```

Make sure `localhost.key` and `localhost.crt` are under `./exporter/`

*If you need to change file names of cert and key, don't forget to modify the `Dockerfile`.*

e. Final Customized Image

```sh
# Customized Image

cd exporter
docker build --tag {image_name}:{tag_name} .
# example
docker build --tag oracledb_monitor_exporter:1.0 .
```

#### Prometheus Docker Image Building

- Files involved:
  - `./docker_vol/prom_app_vol/config.yml`
  - `./docker_vol/prom_app_vol/myrules.yml`
  - `./docker_vol/prom_app_vol/web.yml`
  - `./prometheus/Dockerfile`
  - `./prometheus/prom_entrypoint.sh`
  - `./prometheus/localhost.cert` (you need to create your own version)
  - `./prometheus/localhost.key` (you need to create your own version)

a. Generate ssl key and ssl certificate for https transportation.

```sh
cd exporter
openssl req \
  -x509 \
  -newkey rsa:4096 \
  -nodes \
  -keyout localhost.key \
  -out localhost.crt
```

Make sure your `localhost.key` and `localhost.crt` are under `./prometheus/`

*If you need to change file names of cert and key, don't forget to modify the `Dockerfile` and `web.yml`.*

b. Build Prometheus image

```sh
cd prometheus
docker build --tag {image_name}:{tag_name} .
# example
docker build --tag oracledb_monitor_prometheus:1.0 .
```

c. Setup http authentication username and password for Prometheus in `./docker_vol/prom_app_vol/web.yml` and Docker Secret.

The password required to connect to Prometheus should be hashed with [bcrypt](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md#about-bcrypt). Use `htpasswd` command to hash the password with bcrypt.

```sh
htpasswd -nBC 10 “” | tr -d ‘:\n’
```

Copy and paste the result, and create such docker secret. For example, the bcrypt hashing of `test` is `$2b$12$hNf2lSsxfm0.i4a.1kVpSOVyBCfIB51VRjgBUyv6kdnyTlgWj81Ay`.

```sh
echo "{prom_auth_password}" | docker secret create prom.auth.pwd -
# example
echo "\$2b\$12\$hNf2lSsxfm0.i4a.1kVpSOVyBCfIB51VRjgBUyv6kdnyTlgWj81Ay" | docker secret create prom.auth.pwd -
# don't forget add a '\' before each '$'
# you can also create a docker secret with a file. visit the documentation.
```

***Here the secret name is required to be 'prom.auth.pwd'.***

```yaml
# web.yml
basic_auth_users:
  {prom_auth_username}: {docker_secret_name_of_auth_pwd}
  # example:
  mntmgr: prom.auth.pwd
```

#### Grafana Docker Image Building

- Files involved:
  - `./docker_vol/prom_app_vol/config.yml`
  - `./docker_vol/prom_app_vol/myrules.yml`
  - `./docker_vol/prom_app_vol/web.yml`
  - `./prometheus/Dockerfile`
  - `./prometheus/prom_entrypoint.sh`
  - `./prometheus/localhost.cert` (you need to create your own version)
  - `./prometheus/localhost.key` (you need to create your own version)

a. Generate ssl key and ssl certificate for https transportation.

```sh
cd exporter
openssl req \
  -x509 \
  -newkey rsa:4096 \
  -nodes \
  -keyout localhost.key \
  -out localhost.crt
```

Make sure your `localhost.key` and `localhost.crt` are under `./grafana/`

*If you need to change file names of cert and key, don't forget to modify the `Dockerfile` and `grafana.ini`.*

b. For Grafana, you can setup the connection to Prometheus in `./grafana/datasources/all.yml`, setup config of dashboards in `./grafana/dashboards/all.yml`, while all of the provision dashboards are in `./docker_vol/graf_app_vol/*.json`.

You should setup connection and configuration of Grafana before building the image, but you can modify dashboard raw codes after startup.

```sh
cd grafana
docker build --tag {image_name}:{tag_name} .
# example
docker build --tag oracledb_monitor_grafana:1.0 .
```

### Docker Compose with Docker Swarm

To enable the usage of compose file in docker swarm command line, we need the version of `docker-compose.yml` to be at least 3.1.

```yaml
# docker-compose.yml
version: 3.1          # We have it by default. Don't delete it in your customization.
```

There is a default `docker-compose.yml`, but it is still necessary to setup docker secret in your environment.

#### Oracle Database Part in Compose File

```yaml
services:
  oracledb:
    image: {db_image_name:image_tab}              # oracledb_monitor_oracledb:1.0
    container_name: 'oracledb'
    environment:
      ORACLE_SID: ORCLCDB
      ORACLE_PWD: DOCKER_SECRET@{pwd_secret_name} # DOCKER_SECRET@oracle.pwd
    secrets:
      - {pwd_secret_name}                         # oracle.pwd
    ports:
      - '1521:1521'
      - '8080:8080'
    tty: true

secrets:
  {pwd_secret_name}:                             # oracle.pwd:
    external: true
```

You need to create your password to DBA with Docker Secret.

```sh
echo "{sysdba_pwd}" | docker secret create {secret_name} -
# example
echo "P@55w0rd" | docker secret create oracle.pwd -  
```

> For more details about docker secret in compose file, please visit the [official documentation](https://docs.docker.com/engine/swarm/secrets/#use-secrets-in-compose).

#### Exporter Part in Compose File

```yaml
services:
  exporter:
    image: {exporter_image_name:image_tab}       # oracledb_monitor_exporter:1.0
    container_name: 'exporter'
    environment:
      DATA_SOURCE_NAME: {dsn_secret_name}        # data.source.name
    secrets:
      - {dsn_secret_name}        # data.source.name
      - {exp_auth_username}  # auth.username
      - {exp_auth_password}   # auth.password
    depends_on:
      - oracledb
    ports:
      - '9161:9161'

secrets:
  {dsn_secret_name}:                # data.source.name
    external: true
  {exp_auth_username}:          # auth.username
    external: true
  {exp_auth_password}:           # auth.password
    external: true
```

`{exp_auth_username}` and `{exp_auth_password}` are the ones we've setup in the [previous step](#oracle-database-exporter-docker-image-building).

You need to setup your database connection string, auth username and password of the exporter with Docker Secret.

For the Data Connection String, we strongly recommend not using sysdba, and instead creating your own common cdb user.

```sh
# After the creation and initialization of your Oracle Database
# in the shell of your database system
# for container, to login to shell
docker exec -it --user oracle {container_id} /bin/bash

sqlplus sys/{sysdba_pwd} as sysdba
```

```sql
DROP USER c##mntmgr CASCADE;    -- a prefix of c## is required
CREATE USER c##mntmgr IDENTIFIED BY test CONTAINER=ALL;
GRANT CREATE SESSION TO c##mntmgr;
GRANT select_catalog_role TO c##mntmgr;
GRANT select any dictionary TO c##mntmgr;
```

So the DSN of `c##mntmgr` to your Oracle Database is `c##mntmgr:test@oracledb/ORCLCDB`. Encrypt it with Docker Secret.

```sh
echo "c##mntmgr:test@oracledb/ORCLCDB" | docker secret create data.source.name -
```

> For more details about Oracle Easy Connect Naming, please visit [official documentation](https://docs.oracle.com/en/database/oracle/oracle-database/18/ntcli/specifying-a-connection-by-using-the-easy-connect-naming-method.html#GUID-1035ABB3-5ADE-4697-A5F8-28F9F79A7504)*

#### Docker Volume in Compose File

Before Prometheus and Grafana Part, we need to set the docker volume.

To use configuration files and dashboard of Prometheus and Grafana in `./docker_vol`, please setup volumes to Prometheus and Grafana containers.

```yaml
services:
  prometheus:
    volumes:
      - {directory_to_prom_app_vol}:/etc/prometheus/prometheus_vol
      # for example
      # ?/docker_vol/prom_app_vol:/etc/prometheus/prometheus_vol
```

#### Prometheus Part in Compose File

```yaml
# docker-compose.yml
prometheus:
  image: oracledb_monitor_prometheus:1.0
  container_name: 'prometheus'
  secrets:
    - prom.auth.pwd
    - auth.username  # exporter auth username
    - auth.password  # exporter auth password
  depends_on:
    - exporter
  ports:
    - '9090:9090'
    - '9093:9093'
  volumes:
    - ./docker_vol/prom_app_vol:/etc/prometheus/prometheus_vol
  tty: true

secrets:
  prom.auth.pwd:
    external: true
```

```yaml
# config.yml in ./docker_vol/prom_app_vol/
# this file is for exporter connection
global:
  scrape_interval:     30s
  scrape_timeout:      30s
  evaluation_interval: 30s

scrape_configs:
  - job_name: 'TEQ Monitor'
    static_configs:
      - targets: ['exporter:9161']
    basic_auth:
      username: auth.username      # docker secret name of exporter auth username
      password: auth.password      # docker secret name of exporter auth password

rule_files:
  - "/etc/prometheus/prometheus_vol/myrules.yml"    # your prom rules
```

#### Grafana Part in Compose File

```yaml
# docker-compose.yml
grafana:
    image: oracledb-monitor_graf
    container_name: 'grafana'
    depends_on:
      - prometheus
    ports:
      - '3000:3000'
    volumes:
      - ./docker_vol/graf_app_vol:/var/lib/grafana/grafana_vol
```

---

## Monitor Startup and Components Modification

Setup all [configurations prerequisites](#prerequisite-link).

### Startup and Run

#### i. Start the Monitor

```sh
docker stack deploy --compose-file docker-compose.yml {stack_name}

# for example
docker stack deploy --compose-file docker-compose.yml oracledb-monitor

# or run `make deploy`
# check Makefile to edit your own commands
```

The first time you build and start the Oracle Database container, it will take about 15 to 20 minutes for Oracle Database to get ready. Create your general user when it done.

Then, go to the [Grafana Dashboard](https://localhost:3000). By default, username: admin, password: admin

> If using Chrome, it may show "Your connection is not private" and "NET::ERR_CERT_INVALID", and prevent you from visiting the Grafana board. Please use other browser. You will meet the same problem during visiting local [Prometheus Dashboard](https://localhost:9090) and [exporter metrics](https://localhost:9161/metrics) with https protocol. This problem is due to that we are using self-signed certificates which Chrome does not recognize.

***To enable your Grafana to connect to your Prometheus database, when you login to Grafana dashboard, go to `Configuration` -> `Data Sources` -> `Prometheus`(data connection). Then A) enable the `Basic auth`, `Skip TLS Verify` and `With CA Cert` under `Auth` section, B) type in the Prometheus auth username and password you just setup, and then finally C) save and test.***

You can also visit the [Prometheus Dashboard](https://localhost:9090) and [exporter metrics](https://localhost:9161/metrics) to track.

#### ii. View Logging

```sh
# display all docker services
docker service ls
# show logs of one service
docker service logs --follow {docker_service_name} --raw
# for example
docker service logs --follow oracledb-monitor_oracledb --raw
docker service logs --follow oracledb-monitor_exporter --raw

# or run `make log-oracledb`
# check Makefile to edit your own commands
```

> For more details about docker service logging, please visit the [official documentation](https://docs.docker.com/engine/reference/commandline/service_logs/)

#### iii. Stop/Remove the Monitor Program

```sh
docker stack rm {stack_name}
# example
docker stack rm oracledb-monitor

# or run `make down`
# This will both stop and remove all four services in this docker stack.
```

*Be careful when you run this, since it will clean all local files in the containers (files in volumes won't be deleted), so make sure you've backup/relocated necessary files before running this.*

### Exporter Metrics modification and refresh

You can modify or add metrics by editing `exporter\default-metrics.toml`

After updating it, it is necessary to rebuild the image and redeploy the exporter service.
*Be careful when you rebuild the image and redeploy the service, since it will remove the old container and start a new one, so make sure you've backup/relocated necessary files.*

```sh
# if no updates were made to the image and only changes docker-compose of exporter service were made,
# there is no need to re-build the image, simply run:
docker stack deploy --compose-file docker-compose.yml oracledb-monitor

# if metric files were modified
# rebuild and restart the exporter service with a new image
cd {Dockerfile_path}
docker build --tag {image_title_tag} .
docker service update {service_name} --image {image_title_tag}
# for example
docker build --tag oracledb_monitor_exporter:1.1 .
docker service update oracledb-monitor_exporter --image oracledb_monitor_exporter:1.1

# if you change your compose file, just re-run the deploy command
# it will restart the service you changed
docker stack deploy --compose-file docker-compose.yml oracledb-monitor

# get service name
docker service ls
```

> For more details about docker service, please visit the [Official Documentation](https://docs.docker.com/engine/reference/commandline/service/)
> For more details about exporter and metrics editing/configuring, please check [Oracle Database Monitoring Exporter](#db-mnt-exporter) part.

### Prometheus storage/alert rule modification and refresh

Prometheus configuration can be modified in `docker_vol\prom_app_vol\config.yml`, and add recording and alerting rules can be modified in `docker_vol\prom_app_vol\myrules.yml`.

After updating either of them, it is necessary to enter the container and restart the Prometheus process.

```sh
# get container name
docker ps

# enter bash shell of the container
docker exec -it --user root {container_name/container_id} /bin/bash
# for example
docker exec -it --user root oracledb-monitor_prometheus /bin/bash

# restart the prometheus process without killing it
kill -HUP 1
```

> For more details about Prometheus config and rule files, please visit [Prometheus Configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/)

### Grafana Setup and Refresh

You can add or modify Grafana panels and add dashboards on the [Grafana webpage](https://localhost:3000). However, although you can save cache, you can not save the dashboard to the source file. Instead, you can go to Setting of the dashboard and copy the JSON Model to replace the original json file(`docker_vol\graf_app_vol\{dashboard}.json`).

> For more details about provision and config of Grafana, please visit [Grafana Lab](https://grafana.com/docs/grafana/latest/administration/provisioning/).

## Oracle Database Monitoring Exporter

### Description

A [Prometheus](https://prometheus.io/) exporter for Oracle Database.

### Installation

We currently only support `oraclelinux` container version.

```bash
cd oracle-db-monitoring-exporter
make oraclelinux-image
```

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

You can find [here](./custom-metrics-example/custom-metrics.toml) a working example of custom metrics for slow queries, big queries and top 100 tables.

#### Customize metrics in a docker image

If you run the exporter as a docker image and want to customize the metrics, you can use the following example:

```Dockerfile
FROM oracle-db-monitoring-exporter:oraclelinux

COPY custom-metrics.toml /

ENTRYPOINT ["/oracledb_exporter", "--custom.metrics", "/custom-metrics.toml"]
```

#### Using a multiple host data source name

> NOTE: This has been tested with v0.2.6a and will most probably work on versions above.
> NOTE: While `user/password@//database1.example.com:1521,database3.example.com:1521/DBPRIM` works with SQLPlus, it doesn't seem to work with `oracledb-exporter` v0.2.6a.

In some cases, one might want to scrape metrics from the currently available database when having a active-passive replication setup.

This will try to connect to any available database to scrape for the metrics. With some replication options, the secondary database is not available when replicating. This allows the scraper to automatically fall back in case of the primary one failing.

This example allows to achieve this:

#### Files & Folder

- tns_admin folder: `/path/to/tns_admin`
- tnsnames.ora file: `/path/to/tns_admin/tnsnames.ora`

Example of a tnsnames.ora file:

```ora
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

#### Environment Variables

- `TNS_ENTRY`: Name of the entry to use (`database` in the example file above)
- `TNS_ADMIN`: Path you choose for the tns admin folder (`/path/to/tns_admin` in the example file above)
- `DATA_SOURCE_NAME`: Datasource pointing to the `TNS_ENTRY` (`user/password@database` in the example file above)

#### TLS connection to database

First, set the following variables:

```bash
export WALLET_PATH=/wallet/path/to/use
export TNS_ENTRY=tns_entry
export DB_USERNAME=db_username
export TNS_ADMIN=/tns/admin/path/to/use
```

Create the wallet and set the credential:

```bash
mkstore -wrl $WALLET_PATH -create
mkstore -wrl $WALLET_PATH -createCredential $TNS_ENTRY $DB_USERNAME
```

Then, update sqlnet.ora:

```bash
echo "
WALLET_LOCATION = (SOURCE = (METHOD = FILE) (METHOD_DATA = (DIRECTORY = $WALLET_PATH )))
SQLNET.WALLET_OVERRIDE = TRUE
SSL_CLIENT_AUTHENTICATION = FALSE
" >> $TNS_ADMIN/sqlnet.ora
```

To use the wallet, use the wallet_location parameter. You may need to disable ssl verification with the
ssl_server_dn_match parameter.

Here a complete example of string connection:

```text
DATA_SOURCE_NAME=username/password@tcps://dbhost:port/service?
ssl_server_dn_match=false&wallet_location=wallet_path
```

### FAQ/Troubleshooting

#### Unable to convert current value to float (metric=par,metri...in.go:285

Oracle is trying to send a value that we cannot convert to float. This could be anything like 'UNLIMITED' or 'UNDEFINED' or 'WHATEVER'.

In this case, you must handle this problem by testing it in the SQL request. Here an example available in default metrics:

```toml
[[metric]]
context = "resource"
labels = [ "resource_name" ]
metricsdesc = { current_utilization= "Generic counter metric from v$resource_limit view in Oracle (current value).", limit_value="Generic counter metric from v$resource_limit view in Oracle (UNLIMITED: -1)." }
request="SELECT resource_name,current_utilization,CASE WHEN TRIM(limit_value) LIKE 'UNLIMITED' THEN '-1' ELSE TRIM(limit_value) END as limit_value FROM v$resource_limit"
```

If the value of limit_value is 'UNLIMITED', the request send back the value -1.

You can increase the log level (`--log.level debug`) in order to get the statement generating this error.

#### Error scraping for wait_time

If you experience an error `Error scraping for wait_time: sql: Scan error on column index 1: converting driver.Value type string (",01") to a float64: invalid syntax source="main.go:144"` you may need to set the NLS_LANG variable.

```bash
export NLS_LANG=AMERICAN_AMERICA.WE8ISO8859P1
export DATA_SOURCE_NAME=system/oracle@myhost
/path/to/binary --log.level error --web.listen-address 9161
```

If using Docker, set the same variable using the -e flag.

#### An Oracle instance generates trace files

An Oracle instance will generally generate a number of trace files alongside its alert log file. One trace file per scraping event. The trace file contains the following lines

```text
...
*** MODULE NAME:(prometheus_oracle_exporter-amd64@hostname)
...
kgxgncin: clsssinit: CLSS init failed with status 3
kgxgncin: clsssinit: return status 3 (0 SKGXN not av) from CLSS
```

The root cause is Oracle's reaction of querying ASM-related views without ASM used. The current workaround proposed is to setup a regular task to cleanup these trace files from the filesystem, as example

```bash
find $ORACLE_BASE/diag/rdbms -name '*.tr[cm]' -mtime +14 -delete
```

## Data Storage

By default the retention of Prometheus is configured to 15 days. On average, Prometheus uses only around 1-2 bytes per sample. Thus, to plan the capacity of a Prometheus server, you can use the rough formula:

```text
needed_disk_space = retention_time_seconds * ingested_samples_per_second * bytes_per_sample
```

Roughly, Oracle Database Monitor System has 100 samples every 1 minute, meaning 1.67 samples per second on average. You could base on your retention time to determine the capacity of the server.
