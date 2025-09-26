---
title: Changelog
sidebar_position: 2
---

# Release Notes

List of upcoming and historic changes to the exporter.

### Next, in-development

Our current priorities to support metrics for advanced database features and use cases, like Exadata, GoldenGate, and views included in the Oracle Diagnostics Pack.

- Updated project dependencies.
- Standardize multi-arch builds and document supported database versions.
- If the exporter fails to connect to a database due to invalid credentials (ORA-01017 error), that database configuration will be invalidated and the exporter will not attempt to re-establish the database connection. Other databases will continue to be scraped.
- Metrics with an empty databases array (`databases = []`) are now considered disabled, and will not be scraped.
- Increased the default query timeout for the `top_sql` metric to 10 seconds (previously 5 seconds).
- Metrics using the `scrapeinterval` property will no longer be scraped on every request if they have a cached value. This only applies when the metrics exporter is configured to scrape metrics _on request_, rather than on a global interval.
- Metrics using the `fieldtoappend` property now support labels. The `wait_time` and `activity` default metrics use the `fieldtoappend` property, and now properly display their labels. 
- Fix `wait_time` default metric to work with Oracle Database 19c.
- Fix an issue where the exporter would unnecessarily scrape metrics with a custom scrape interval.

Thank you to the following people for their suggestions and contributions:
- [@romankspb](https://github.com/romankspb)
- [@muhammadabdullah-amjad](https://github.com/muhammadabdullah-amjad)
- [@MansuyDavid](https://github.com/MansuyDavid)
- [@borkoz](https://github.com/borkoz)
- [@ristagg](https://github.com/ristagg)

### Version 2.0.4, September 8, 2025

This release includes the following changes:
- Added WARN logging when database configurations are duplicated in the exporter configuration.
- Added INST_ID to `gv$` query metrics as a label.
- Fixed multiple concurrency bugs when the exporter is connected to multiple databases and using a custom scrape interval.

Thank you to the following people for their suggestions and contributions:

- [@Supporterino](https://github.com/Supporterino)
- [@msafdal](https://github.com/msafdal)
- [@aberinnj](https://github.com/aberinnj)

### Version 2.0.3, August 27, 2025

This release includes the following changes:
- Enable configuration of the prometheus webserver from the config file using the `web` prefix.
- Allow loading of database password(s) from a file.
- Fixed a bug where database type (CDB, PDB, etc.) was not reported in certain situations.
- Fixed a bug where literal passwords containing the '$' character (in the config file) would be evaluated as environment variables. To use literal passwords with the '$' character, escape the '$' character with a second '$': `$test$pwd` becomes `$$test$$pwd`.
- Fixed a bug when using `metrics.scrapeInterval` combined with per-metric scrape intervals that made the available metrics data set inconsistent.

Thank you to the following people for their suggestions and contributions:

- [@Supporterino](https://github.com/Supporterino)
- [@neilschelly](https://github.com/neilschelly)
- [@aberinnj](https://github.com/aberinnj)
- [@redelang](https://github.com/redelang)
- [@qrkop](https://github.com/qrkop)
- [@KevDi](https://github.com/KevDi)
- [@bomuva](https://github.com/bomuva)
- [@anilmoris](https://github.com/anilmoris)
- [@Sycri](https://github.com/Sycri)
- [@kizuna-lek](https://github.com/kizuna-lek)
- [@rfrozza](https://github.com/rfrozza)
- [@neilschelly](https://github.com/neilschelly)

### Version 2.0.2, June 24, 2025

This release includes the following changes:

- Fixed a case-sensitive issue with resource name in the default metrics file.
- Add query timeouts to initial database connections, which could cause the exporter to hang in multi-database configurations
- Fix an issue where rapidly acquiring connections could cause the exporter to crash. This was more common in multi-database configurations, due to the increased number of connection pools.
- Update some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- [@rfrozza](https://github.com/rfrozza)
- [@neilschelly](https://github.com/neilschelly)
- [@rafal-szypulka](https://github.com/rafal-szypulka)
- [@darkelfit](https://github.com/darkelfit)

### Version 2.0.1, June 12, 2025

This release includes the following changes:

- Use gv$ views instead of v$ views to allow collection of metrics from all instances in a cluster. (In preparation for RAC support).
- Update some third-party dependencies.

### Version 2.0.0, May 27, 2025

This release includes the following changes:

- Fixed an issue with `scrapeinterval` that could cause metrics not to be scraped (#172, #176).
- Added configuration through a YAML file, passed using the `--config.file` command-line argument. Backwards compatibility is maintained for the command-line arguments, through it is recommended to use the configuration file from the 2.0.0 release onward. It is not recommended to use a combination of command-line arguments and the configuration file.
- Added support for multiple databases through the configuration file. As many database instances may be specified as needed, which will be scraped concurrently (#89).
- Updated provided dashboards.
- Updated some third-party dependencies.

### Version 1.6.1, May 2, 2025

This release includes the following changes:

- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- Deepak A.

### Version 1.6.0, April 18, 2025

This release includes the following changes:

- Added support for Azure Key Vault (#200).
- [4Aiur](https://github.com/4Aiur) added missing grants for alert log to the demo environment (#207).
- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- Brian, Damian et al.
- [4Aiur](https://github.com/4Aiur)


### Version 1.5.5, March 13, 2025

This release includes the following changes:

- [@VictorErmakov](https://github.com/VictorErmakov) updated the docker-compose sample with connection pool parameters to avoid fast connect cycling (#191).
- Update default values for connection pool parameters to use go-sql pooling by default to avoid fast connet cycling.
- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- [@VictorErmakov](https://github.com/VictorErmakov)


### Version 1.5.4, March 3, 2025

This release includes the following changes:

- Based of this recommendation from [godror](https://github.com/godror/godror?tab=readme-ov-file#pooling), which relates to the two following items, and in discussion with the ODPI-C team, we have introduced additional parameters to allow you to set connection pool parameters, and have set defaults which will avoid fast connect cycling.  It is our expectation that a fix may be produced in the underlying ODPI-C library for the underlying issue.  In the mean time, these changes will avoid the conditions under which the error can occur.
- Fix malloc error (#177, #181).
- Fix intermittent connection issues with ADB-S when exporter is run in a container (#169).
- Fix Multiple custom metrics files overwrite one another (#179).
- Replace go-kit/log with log/slog, due to upstream changes in prometheus/common.
- Add support for additional admin roles, expanding list of options for `DB_ROLE` to `SYSDBA`, `SYSOPER`, `SYSBACKUP`, `SYSDG`, `SYSKM`, `SYSRAC` and `SYSASM` (#180).
- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- [@Jman1993](https://github.com/Jman1993)
- [@oey](https://github.com/oey)
- [@jlembeck06](https://github.com/jlembeck06)
- [@Jman1993](https://github.com/Jman1993)
- [@PeterP55P](https://github.com/PeterP55P)
- [@rlagyu0](https://github.com/rlagyu0)
- [@Sycri](https://github.com/Sycri)

Thank you to [@tgulacsi](https://github.com/tgulacsi) for changes in godror (https://github.com/godror/godror/issues/361, https://github.com/godror/godror/issues/360), and to [@cjbj](https://github.com/cjbj) and [@sudarshan12s](https://github.com/sudarshan12s) for support and guidance from ODPI-C (https://github.com/oracle/odpi).

In this release, we also continued some minor code refactoring.

### Version 1.5.3, January 28, 2025

*Known issue*: This release has a known issue that results in the error message `malloc(): unsorted double linked list corrupted`.
We recommend staying on 1.5.2 until a new release with a fix is available.  We hope to have a fix by early March.

This release includes the following changes:

- Fix over-zealous supression of errors when `ignorezeroresult = true` (#168).
- When `scrapeinterval` is set, do first scrape immediately, not after the interval (#166).
- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- [@redelang](https://github.com/redelang)

In this release, we also started some minor code refactoring.

### Version 1.5.2, December 2, 2024

This release includes the following changes:

- Update the metric defintion for tablespace usage to report more accurate temp space usage.
- Revert InstantClient to 21c version due to ADB connectivity issue.
- Update documentation to explain how to obtain credentials from a wallet.
- Fix race condition on err variable in scrape() func (by @valrusu).
- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- [@aureliocirella](https://github.com/aureliocirella)
- [@mitoeth](https://github.com/mitoeth)
- [@valrusu](https://github.com/valrusu)

### Version 1.5.1, October 28, 2024

This release includes the following changes:

- Added support for using the `TNS_ADMIN` environment variable, which fixes an issue when connecting to Autonomous Database instances using TNS name.
- Updated InstantClient to 23ai version for amd64 and latest available 19.24 version for arm64.
- Fixed an issue with wrong `LD_LIBRARY_PATH` on some platforms. (#136)
- Added documentation and an example of using the `scrapeinterval` setting to change the interval at which a certain metric is colected.
- Added notes to documentation for extra security parameters needed when using a wallet with Podman.
- Updated some third-party dependencies.

### Version 1.5.0, September 26, 2024

This release includes the following changes:

- Support for running the exporter on ARM processors (darwin and linux).
- Updated some third-party dependencies.
- Updated the "test/demo environment" to use newer version of Oracle Database (23.5.0.24.07) and faster startup.

### Version 1.4.0, September 4, 2024

This release includes the following changes:

- Allow multiple custom metrics definition files.
- Allow query timeout per-metric.
- Allow scrape interval per-metric.
- Updated some third-party dependencies.

### Version 1.3.1, July 22, 2024

This release includes the following changes:

- Alert logs can be disabled by setting parameter `log.disable` to `1`.
- Alert log exporter will stop if it gets three consecutive failures.
- Updated the list of required permissions.
- Updated the TxEventQ sample dashboard.
- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- [@tux-jochen](https://github.com/tux-jochen)

### Version 1.3.0, June 7, 2024

This release includes the following changes:

- Alert logs can be exported for collection by a log reader like Promtail or FluentBit. Default
  output to `/log/alert.log` in JSON format.
- Provide ability to connect as SYSDBA or SYSOPER by setting DB_ROLE.
- New default metric is added to report the type of database connected to (CDB or PDB).
- New default metrics are added for cache hit ratios.
- Default metrics updated to suppress spurious warnings in log.
- Wait class metric updated to use a better query.
- The sample dashboard is updated to include new metrics.
- Fixed a bug which prevented periodic freeing of memory.
- Set CLIENT_INFO to a meaningful value.
- Update Go toolchain to 1.22.4.
- Updated some third-party dependencies.

Thank you to the following people for their suggestions and contributions:

- [@pioro](https://github.com/pioro)
- [@savoir81](https://github.com/savoir81)

### Version 1.2.1, April 16, 2024

This release includes the following changes:

- Accept max idle and open connections settings as parameters.
- Updated some third-party dependencies.

### Version 1.2.0, January 17, 2024

This release includes the following changes:

- Introduced a new feature to periodically restart the process if requested.
- Introduced a new feature to periodically attempt to free OS memory if requested.
- Updated some third-party dependencies.

### Version 1.1.1, November 28, 2023

This release just updates some third-party dependencies.

### Version 1.1, October 27, 2023

This release includes the following changes:

- The query for the standard metric `wait_class` has been updated so that it will work in both container databases
  and pluggable databases, including in Oracle Autonomous Database instances.  Note that this query will not return
  any data unless the database instance is under load.
- Support for reading the database password from OCI Vault has been added (see [details](../configuration/oci-vault.md))
- Log messages have been improved
- Some dependencies have been updated

### Version 1.0, September 13, 2023

The first production release, v1.0, includes the following features:

- A number of [standard metrics](../getting-started/default-metrics.md) are exposed,
- Users can define [custom metrics](../configuration/custom-metrics.md),
- Oracle regularly reviews third-party licenses and scans the code and images, including transitive/recursive dependencies for issues,
- Connection to Oracle can be a basic connection or use an Oracle Wallet and TLS - connection to Oracle Autonomous Database is supported,
- Metrics for Oracle Transactional Event Queues are also supported,
- A Grafana dashboard is provided for Transactional Event Queues, and
- A pre-built container image is provided, based on Oracle Linux, and optimized for size and security.

Note that this exporter uses a different Oracle Database driver which in turn uses code directly written by Oracle to access the database.  This driver does require an Oracle client.  In this initial release, the client is bundled into the container image, however we intend to make that optional in order to minimize the image size.

The interfaces for this version have been kept as close as possible to those of earlier alpha releases in this repository to assist with migration.  However, it should be expected that there may be breaking changes in future releases.