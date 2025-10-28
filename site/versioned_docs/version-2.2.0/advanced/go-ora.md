---
title: go-ora Driver
sidebar_position: 5
---

# Using the go-ora database driver

The Oracle Database Metrics Exporter experimentally supports compiling with the [go-ora database driver](https://github.com/sijms/go-ora).  By default, the exporter compiles using the `godror` database driver, which uses CGO execution to invoke Oracle Instant Client. the go-ora driver presents an option for users who want to use a "thin" database client without the Oracle Instant Client and CGO.

### Configuring go-ora

Because go-ora does not use Oracle Instant Client, it is recommended to provide all connection string options in the `database.url` property:

```yaml
databases:
  go_ora_db:
    username: myuser
    password: ******
    url: my_tnsname?wallet=/path/to/wallet&ssl=1
```

### Build with go-ora

To build using `go-ora` instead of `godror`, set `TAGS=goora CGO_ENABLED=0`:

```bash
make go-build TAGS=goora CGO_ENABLED=0
```
