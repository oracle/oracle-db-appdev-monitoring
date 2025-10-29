#!/bin/bash

VERSION="$1"  # Exporter version
REGISTRY="$2" # Container registry/repository

# Tag the images
docker tag exporter-amd64:latest ${REGISTRY}:${VERSION}-amd64
docker tag exporter-arm64:latest ${REGISTRY}:${VERSION}-arm64

# Push the images
docker push ${REGISTRY}:${VERSION}-amd64
docker push ${REGISTRY}:${VERSION}-arm64

# Create and push the manifest
docker manifest create ${REGISTRY}:${VERSION} ${REGISTRY}:${VERSION}-amd64 ${REGISTRY}:${VERSION}-arm64
docker manifest push ${REGISTRY}:${VERSION}
