---
title: Custom Query Logs
---

# Custom query logs

Use custom query definition files to export append-only application events using the same checkpoints and outputs as [alert logs](./alert-logs.md). Each stream has a stable ID, optional database selection, an ordered query, and a timestamp column for checkpointing.

List YAML or TOML files under `logStreams.custom` in the exporter configuration, following the `metrics.custom` convention. To replace the embedded defaults, set `logStreams.default` to a definition file. Default files are loaded first, then custom files can replace any stream ID, including `alert`:

```yaml
log:
  # Disable the built-in alert stream if only application events are needed.
  disable: 1
  stateDirectory: /log/checkpoints

logStreams:
  custom:
    - /etc/exporter/application-logs.yaml
```

The separate `/etc/exporter/application-logs.yaml` file contains the definitions:

```yaml
logStreams:
  - id: application-events
    databases: [db1]
    interval: 15s
    queryTimeout: 30s
    initialLookback: 24h
    maxRows: 10000
    maxBytes: 16777216
    query: |
      SELECT event_timestamp, message, severity,
             TO_CHAR(event_id, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS event_id,
             TO_CHAR(amount, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS amount
      FROM application_events
      WHERE event_timestamp >= TO_TIMESTAMP_TZ(:1, 'YYYY-MM-DD"T"HH24:MI:SS.FF9TZH:TZM', 'NLS_DATE_LANGUAGE=American')
        AND event_timestamp <= TO_TIMESTAMP_TZ(:2, 'YYYY-MM-DD"T"HH24:MI:SS.FF9TZH:TZM', 'NLS_DATE_LANGUAGE=American')
      ORDER BY event_timestamp, application_events.event_id
    timestampColumn: event_timestamp
    output:
      type: file
      destination: /log/application-events.log
      perDatabaseFiles: true
```

Paths are relative to the process working directory, as with custom metrics. Files are loaded in order; a later definition with the same stream ID replaces the earlier one. A [sample YAML file](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/custom-log-streams-example/application-logs.yaml) is included in the repository.

TOML uses the same fields and duration strings:

```toml
[[logStreams]]
id = "application-events"
query = "SELECT event_timestamp, message FROM application_events WHERE event_timestamp BETWEEN TO_TIMESTAMP_TZ(:1, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9TZH:TZM') AND TO_TIMESTAMP_TZ(:2, 'YYYY-MM-DD\"T\"HH24:MI:SS.FF9TZH:TZM') ORDER BY event_timestamp, event_id"
interval = "15s"

timestampColumn = "event_timestamp"

[logStreams.output]
type = "stdout"
```

## Reloading definitions

The exporter watches the referenced files, including atomic file replacements and Kubernetes ConfigMap updates. Valid changes reload the log collectors without restarting the exporter or its metric collection. Existing collectors drain and release checkpoint locks before their replacements start. Invalid or unreadable definitions leave the last valid collectors running.

Reloading resumes the checkpoint for the same configured database alias and stream ID when it exists. Query, timestamp-column, and output changes do not invalidate that checkpoint. Changing a stream ID starts a separate checkpoint; removing a stream stops its collector and retains its checkpoint for later reuse. Stop the collector and deliberately remove its checkpoint only when you want to reset its history. Adding or removing definition-file paths in the main configuration follows the existing main-config restart behavior.

## Queries and row output

Supply a query that returns rows, without a terminating SQL client delimiter. The exporter passes SQL directly to the driver without parsing or validating its text. Its positional binds `:1` and `:2` are inclusive UTC read boundaries formatted as `YYYY-MM-DDTHH24:MI:SS.FF9+00:00`. On resumed reads, the lower boundary is the saved cursor plus one nanosecond, so retain all nine fractional digits and do not truncate or round the binds. Convert them using the exact format shown above. Do not substitute timestamp text into SQL. Order by the event timestamp and sufficient tie-breakers to make tied rows deterministic. The collector executes the configured statement without rewriting it.

Set `timestampColumn` to a non-null native timestamp column in the result, preferably TIMESTAMP WITH TIME ZONE. Timestamp selection prefers an exact column name, otherwise accepts a unique case-insensitive match, and controls checkpointing; it does not rename, remove, or change the output column. No message, severity, identity, or attribute mappings are required.

File and stdout outputs serialize each retrieved row directly as one JSON object. Keys preserve the column names returned by the driver; Oracle normally uppercases unquoted names. Use quoted SQL aliases to choose output names. All selected columns are included, nulls stay null, and message text is preserved, including trailing newlines. No database, stream, source, or alert-specific envelope fields are injected. Include any desired metadata in the query projection or use per-database output files to distinguish databases.

Values use their driver's JSON representation: native timestamps become timestamp strings, binary byte slices become base64 strings, and numbers retain their driver-provided representation. Use explicit SQL conversions when precision or formatting matters. Exact duplicate column names are rejected to prevent overwritten values; names that differ only by case remain distinct. If multiple columns match `timestampColumn` case-insensitively, specify the exact name to select one.

For OTLP, the complete row becomes a structured log body with the same column names. The selected timestamp also populates the OTLP event timestamp. Severity and per-record attributes are left unset; a column called `severity` remains a body field. Null, string, boolean, signed integer, floating-point, binary, and timestamp values are supported. Driver numeric wrappers use their standard SQL value; unsigned integers beyond the signed 64-bit range are sent as decimal strings. Timestamps in the body are strings, and binary values use OTLP's bytes type. Unsupported driver-specific values require an explicit SQL conversion.

Streams use a **single timestamp cursor**, persisted after the complete read range is delivered. There is no overlap or per-record deduplication state. **Records that become visible later with timestamps at or before the saved cursor may be missed.** A primary key allocated before transaction commit does not establish visibility order. Failed partial ranges can replay already delivered batches. Mutable tables, change-data capture, updates/deletes, and snapshot differencing are unsupported.

An omitted databases list selects all configured databases. IDs must be nonempty. Duplicate IDs use the last definition in file order, as with custom metrics. Custom streams can override `alert`; use `log.disable` to disable all default streams while retaining custom streams. Duplicate collection caused by intentionally querying the same data through multiple stream IDs is the operator's responsibility.

Each stream selects one output: file, stdout, or otlp. Streams using `output.type: otlp` share the top-level `otlp` endpoint, timeout, headers, TLS, and resource attributes with metric publishing; stream definitions contain no nested OTLP transport configuration. Multiple destinations per stream are unsupported because independent destinations require independent delivery state. Custom streams use the global log.stateDirectory and keep separate checkpoints per configured database alias and stream ID. Connection changes under the same alias retain progress. Use distinct aliases for distinct sources.
