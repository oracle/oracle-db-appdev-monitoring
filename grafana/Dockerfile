#
#
# Copyright (c) 2021 Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
#
#

FROM grafana/grafana:7.5.5

COPY dashboards/*.yml /etc/grafana/provisioning/dashboards/
COPY datasources/*.yml  /etc/grafana/provisioning/datasources/
COPY grafana.ini /etc/grafana/grafana.ini
COPY localhost.crt /
COPY localhost.key /
# COPY dashboards/*.json /var/lib/grafana/dashboards/
