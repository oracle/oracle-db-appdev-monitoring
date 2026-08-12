---
title: OCI Autonomous Database
sidebar_position: 2
---

# OCI Autonomous Database discovery

:::warning Work in progress

This page describes the OCI Autonomous Database discovery code that is complete today. It is not yet wired into exporter startup or configuration reload, so adding this configuration does not yet add scrape targets. The remaining integration work is tracked below as TODOs.

:::

The discovery configuration identifies Autonomous Databases in one or more OCI compartments. The exporter lists Autonomous Database resources, follows all OCI result pages, filters by lifecycle state, and requires every configured freeform tag to match.

Each discovered database is named from the Autonomous Database `dbName`. That name will become the key in the exporter's `databases` map when discovery is connected to configuration loading.

## Discovery configuration

`databasesFrom.oci` accepts the OCI authentication mode, the compartments to search, and optional filters:

```yaml
databasesFrom:
  oci:
    # config_file, instance_principal, resource_principal, or workload_identity
    auth: workload_identity
    compartments:
      - ocid1.compartment.oc1..example
    filters:
      lifecycleState: AVAILABLE # defaults to AVAILABLE
      requiredTags:
        oracledb-metrics-exporter-enabled: "true" # default discovery tag
```

`requiredTags` is an AND filter: an Autonomous Database must contain every key/value pair to be discovered. `lifecycleState` is sent to OCI when listing databases.

## Freeform tag configuration

Discovery reads freeform tags prefixed with `oracledb-metrics-exporter-`. These tags provide the discovered database's connection settings.

| Tag suffix | Example | Current behavior |
| --- | --- | --- |
| `vault-id` | `oracledb-metrics-exporter-vault-id: ocid1.vault.oc1..example` | Sets `vault.oci.id`. |
| `usernamePasswordSecret` | `oracledb-metrics-exporter-usernamePasswordSecret: orders-credentials` | Sets `vault.oci.usernamePasswordSecret`. The secret must contain the JSON credentials described in [OCI Vault](../oci-vault.md). |
| `connect-service` | `oracledb-metrics-exporter-connect-service: HIGH` | Uses the ADB `HIGH`, `MEDIUM`, `LOW`, or `DEDICATED` connection string as the database URL. |
| `wallet-secret` | `oracledb-metrics-exporter-wallet-secret: orders-wallet` | Recognized but ignored. See TODOs. |
| `is-mtls-connection-required` | `oracledb-metrics-exporter-is-mtls-connection-required: "true"` | Recognized but ignored. See TODOs. |

The `auth` value from `databasesFrom.oci` is also applied to the discovered OCI Vault configuration.

### Other database connection settings

Any other tag with this prefix is decoded into the database configuration. The part after the prefix must use the normal configuration key, and its value must be valid YAML for that field's type. For example:

```text
oracledb-metrics-exporter-role: SYSDBA
oracledb-metrics-exporter-connMaxLifetime: 5m
oracledb-metrics-exporter-maxOpenConns: 20
oracledb-metrics-exporter-maxIdleConns: 10
oracledb-metrics-exporter-queryTimeout: 15
```

This currently supports the existing database connection fields, including `role`, `externalAuth`, connection-pool settings, `queryTimeout`, and `labels`. Unknown or incorrectly typed fields are rejected by strict configuration decoding.

## TODO

- Wire discovery into exporter startup and configuration-file reload, then merge discovered entries into the active `databases` map.
- Define collision behavior when a discovered `dbName` also appears in static `databases` configuration.
- Fetch and configure wallets referenced by `wallet-secret`.
- Implement mTLS handling, including `is-mtls-connection-required` and wallet setup.
- Add end-to-end OCI discovery tests and document the required OCI IAM policies.
