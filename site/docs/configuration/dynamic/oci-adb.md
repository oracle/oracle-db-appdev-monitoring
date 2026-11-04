---
title: OCI Autonomous Database
sidebar_position: 2
---

# OCI Autonomous Database discovery

Configure `databasesFrom.oci` to discover Autonomous Databases from OCI metadata and freeform tags. The exporter lists all result pages in each configured compartment, filters by lifecycle state and required tags, selects a secure connection profile, resolves credentials, and registers ordinary database scrape targets.

This implementation covers Autonomous Database in explicit compartments in one OCI provider-selected region. It does not recursively search compartments, search other regions, read defined tags, or discover Base Database and Exadata DB systems.

## Discovery configuration

```yaml
databasesFrom:
  timeout: 30s
  refreshInterval: 5m
  walletRefreshInterval: 24h
  oci:
    # config_file (default), instance_principal, resource_principal,
    # or workload_identity
    auth: workload_identity
    compartments:
      - ocid1.compartment.oc1..example
    filters:
      lifecycleState: AVAILABLE
      requiredTags:
        oracledb-metrics-exporter-enabled: "true"
```

`AVAILABLE` and the enablement tag above are the default filters. Every required freeform tag must match exactly. Supplying a non-empty custom `requiredTags` map replaces the default map.

`timeout` bounds each startup discovery or metadata refresh, including startup wallet preparation, and must be positive. `refreshInterval` defaults to five minutes; `0s` disables metadata polling. `walletRefreshInterval` defaults to 24 hours; `0s` disables scheduled renewal of wallets managed by discovery. Negative intervals are invalid.

Configuration validation does not call OCI.

## Database tags

All tag keys below have the prefix `oracledb-metrics-exporter-`.

| Suffix | Example value | Behavior |
| --- | --- | --- |
| `enabled` | `true` | Matches the default discovery filter. |
| `dbname` | `production-orders` | Overrides the exporter target name and database label; defaults to OCI `dbName`. |
| `connect-service` | `LOW` | Selects `LOW` (default), `MEDIUM`, `HIGH`, `TP`, or `TPURGENT` from returned connection profiles. |
| `vault-id` | `ocid1.vault.oc1..example` | Vault containing the credential and/or wallet secret. |
| `usernamePasswordSecret` | `orders-credentials` | Name of a JSON credential secret in that Vault. |
| `wallet-secret` | `orders-wallet` | Name of a Vault secret containing wallet ZIP bytes; requests an mTLS profile. |
| `is-mtls-connection-required` | `true` | Requests mTLS. `false` explicitly requests walletless TLS and is rejected if OCI says mTLS is required. |

For a walletless TLS target, the usual tags are:

```text
oracledb-metrics-exporter-enabled: true
oracledb-metrics-exporter-dbname: production-orders
oracledb-metrics-exporter-connect-service: LOW
oracledb-metrics-exporter-vault-id: ocid1.vault.oc1..example
oracledb-metrics-exporter-usernamePasswordSecret: orders-credentials
```

The credential secret contains:

```json
{"username":"exporter","password":"example"}
```

The discovery authentication mode is also used for these special Vault references. OCI authentication authorizes control-plane operations; database username/password authentication and SQL grants remain separate requirements.

An override name is trimmed; a present but empty override skips the resource. Names do not change the database connection service or resource identity. The OCI resource OCID is retained separately. Static entries take precedence over discovered targets with the same effective name; among discovered name collisions, the first valid entry in stable OCID order wins with a warning identifying both resources. Assign distinct `dbname` tags to avoid collisions across compartments.

Invalid individual configurations are skipped with warnings. A successful listing with zero usable targets remains running and exposes discovery status.

## Connection service and transport

The exporter selects structured OCI connection profiles by consumer group and TLS authentication, preserving TCPS and enabling server identity verification. FQDN/direct profiles are preferred, with deterministic selection when OCI reorders profiles. Bare legacy `host:port/service` fields are not used as a secure fallback. `DEDICATED` is not accepted as a consumer group.

