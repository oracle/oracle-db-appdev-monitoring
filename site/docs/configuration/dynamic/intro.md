---
title: Dynamic Database Discovery
sidebar_position: 1
---

# Dynamic Database Discovery

Dynamic database discovery lets the exporter find database scrape targets from a supported control plane instead of requiring every target to be written in the `databases` section of its configuration file. Configure a source under `databasesFrom` to tell the exporter where to discover databases.

A discovery source selects databases, turns the available metadata into a database configuration, and supplies each result with a stable database name. It is designed both for autoconfiguration and for exporters that monitor more databases than would be feasible to configure manually. It keeps connection settings and credentials with the database resource rather than duplicating them in every exporter deployment.

## Support matrix

| Discovery source | Status | Notes |
| --- | --- | --- |
| [OCI Autonomous AI Database](./oci-adb.md) | Supported | Discovers tagged Autonomous Database resources and maps their connection and OCI Vault settings. Selects TLS/mTLS profiles, resolves OCI Vault credentials, provisions wallets for supported drivers, and refreshes through controlled restarts. |

## How it works

The exporter configuration identifies a discovery source and its scope under `databasesFrom`. The source returns named database configurations that the exporter uses alongside its normal connection and metrics settings. Periodic metadata polling detects changes and requests a graceful process restart; failed listings retain active targets. Static database configuration remains the appropriate choice for databases that are not available through a supported discovery source.

As more sources are added, they will appear in the support matrix and have their own configuration pages in this section.
