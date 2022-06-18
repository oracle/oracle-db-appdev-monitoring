## Tracing Exporter

###Tracing export entries contain the following elements:

- `context`: Overall context of tracing entry
- `logdesc`: Overall description of tracing entry
- `traceidfield`: Field used to indicate which field in the query contains the trace/spancontext id
- `request: The query used to retrieve tracing information. Values are tagged to a tracespan in the format `[fieldname]=[fieldvalue]`
- `template: Template request/query that exist for common/base cases. May be used as is, in which case, `request` and `traceidfield` values are not required.

The following is an example entry.

```toml
[[trace]]
context = "orderdb_tracing"
tracingdesc = { value = "Trace including sqltext with bind values of all sessions by orderuser"}
traceidfield = "ECID"
template = "ECID_BIND_VALUES"
request = "select ECID, SQL_ID from GV$SESSION where ECID IS NOT NULL"
```