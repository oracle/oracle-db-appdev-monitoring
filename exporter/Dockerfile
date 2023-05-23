#
#
# Copyright (c) 2021 Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
#
#

FROM oracle-db-monitoring-exporter:0.3.0-oraclelinux

COPY default-metrics.toml /opt/
COPY auth_config.yml /
COPY localhost.crt /
COPY localhost.key /

ENTRYPOINT ["/oracledb_exporter", "--default.metrics", "/opt/default-metrics.toml", "--auth.config", "/auth_config.yml", \
"--web.secured-metrics", "--web.ssl-server-cert", "/localhost.crt", "--web.ssl-server-key", "/localhost.key","--query.timeout", "60"]
