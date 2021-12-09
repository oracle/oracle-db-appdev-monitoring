#
#
# Copyright (c) 2021 Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
#
#

FROM prom/prometheus:v2.30.0

COPY prom_entrypoint.sh /
COPY localhost.crt /
COPY localhost.key /
USER root
RUN ["chmod", "+x", "/prom_entrypoint.sh"]

ENTRYPOINT ["/bin/sh", "/prom_entrypoint.sh"]
