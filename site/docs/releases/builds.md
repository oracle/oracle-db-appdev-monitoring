---
title: Builds
sidebar_position: 3
---

The Oracle AI Database Metrics Exporter publishes cross-platform builds for each release.

Binaries:
- linux/amd64
- linux/arm64
- darwin/arm64

Container images:
- linux/amd64
- linux/arm64

### Pre-built binaries

Download pre-built binaries from the metrics exporter [GitHub Releases page](https://github.com/oracle/oracle-db-appdev-monitoring/releases).

`linux-amd64`, `linux-arm64`, and  `darwin-arm64` binaries are included, built using GLIBC 2.39. If you require a specific target architecture or are using an older verison of GLIBC, it's recommended to build the metrics exporter binary yourself.

### Container images

```bash
docker pull container-registry.oracle.com/database/observability-exporter:${VERSION}
```

### Build the Oracle AI Database Metrics Exporter

Follow these steps to build the metrics exporter on a Ubuntu Linux system.

#### Install build tools. 

Note that `podman` and `qemu-user-static` are only required for container builds.

```bash
sudo apt-get -y install podman qemu-user-static golang gcc-aarch64-linux-gnu
```

#### How to build metrics exporter binaries

Linux amd64:

```bash
make go-build-linux-amd64
```

Linux arm64 (requires `gcc-aarch64-linux-gnu`):

``bash
make go-build-linux-gcc-arm64
``

Darwin arm64 (requires MacOS platform compilers):

```bash
make go-build-darwin-arm64
```

#### How to build metrics exporter container images

Creates multi-arch container builds for linux/amd64 and linux/arm64:

```
make podman-build
```

### Build on Oracle Linux

To build on Oracle Linux, follow these steps.

#### 1. Install build tools

```bash
dnf install -y git golang make
```

#### 2. Clone the exporter git repository

```bash
git clone git@github.com:oracle/oracle-db-appdev-monitoring.git
```

#### 3. Build the binary

```bash
cd oracle-db-appdev-monitoring
make go-build
```

You will now have a tar.gz and binary file in the `dist/` directory, named according to your target platform.

For example, for the darwin-arm64 platform:

```
dist/
├── oracledb_exporter-2.2.1.darwin-arm64
│   └── oracledb_exporter
└── oracledb_exporter-2.2.1.darwin-arm64.tar.gz
```
