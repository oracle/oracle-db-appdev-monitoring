---
title: go-ora Driver
sidebar_position: 5
---

# Using the go-ora database driver

The Oracle AI Database Metrics Exporter experimentally supports compiling with the [go-ora database driver](https://github.com/sijms/go-ora).  By default, the exporter compiles using the `godror` database driver, which uses CGO execution to invoke Oracle Instant Client. the go-ora driver presents an option for users who want to use a "thin" database client without the Oracle Instant Client and CGO.

### Configuring go-ora

Because go-ora does not use Oracle Instant Client, it is recommended to provide all connection string options in the `database.url` property:

```yaml
databases:
  go_ora_db:
    username: myuser
    password: ******
    url: my_tnsname?wallet=/path/to/wallet&ssl=1&FAST+LOGIN=false
```

#### Disable fast login with go-ora v3.0.1

With go-ora v3.0.1, add `FAST+LOGIN=false` to each database URL to work around a connection-cookie regression that can cause the second and subsequent physical connections to fail with `EOF`, preventing connection-pool warmup and metric collection. See [go-ora issue #732](https://github.com/sijms/go-ora/issues/732).

For a URL without query parameters, use `?FAST+LOGIN=false`:

```yaml
databases:
  go_ora_db:
    username: myuser
    password: ******
    url: localhost:1521/FREEPDB1?FAST+LOGIN=false
```

If the URL already has query parameters, append `&FAST+LOGIN=false`, as in the wallet example above. The `+` encodes the space in the option name `FAST LOGIN`. Disabling this optimization uses the full connection handshake while retaining connection pooling.

### Build with go-ora

To build using `go-ora` instead of `godror`, set `TAGS=goora CGO_ENABLED=0`:

```bash
make go-build TAGS=goora CGO_ENABLED=0
```
