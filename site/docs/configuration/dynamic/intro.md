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
| [OCI Autonomous AI Database](./oci-adb.md) | Supported (WIP) | Discovers tagged Autonomous Database resources and maps their connection and OCI Vault settings. Runtime activation and mTLS wallet support are still TODO. |

## How it will work

The exporter configuration identifies a discovery source and its scope under `databasesFrom`. The source then returns named database configurations that the exporter can use alongside its normal connection and metrics settings. Static database configuration remains the appropriate choice for databases that are not available through a supported discovery source.

As more sources are added, they will appear in the support matrix and have their own configuration pages in this section.
