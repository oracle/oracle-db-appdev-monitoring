#!/bin/sh
#
# Copyright (c) 2021 Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
#
#

: ${SECRETS_DIR:=/run/secrets}

env_secrets_expand() {
    for env_var in $(printenv | cut -f1 -d"=")
    do
        eval val=\$$env_var
        if secret_name=$(expr match "$val" "DOCKER_SECRET@\([^}]\+\)$"); then
            secret="${SECRETS_DIR}/${secret_name}"
            if [ -f "$secret" ]; then
                val=$(cat "${secret}")
                export "$env_var"="$val"
            fi
        fi
    done
}

env_secrets_expand

/bin/bash /opt/oracle/runOracle.sh
