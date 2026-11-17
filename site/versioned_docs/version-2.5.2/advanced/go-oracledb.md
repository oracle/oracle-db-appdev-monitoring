---
title: go-oracledb Driver
sidebar_position: 6
---

# Using the go-oracledb database driver

The exporter experimentally supports Oracle's [go-oracledb driver](https://github.com/oracle/go-oracledb). This pure Go driver supports Oracle Database 19c and later and does not require CGO or Oracle Instant Client. The default build continues to use godror.

### Build with go-oracledb

```bash
CGO_ENABLED=0 go build -tags oracledb
# Or build packaged artifacts:
make go-build TAGS=oracledb CGO_ENABLED=0
# Run tests:
make go-test-oracledb
```

Select one experimental driver tag at a time; do not combine `oracledb` and `goora`.

To build a Linux container locally:

```bash
make docker-arm TAGS=oracledb CGO_ENABLED=0 DOCKER_TARGET=exporter-oracledb
# For AMD64, use docker-amd instead of docker-arm.
```

### Configure connections

Use separate username and password settings and an Easy Connect string or full connect descriptor in `database.url`:

```yaml
databases:
  oracle_db:
    username: exporter
    password: your_password
    url: localhost:1521/FREEPDB1
```

Existing password-file and vault credential resolution also applies. Credentials are passed to the driver's connector configuration, preserving special characters without DSN escaping. Do not embed credentials in `url`.

```yaml
databases:
  oracle_db:
    username: exporter
    password: your_password
    url: "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=FREEPDB1)))"
```

Use driver-supported Easy Connect query parameters for connection options. See the [upstream driver documentation](https://github.com/oracle/go-oracledb) for TCP, TCPS, TLS/wallet options, and configuration precedence. The upstream driver's environment variables can override programmatic connector settings. The exporter accepts its own `--config.file` flag; configure driver options through the URL or environment rather than upstream driver CLI flags.
