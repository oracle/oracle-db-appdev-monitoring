---
title: Alert Logs
sidebar_position: 5
---

# Alert logs

The exporter reads Oracle alert logs and writes JSON lines to a file, stdout, or an OTLP/gRPC receiver. Collection uses local checkpoint files independently of the destination. It does not write collection state back to Oracle.

```yaml
log:
  destination: /log/alert.log
  interval: 15s
  stateDirectory: /log/checkpoints
  queryTimeout: 30s
  # 0 means all retained history on the first startup without a checkpoint.
  initialLookback: 0s
  maxRows: 10000
  maxBytes: 16777216
  perDatabaseFiles: false
  messageLevelEnabled: false
  disable: 0
```

The built-in alert stream uses the same plain YAML format, SQL source, and collector as custom query streams. `config/default_log_streams.yaml` defines the default query; `log.messageLevelEnabled` selects the plain YAML variant in `config/default_log_streams_with_levels.yaml`. Both files can be used directly as replacement or custom definitions.

`log.level` and `log.format` control exporter diagnostics; they do not filter database events. The existing destination, interval, perDatabaseFiles, and messageLevelEnabled options configure the embedded defaults. Set `logStreams.default` to a YAML or TOML file to replace those defaults, or override the `alert` stream ID in `logStreams.custom`. Explicit file definitions use their own collection and output settings. `log.disable: 1` disables default streams, independently of custom [query streams](./query-logs.md).

The default output is a file. With `perDatabaseFiles: true`, `/log/alert.log` becomes `/log/alert-<database>.log`; database names used in paths cannot contain path separators. Shared destinations serialize writes within the exporter process; other processes must use separate output files. Rename-and-create file rotation is supported because each delivered batch opens the current path. Copy-and-truncate rotation can lose unshipped data and is unsupported.

To write to stdout, set `log.output.type: stdout`, or use `log.destination: /dev/stdout`. Stdout cannot use per-database files. Exporter operational logs go to stderr. Stdout is never opened for reading or closed by the collector.

## Log checkpoints

Log checkpoints require writable storage at `log.stateDirectory`. Use a persistent volume when progress must survive pod/container replacement. When this value is not present, it defaults to a `checkpoints` subdirectory beside the effective log destination, an explicit `log.stateDirectory` takes precedence. Missing or unwritable checkpoint storage pauses logging and retries with backoff; metrics collection continues.

A log checkpoint contains a timestamp cursor (`completedThrough`) and version format for each configured database alias and stream ID. The cursor marks the end of the last fully delivered read range, including empty ranges. Files are replaced atomically once that range is acknowledged, using a process lock. Pause exporter collection if you must deliberately move a log checkpoint (to reset or adjust it).

Set `initialLookback`, for example, to `24h` to limit initial log replay.

