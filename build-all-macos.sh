#!/bin/bash

# This script builds release artifacts for the Oracle Database Metrics Exporter.
# You must have a working docker socket, and docker or aliased docker command.
# It is designed to run on MacOS aarch64, creating the darwin-arm64 on the local host.
# Artifacts for linux-arm64, linux-amd64 are built in containers.

# The following artifacts are created on a successful build in the 'dist' directory for the selected database driver target (godror or goora):
# - linux/arm64 and linux/amd64 container images
# - linux/arm64 and linux/amd64 binary tarballs for glibc 2.28 built on OL8
# - linux/arm64 and linux/amd64 binary tarballs built on the latest Ubuntu distribution
# - darwin-arm64 binary tarball

# Example usage:
# ./build-all-macos.sh 2.2.0 godror

USAGE="Usage: $0 [-v VERSION] [-t TARGET] [-cmuo]"

while getopts "v:t:cmuo" opt; do
  case ${opt} in
    v ) VERSION=$OPTARG;; # Exporter version
    t ) TARGET=$OPTARG;;  # Target database driver, may be "godror" or "goora"
    c ) BUILD_CONTAINERS=true;; # Build exporter containers
    m ) BUILD_DARWIN=true;; # Build darwin/macos binary
    u ) BUILD_UBUNTU=true;; # Build binaries on latest Ubuntu
    o ) BUILD_OL8=true;;    # Build binaries on OL8
    \? ) echo $USAGE; exit 1;;
  esac
done

if [[ -z "$VERSION" ]] || [[ -z "$TARGET" ]]; then
  echo $USAGE
  exit 1
fi

OL_IMAGE="oraclelinux:8"
BASE_IMAGE="ghcr.io/oracle/oraclelinux:8-slim"
UBUNTU_IMAGE="ubuntu:24.04"
OL8_GLIBC_VERSION="2.28"
GO_VERSION="1.24.9"

if [[ "${TARGET}" == "goora" ]]; then
  TAGS="goora"
  CGO_ENABLED=0
else
  TAGS="godror"
  CGO_ENABLED=1
fi

build_darwin_local() {
  echo "Build dawrin-arm64"
  make go-build
  echo "Built for darwin-arm64"
}

build_ol_platform() {
  build_ol "$1"
  rename_glibc "$1"
}

build_ol() {
  local platform="$1"
  local container="build-${platform}"
  local image_artifact="exporter-${platform}"
  local image_tar=${image_artifact}.tar
  local filename="oracledb_exporter-${VERSION}.linux-${platform}.tar.gz"

  if [[ -n "$BUILD_CONTAINERS" ]]; then
    echo "Starting $OL_IMAGE-${platform} build container"
    docker build --platform "linux/${platform}" --target=exporter-$TARGET -t $image_artifact \
        --build-arg GO_VERSION=$GO_VERSION --build-arg TAGS=$TAGS --build-arg CGO_ENABLED=$CGO_ENABLED \
        --build-arg BASE_IMAGE=$BASE_IMAGE --build-arg GOARCH=$platform --build-arg GOOS=linux --build-arg VERSION=$VERSION .
  fi

  if [[ -n "BUILD_OL8" ]]; then
    docker run -d --privileged --platform "linux/${platform}" --name "${container}" "${OL_IMAGE}" tail -f /dev/null
    docker exec "${container}" bash -c "dnf install -y wget git make gcc && \
                                      wget -q https://go.dev/dl/go${GO_VERSION}.linux-${platform}.tar.gz && \
                                      rm -rf /usr/local/go && \
                                      tar -C /usr/local -xzf go${GO_VERSION}.linux-${platform}.tar.gz && \
                                      export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin && \
                                      git clone --depth 1 https://github.com/oracle/oracle-db-appdev-monitoring.git && \
                                      cd oracle-db-appdev-monitoring && \
                                      make go-build TAGS=$TAGS CGO_ENABLED=$CGO_ENABLED"

    docker cp "$container:/oracle-db-appdev-monitoring/dist/$filename" dist

    echo "Build complete for $OL_IMAGE-${platform}"
    docker stop "$container"
    docker rm "$container"
  fi
}

build_ubuntu() {
  local container="ubuntu-build"
  docker run -d --platform "linux/amd64" --name "${container}" "${UBUNTU_IMAGE}" tail -f /dev/null
  docker exec "${container}" bash -c "apt-get update -y && \
                                      apt-get -y install podman qemu-user-static golang gcc-aarch64-linux-gnu git make && \
                                      git clone --depth 1 https://github.com/oracle/oracle-db-appdev-monitoring.git && \
                                      cd oracle-db-appdev-monitoring && \
                                      make go-build-linux-amd64 TAGS=$TAGS CGO_ENABLED=$CGO_ENABLED && \
                                      make go-build-linux-gcc-arm64 TAGS=$TAGS CGO_ENABLED=$CGO_ENABLED"


  docker cp "$container:/oracle-db-appdev-monitoring/dist/oracledb_exporter-${VERSION}.linux-amd64.tar.gz" dist
  docker cp "$container:/oracle-db-appdev-monitoring/dist/oracledb_exporter-${VERSION}.linux-arm64.tar.gz" dist

  docker stop "$container"
  docker rm "$container"
}

rename_glibc() {
  local platform="$1"

  local f1="oracledb_exporter-${VERSION}.linux-${platform}.tar.gz"
  local f2="oracledb_exporter-${VERSION}.linux-${platform}-glibc-${OL8_GLIBC_VERSION}.tar.gz"

  mv "out/$f1" "out/$f2" 2>/dev/null
}

# clean dist directory before build
rm -r dist/* 2>/dev/null

# Create darwin-arm64 artifacts on local host
if [[ -n "$BUILD_DARWIN" ]]; then
  build_darwin_local
fi
# Create OL8 linux artifacts and containers for glibc 2.28
# OL8 Linux artifacts are built on OL8 containers
build_ol_platform "arm64"
build_ol_platform "amd64"


if [[ -n "$BUILD_UBUNTU" ]]; then
  # Create Linux artifacts and containers
  build_ubuntu
fi

