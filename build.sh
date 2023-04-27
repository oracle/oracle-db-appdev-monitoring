#!/bin/bash
## Copyright (c) 2021 Oracle and/or its affiliates.
## Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/


export IMAGE_NAME=observability-exporter
export IMAGE_VERSION=0.1.0

if [ -z "$DOCKER_REGISTRY" ]; then
    echo "DOCKER_REGISTRY not set."
    exit
fi

export IMAGE=${DOCKER_REGISTRY}/${IMAGE_NAME}:${IMAGE_VERSION}

mvn clean package -DskipTests
docker build -t=$IMAGE .
docker push $IMAGE
