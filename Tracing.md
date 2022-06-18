
[[trace]]
context = "orderdb_tracing"
tracingdesc = { value = "Trace including sqltext with bind values of all sessions by orderuser"}
traceidfield = "ECID"
template = "ECID_BIND_VALUES"
request = "select ECID, SQL_ID from GV$SESSION where ECID IS NOT NULL"