`LOW` is the default for monitoring. You can select any of the five groups when the database returns a compatible profile. The exporter does not switch to another service when the requested service is unavailable. See [Oracle's service guidance](https://docs.oracle.com/en-us/iaas/autonomous-database-serverless/doc/predefined-database-services-names.html).

OCI's actual `isMtlsConnectionRequired` metadata is respected. If no transport preference is specified and only mutual profiles are available, discovery can select mTLS. A tag cannot enable walletless TLS on a database that requires mTLS. A supplied wallet secret or `tnsAdmin` also requests a mutual profile.

An explicit `url` configuration tag is supported only as a full TCPS descriptor with server identity verification. An explicit `connect-service` takes precedence over `url`.

| Driver | Walletless TLS path | Automatic mTLS wallet path |
| --- | --- | --- |
| godror | Full OCI descriptor and Oracle Client verification | Per-target `tnsAdmin` with auto-login wallet |
| go-ora | Encoded descriptor with SSL and certificate verification enabled | Per-target wallet option using `cwallet.sso` |
| go-oracledb (experimental) | Full OCI descriptor | Unsupported; initialization reports an explicit error |

This table describes implemented paths validated by local configuration and driver tests. Live OCI connections still require validation for the intended deployment. go-ora uses certificate-chain and hostname verification; it does not independently enforce the descriptor's full `SSL_SERVER_CERT_DN` constraint.

## Wallet lifecycle

When mTLS is selected, discovery uses a preconfigured `tnsAdmin` directory if present. Otherwise it reads a wallet ZIP from `wallet-secret`, or calls OCI `GenerateAutonomousDatabaseWallet` when no wallet secret is configured.

A wallet Vault secret must contain the original ZIP bytes. The OCI secret-bundle base64 encoding is decoded once; do not add an extra application-level base64 layer.

Downloaded/generated wallets are extracted into separate private temporary directories. Directories use mode `0700`, files use `0600`, extraction rejects unsafe entries and oversized archives, and the local `sqlnet.ora` points to that target's wallet with server DN matching enabled. The ZIP must contain a non-empty root `cwallet.sso`. A generated wallet's encryption password is not persisted; the supported automatic paths use its auto-login wallet.

Wallets are cached for the process lifetime, so metadata polling never downloads wallets. Managed wallets are renewed through a controlled process restart after `walletRefreshInterval`; this is interval-based renewal, not certificate-expiry detection. Renewal first requires a successful metadata probe; if OCI listing fails, active targets stay running and renewal retries at the metadata interval (five minutes if metadata polling is disabled). Externally managed `tnsAdmin` directories are not renewed or deleted. Discovery-owned directories are removed after database pools close during normal shutdown/restart.

## Refresh, failures, and credentials

Metadata polling compares effective targets, including resource OCIDs, names, labels, connection settings, and wallet sources. A complete successful snapshot that changes these values requests the existing graceful process restart. Shutdown drains workers and closes pools before the replacement process rediscovers and reconnects. This introduces a brief scrape interruption; it does not mutate the collector's target list in place.

An OCI listing failure during polling retains current targets and does not request removal or restart. At startup, discovery failure continues with static targets when present; without static targets, startup fails. A successful empty discovery remains running so later metadata polls can detect new targets.

Combined OCI credentials are read when a SQL handle is created or rebuilt. On an OCI-backed authentication failure, the old handle is retired; after `metrics.connectionBackoff`, rebuilding fetches the current secret. Ordinary pool replacement connections use their existing connector credentials until an authentication failure or restart. OCI credential calls observe caller cancellation and a maximum 30-second deadline.

Discovery exposes these metrics:

- `oracledb_exporter_discovery_success`
- `oracledb_exporter_discovery_duration_seconds`
- `oracledb_exporter_discovery_last_success_timestamp_seconds`
- `oracledb_exporter_discovery_targets`
- `oracledb_exporter_discovery_matched_targets`
- `oracledb_exporter_discovery_skipped_targets`

Matched counts resources after lifecycle/tag filtering; skipped counts invalid targets, duplicate names, and static-name collisions. Counts retain the last successful values during discovery failures. The target count reports accepted discovered entries after static precedence in the last successful snapshot; discovery success does not establish successful SQL connections. Use normal per-database up/error metrics and logs for connection failures.

## IAM, networking, and database prerequisites

For instance-principal authentication, example compartment-scoped policies are:

```text
Allow dynamic-group exporter-instances to inspect autonomous-databases in compartment databases
Allow dynamic-group exporter-instances to read secret-bundles in compartment secrets
```

When discovery generates wallets, grant `read autonomous-databases` instead of only `inspect` in the database compartment. That verb includes wallet generation. Adjust the principal clause for the chosen authentication mode; the dynamic-group example does not apply directly to OKE workload identity. Scope policies to the actual resource compartments, including a separate secret compartment when needed. See [ADB IAM permissions](https://docs.oracle.com/en/cloud/paas/autonomous-database/serverless/adbsb/autonomous-database-iam-policies.html) and [Vault secret policy examples](https://docs.oracle.com/en/cloud/paas/autonomous-database/serverless/adbsb/vault-secret-credentials-oci.html).

The exporter needs DNS and HTTPS access to OCI database/secret APIs, and network access to the returned database endpoint and port. Private endpoints require appropriate VCN routing/DNS; public endpoints require the database's access controls to admit the exporter. IAM permissions do not establish SQL connectivity.

Create a database user with `CREATE SESSION`, permission to initialize `DBMS_APPLICATION_INFO` client information, and the SELECT/EXECUTE grants required by the configured metric queries. The default metric set queries database monitoring views; a successful `SELECT 1 FROM DUAL` is only a connection probe, not proof of all metric privileges. See the normal exporter database setup documentation for metric grants.

## Live validation

The opt-in test `TestOCIDiscoveryLiveScrape` exercises listing, tag conversion, Vault credentials, secure connection, session initialization, and a Prometheus metric from `SELECT 1 FROM DUAL`. Ordinary test runs skip it.

Prepare a dedicated test compartment/database with the tags above and a discovery YAML file with no static databases. Configure credentials for the chosen OCI auth mode and run from the repository root:

```shell
OCI_DISCOVERY_TEST_CONFIG=/absolute/path/discovery-test.yaml go test ./collector -run '^TestOCIDiscoveryLiveScrape$' -count=1 -v
OCI_DISCOVERY_TEST_CONFIG=/absolute/path/discovery-test.yaml go test -tags goora ./collector -run '^TestOCIDiscoveryLiveScrape$' -count=1 -v
OCI_DISCOVERY_TEST_CONFIG=/absolute/path/discovery-test.yaml go test -tags oracledb ./collector -run '^TestOCIDiscoveryLiveScrape$' -count=1 -v
```

Use a walletless target for the experimental go-oracledb run. For mTLS targets without a wallet secret, this test calls OCI wallet generation. It does not create databases, change access controls, or rotate passwords. A test run must produce one probe metric per discovered target to pass.
