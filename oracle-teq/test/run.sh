#!/bin/bash
## Copyright (c) 2021 Oracle and/or its affiliates.
## Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

dbusername=<"DB USERNAME">
dbpassword=<"DB PASSWORD">
dbservice=<"PDB1">
export PRJ_HOME=<"oracle-db-appdev-monitoring-dir">
export DATA_SOURCE_NAME="${dbusername}/${dbpassword}@${dbservice}"
export TNS_ADMIN="${PRJ_HOME}/oracle-teq/TNS_ADMIN"
export DEFAULT_METRICS="${PRJ_HOME}/oracle-teq/metrics/teq-default-metrics.toml"

java -jar ${PRJ_HOME}/target/observability-exporter-0.1.0.jar
