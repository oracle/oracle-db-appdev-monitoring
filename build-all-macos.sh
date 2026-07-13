#!/bin/bash
set -euo pipefail

# This script builds release artifacts for the Oracle AI Database Metrics Exporter.
# You must have a working docker socket, and docker or aliased docker command.
# It is designed to run on MacOS aarch64, creating the darwin-arm64 on the local host.
# Artifacts for linux-arm64, linux-amd64 are built in containers.

# The following artifacts are created on a successful build in the 'dist' directory for the selected database driver target (godror or goora):
# - linux/arm64 and linux/amd64 container images
# - linux/arm64 and linux/amd64 binary tarballs for glibc 2.28 extracted from the container images
# - linux/arm64 and linux/amd64 binary tarballs built on the latest Ubuntu distribution
# - darwin-arm64 binary tarball

# Example usage:
# ./build-all-macos.sh -v 2.4.2 -t godror -cmu

USAGE="Usage: $0 [-v VERSION] [-t TARGET] [-cmu]"
VERSION=""
TARGET=""
BUILD_CONTAINERS=""
BUILD_DARWIN=""
BUILD_UBUNTU=""
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_NAME="${IMAGE_NAME:-container-registry.oracle.com/database/observability-exporter}"

while getopts "v:t:cmu" opt; do
  case ${opt} in
    v ) VERSION=$OPTARG;; # Exporter version
    t ) TARGET=$OPTARG;;  # Target database driver, may be "godror" or "goora"
    c ) BUILD_CONTAINERS=true;; # Build exporter containers and extract OL8/glibc binary tarballs
    m ) BUILD_DARWIN=true;; # Build darwin/macos binary
    u ) BUILD_UBUNTU=true;; # Build binaries on latest Ubuntu
    \? ) echo "$USAGE"; exit 1;;
  esac
done

if [[ -z "$VERSION" ]] || [[ -z "$TARGET" ]]; then
  echo "$USAGE"
  exit 1
fi

UBUNTU_IMAGE="ubuntu:24.04"
OL8_GLIBC_VERSION="2.28"
IMAGE_ID="${IMAGE_ID:-${IMAGE_NAME}:${VERSION}}"

case "${TARGET}" in
  goora)
    TAGS="goora"
    CGO_ENABLED=0
    DOCKER_TARGET="exporter-goora"
    ;;
  godror)
    TAGS="godror"
    CGO_ENABLED=1
    DOCKER_TARGET="exporter-godror"
    ;;
  *)
    echo "Unsupported target: ${TARGET}"
    echo "$USAGE"
    exit 1
    ;;
esac

copy_workspace_to_container() {
  local container="$1"

  docker exec "${container}" rm -rf /oracle-db-appdev-monitoring
  docker cp "${SCRIPT_DIR}/." "${container}:/oracle-db-appdev-monitoring"
  docker exec "${container}" rm -rf /oracle-db-appdev-monitoring/.git
}

build_darwin_local() {
  echo "Build darwin-arm64"
  make go-build-darwin-arm64 VERSION="$VERSION" TAGS="$TAGS" CGO_ENABLED="$CGO_ENABLED"
  echo "Built for darwin-arm64"
}

docker_image_for_platform() {
  local platform="$1"

  echo "${IMAGE_ID}-${platform}"
}

extract_container_binary() {
  local platform="$1"
  local image="$2"
  local container
  local artifact_dir="oracledb_exporter-${VERSION}.linux-${platform}"
  local output_dir="dist/${artifact_dir}"

  echo "Extract linux-${platform} binary from ${image}"
  mkdir -p "${output_dir}"
  container="$(docker create "${image}")"
  if ! docker cp "${container}:/oracledb_exporter" "${output_dir}/oracledb_exporter"; then
    docker rm "${container}" >/dev/null
    return 1
  fi
  docker rm "${container}" >/dev/null
  chmod +x "${output_dir}/oracledb_exporter"
  (cd dist ; tar cfz "${artifact_dir}.tar.gz" "${artifact_dir}")
  rename_glibc "${platform}"
}

build_ubuntu() {
  local container="ubuntu-build"
  docker run -d --platform "linux/amd64" --name "${container}" "${UBUNTU_IMAGE}" tail -f /dev/null
  copy_workspace_to_container "${container}"
  docker exec "${container}" bash -c "apt-get update -y && \
                                      apt-get -y install podman qemu-user-static golang gcc-aarch64-linux-gnu git make && \
                                      cd oracle-db-appdev-monitoring && \
                                      make go-build-linux-amd64 VERSION=\"${VERSION}\" TAGS=\"${TAGS}\" CGO_ENABLED=\"${CGO_ENABLED}\" && \
                                      make go-build-linux-gcc-arm64 VERSION=\"${VERSION}\" TAGS=\"${TAGS}\" CGO_ENABLED=\"${CGO_ENABLED}\""


  docker cp "${container}:/oracle-db-appdev-monitoring/dist/oracledb_exporter-${VERSION}.linux-amd64.tar.gz" "dist/"
  docker cp "${container}:/oracle-db-appdev-monitoring/dist/oracledb_exporter-${VERSION}.linux-arm64.tar.gz" "dist/"

  docker stop "$container"
  docker rm "$container"
}

build_container_artifacts() {
  echo "Building container images"
  make docker-arm VERSION="$VERSION" IMAGE_ID="$IMAGE_ID" TAGS="$TAGS" CGO_ENABLED="$CGO_ENABLED" DOCKER_TARGET="$DOCKER_TARGET"
  make docker-amd VERSION="$VERSION" IMAGE_ID="$IMAGE_ID" TAGS="$TAGS" CGO_ENABLED="$CGO_ENABLED" DOCKER_TARGET="$DOCKER_TARGET"
  echo "Build complete for container images"

  extract_container_binary "arm64" "$(docker_image_for_platform "arm64")"
  extract_container_binary "amd64" "$(docker_image_for_platform "amd64")"
  echo "Build complete for container binary tarballs"
}

rename_glibc() {
  local platform="$1"

  local f1="oracledb_exporter-${VERSION}.linux-${platform}.tar.gz"
  local f2="oracledb_exporter-${VERSION}.linux-${platform}-glibc-${OL8_GLIBC_VERSION}.tar.gz"

  mv "dist/$f1" "dist/$f2" 2>/dev/null
}

# clean dist directory before build
rm -rf dist/*

# Create darwin-arm64 artifacts on local host
if [[ -n "$BUILD_DARWIN" ]]; then
  build_darwin_local
fi

# build containers
if [[ -n "$BUILD_CONTAINERS" ]]; then
  build_container_artifacts
fi

if [[ -n "$BUILD_UBUNTU" ]]; then
  # Create Linux artifacts and containers
  build_ubuntu
fi
