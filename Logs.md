
[[log]]
context = "orderpdb_alertlogs"
logdesc = "alert logs for order PDB"
timestampfield = "ORIGINATING_TIMESTAMP"
request = "select ORIGINATING_TIMESTAMP, MODULE_ID, EXECUTION_CONTEXT_ID, MESSAGE_TEXT from V$diag_alert_ext"

