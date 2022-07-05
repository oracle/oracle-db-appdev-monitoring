#!/bin/bash
## Copyright (c) 2021 Oracle and/or its affiliates.
## Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

dbusername="sys as sysdba"
dbpassword="Welcome#10racle"
export PRJ_HOME="/home/pasimoes/Code/Oracle/oracle-db-appdev-monitoring"
export DATA_SOURCE_NAME="${dbusername}/${dbpassword}@PDB1"
export TNS_ADMIN="${PRJ_HOME}/test/TNS_ADMIN"
export DEFAULT_METRICS="${PRJ_HOME}/examples/metrics/teq-metrics.toml"

java -jar ${PRJ_HOME}/target/observability-exporter-0.1.0.jar
