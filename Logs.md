## Logs Exporter

###Log export entries contain the following elements:

- `context`: Overall context of log entry
- `logdesc`: Overall description of log entry
- `timestampfield`: Field used to filter the time range for queries logged
- `request`: The query used to retrieve log information. Values are logged in the format `[fieldname]=[fieldvalue]`

The following is an example entry.

```toml
[[log]]
context = "orderpdb_alertlogs"
logdesc = "alert logs for order PDB"
timestampfield = "ORIGINATING_TIMESTAMP"
request = "select ORIGINATING_TIMESTAMP, MODULE_ID, EXECUTION_CONTEXT_ID, MESSAGE_TEXT from V$diag_alert_ext"
```
