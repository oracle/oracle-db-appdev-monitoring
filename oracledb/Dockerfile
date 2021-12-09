#
#
# Copyright (c) 2021 Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
#
#

FROM container-registry.oracle.com/database/enterprise:latest
COPY oracledb_entrypoint.sh /
ENTRYPOINT ["/bin/bash", "/oracledb_entrypoint.sh"]