Configuration changes reuse existing checkpoints for the same configured database alias and stream ID, including changes to queries, timestamp-column selection, and outputs. Definitions in custom files [reload automatically](./query-logs.md#reloading-definitions). Checkpoint filenames hash the database alias and stream ID rather than embedding connection credentials. Changing the connection behind an alias reuses its cursor; choose a new alias or deliberately reset state when switching to a different source.

## Connection pools

Log reads and metric scrapes share each configured database's connection pool. Keep the default `maxOpenConns: 10` as a starting point for a single alert stream rather than reducing it to one. For multiple streams, budget capacity for metric queries plus one concurrent read per enabled stream for that database. Size driver-native pool limits consistently when configured. This is a capacity guideline, not a reservation or priority guarantee; tune it for query duration and measured pool waits.

Streams run independently on ticker cadence, with one poll at a time per stream; slow polls do not overlap, and failures retain retry backoff. The database connection pool bounds concurrent SQL work, as it does for metric polling. Slow log queries can delay metrics when they occupy a small pool. Database sessions are released before output delivery and checkpoint saves.

## Timestamps and delivery

Each poll resumes strictly after the saved timestamp cursor, without overlap or a deduplication map. **Records that become visible later with timestamps at or before the saved cursor may be missed.** This is an accepted limitation of timestamp-based collection, including after quiet polls. Outages resume from the cursor, subject to Oracle retention and the same late-visibility limitation. Future-dated events wait until their event time falls inside the read range. A source clock moving behind the saved cursor pauses collection until it catches up or the operator resets state.

Rows are ordered deterministically within each query, including timestamp ties. Process, thread, and physical row metadata support investigation, but downstream systems may still display tied timestamps in another order. The exporter preserves original timestamps and message text, including trailing newlines. It does not synthesize nanosecond offsets or merge multiline messages. File and stdout outputs contain the query columns directly, without an event envelope or injected database/stream/source fields. This changes field names from the previous alert-log JSON format. `messageLevelEnabled` adds an SQL-projected `level` column; it does not set OTLP severity.

```json
{"ORIGINATING_TIMESTAMP":"2026-11-10T12:00:00.123Z","MODULE_ID":null,"EXECUTION_CONTEXT_ID":null,"MESSAGE_TEXT":"Example alert\n","MESSAGE_TYPE":5,"PROCESS_ID":"123","THREAD_ID":"1","RECORD_ID":456,"PARTITION":1,"FILENAME":"/path/log.xml","level":"INFO"}
```

All visible records sharing a timestamp are delivered before the cursor advances past that timestamp, including identical records. No per-record fingerprints or occurrence counts are stored. ADR purge/rotation and RAC/container behavior require validation against the deployed Oracle version. The collector follows the physical instance reached by the connection; it does not automatically collect every RAC instance. For instance-local views such as the alert log, configure a separate alias with an instance-specific connection for each source.

Rows and bytes per read are bounded by maxRows and maxBytes. Dense ranges are subdivided by timestamp. If a single timestamp group cannot fit, collection fails explicitly rather than skipping rows; increase the limits. Output batches contain at most 256 events. Large initial backlogs catch up over subsequent polls. An unterminated final output line pauses file delivery until the operator adds a newline or repairs the file; existing output is never truncated automatically.

File batches are synced before checkpoints advance. Checkpoints are written once per completed read range, rather than after every output batch. Failed partial ranges and crashes between output and checkpoint persistence can replay already delivered batches. Successful stdout writes acknowledge only the local write, not downstream ingestion. Checkpoints protect collection progress; they do not guarantee that an external shipper has read a file or prevent external deletion of unshipped output.

## OTLP logs

Metrics and logs share the top-level `otlp` transport configuration. Select `output.type: otlp` to publish a log stream:

```yaml
log:
  stateDirectory: /log/checkpoints
  messageLevelEnabled: true
  output:
    type: otlp

otlp:
  endpoint: https://collector.example:4317
  timeout: 10s
  headers:
    authorization: Bearer REPLACE_ME
  resourceAttributes:
    service.name: oracle-database-observability
  # tls:
  #   caFile: /certs/ca.pem
```

The collector sends bounded batches directly to the OTLP/gRPC service and checkpoints only after acknowledgement. When the receiver reports resource exhaustion, batches are split into smaller requests under the original delivery timeout. A single record that still exceeds the receiver limit fails without advancing the checkpoint. It does not checkpoint on acceptance into an SDK queue. Transport failures and partial rejections retry with backoff. Fix the receiver or configuration to resolve repeated rejections, and expect possible replay of accepted records. Partial rejection message bodies are not included in operational errors.

The shared endpoint, headers, TLS, timeout, and resource attributes apply to both signals. Add a positive `metrics.scrapeInterval` to enable metric publishing too; leaving it unset supports logs-only OTLP publishing while Prometheus metric scraping remains available. Each complete query row is sent as the structured OTLP body, with `ORIGINATING_TIMESTAMP` also used as its event timestamp. Severity and per-record attributes are left unset. Resource attributes come from the transport configuration. Numeric JSON representations can vary by database driver; use [custom query streams](./query-logs.md#queries-and-row-output) for explicit SQL projections and output names.

## Collection metrics

`oracledb_exporter_log_records_total`, `oracledb_exporter_log_errors_total`, and `oracledb_exporter_log_cursor_timestamp_seconds` use database and configured stream labels. The cursor metric records completed read coverage, including quiet periods; it is not the latest event timestamp.
