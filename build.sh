#!/bin/bash
## Copyright (c) 2021 Oracle and/or its affiliates.
## Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/


export IMAGE_NAME=observability-exporter
export IMAGE_VERSION=0.1.0

if [ -z "$DOCKER_REGISTRY" ]; then
    echo "DOCKER_REGISTRY not set. Will get it with state_get"
  export DOCKER_REGISTRY=$(state_get DOCKER_REGISTRY)
fi

export IMAGE=${DOCKER_REGISTRY}/${IMAGE_NAME}:${IMAGE_VERSION}

mvn clean package -DskipTests
docker build -t=$IMAGE .

export IS_CREATE_REPOS=$1
if [ -z "IS_CREATE_REPOS" ]; then
    echo "not creating OCIR repos"
else
    echo "creating OCIR repos and setting to public"
    if [ -z "COMPARTMENT_OCID" ]; then
        echo "COMPARTMENT_OCID not set. Will get it with state_get"
        export COMPARTMENT_OCID=$(state_get COMPARTMENT_OCID)
    fi
    if [ -z "RUN_NAME" ]; then
        echo "RUN_NAME not set. Will get it with state_get"
        export RUN_NAME=$(state_get RUN_NAME)
    fi
#    RUN_NAME is randomly generated name from workshop, eg gd4930131
    oci artifacts container repository create --compartment-id "$COMPARTMENT_OCID" --display-name "$RUN_NAME/$IMAGE_NAME" --is-public true
fi

docker push "$IMAGE"
if [  $? -eq 0 ]; then
    docker rmi "$IMAGE"
fi
