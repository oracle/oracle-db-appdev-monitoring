---
title: Compatibility with Older Database Versions
sidebar_position: 4
---

# Older Database Versions

In general, fixes and features are not provided for older database versions. However, it is possible to configure the Oracle Database Metrics Exporter to scrape older versions of Oracle Database.

### Known Issues with Older Database Versions

If you are running an unsupported version of Oracle Database, you may encounter the following issues:

- Metrics using modern SQL syntax may not work. For compatibility, you can disable or modify these metrics.
- The exporter uses a "thick" database client. Ensure your database client libraries are compatible with your database version.

## Disabling incompatible metrics

To disable an incompatible metric, either remove that metric from the metrics file or configure the metric so it does not apply the affected database:

```toml
[[metric]]
context = "process"
labels = [ "inst_id" ]
metricsdesc = { count="Gauge metric with count of processes." }
request = '''
select inst_id, count(*) as count
from gv$process
group by inst_id
'''
# Set databases to an empty array to disable the metric entirely,
# or include only compatible databases in this array.
databases = []
```
