// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
// Portions Copyright (c) 2016 Seth Miller <seth@sethmiller.me>

package collector

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/go-kit/log/level"
)

// needs the const if imported, cannot os.ReadFile in this case
const defaultMetricsConst = `
[[metric]]
context = "sessions"
labels = [ "status", "type" ]
metricsdesc = { value= "Gauge metric with count of sessions by status and type." }
request = "SELECT status, type, COUNT(*) as value FROM v$session GROUP BY status, type"

[[metric]]
context = "resource"
labels = [ "resource_name" ]
metricsdesc = { current_utilization= "Generic counter metric from v$resource_limit view in Oracle (current value).", limit_value="Generic counter metric from v$resource_limit view in Oracle (UNLIMITED: -1)." }
request = '''
SELECT resource_name, current_utilization, CASE WHEN TRIM(limit_value) LIKE 'UNLIMITED' THEN '-1' ELSE TRIM(limit_value) END as limit_value 
FROM v$resource_limit
'''
ignorezeroresult = true

[[metric]]
context = "asm_diskgroup"
labels = [ "name" ]
metricsdesc = { total = "Total size of ASM disk group.", free = "Free space available on ASM disk group." }
request = "SELECT name,total_mb*1024*1024 as total,free_mb*1024*1024 as free FROM v$asm_diskgroup_stat where exists (select 1 from v$datafile where name like '+%')"
ignorezeroresult = true

[[metric]]
context = "activity"
metricsdesc = { value="Generic counter metric from v$sysstat view in Oracle." }
fieldtoappend = "name"
request = "SELECT name, value FROM v$sysstat WHERE name IN ('parse count (total)', 'execute count', 'user commits', 'user rollbacks')"

[[metric]]
context = "process"
metricsdesc = { count="Gauge metric with count of processes." }
request = "SELECT COUNT(*) as count FROM v$process"

[[metric]]
context = "wait_time"
metricsdesc = { value="Generic counter metric from v$waitclassmetric view in Oracle." }
fieldtoappend= "wait_class"
request = '''
SELECT wait_class as WAIT_CLASS, sum(time_waited) as VALUE
FROM gv$active_session_history 
where wait_class is not null 
and sample_time > sysdate - interval '1' hour
GROUP BY wait_class
'''
ignorezeroresult = true

[[metric]]
context = "tablespace"
labels = [ "tablespace", "type" ]
metricsdesc = { bytes = "Generic counter metric of tablespaces bytes in Oracle.", max_bytes = "Generic counter metric of tablespaces max bytes in Oracle.", free = "Generic counter metric of tablespaces free bytes in Oracle.", used_percent = "Gauge metric showing as a percentage of how much of the tablespace has been used." }
request = '''
SELECT
    dt.tablespace_name as tablespace,
    dt.contents as type,
    dt.block_size * dtum.used_space as bytes,
    dt.block_size * dtum.tablespace_size as max_bytes,
    dt.block_size * (dtum.tablespace_size - dtum.used_space) as free,
    dtum.used_percent
FROM  dba_tablespace_usage_metrics dtum, dba_tablespaces dt
WHERE dtum.tablespace_name = dt.tablespace_name
ORDER by tablespace
'''

[[metric]]
context = "db_system"
labels = [ "name" ]
metricsdesc = { value = "Database system resources metric" }
request = '''
select name, value 
from v$parameter 
where name in ('cpu_count', 'sga_max_size', 'pga_aggregate_limit')
'''

[[metric]]
context = "db_platform"
labels = [ "platform_name" ]
metricsdesc = { value = "Database platform" }
request = '''
SELECT platform_name, 1 as value FROM v$database
'''

[[metric]]
context = "top_sql"
labels = [ "sql_id", "sql_text" ]
metricsdesc = { elapsed = "SQL statement elapsed time running" }
request = '''
select * from (
select sql_id, elapsed_time / 1000000 as elapsed, SUBSTRB(REPLACE(sql_text,'',' '),1,55) as sql_text
from   V$SQLSTATS
order by elapsed_time desc
) where ROWNUM <= 15
'''
ignorezeroresult = true
`

// DefaultMetrics is a somewhat hacky way to load the default metrics
func (e *Exporter) DefaultMetrics() Metrics {
	var metricsToScrape Metrics
	if e.config.DefaultMetricsFile != "" {
		if _, err := toml.DecodeFile(filepath.Clean(e.config.DefaultMetricsFile), &metricsToScrape); err != nil {
			level.Error(e.logger).Log("msg", fmt.Sprintf("there was an issue while loading specified default metrics file at: "+e.config.DefaultMetricsFile+", proceeding to run with default metrics."),
				"error", err)
		}
		return metricsToScrape
	}

	if _, err := toml.Decode(defaultMetricsConst, &metricsToScrape); err != nil {
		level.Error(e.logger).Log(err)
		panic(errors.New("Error while loading " + defaultMetricsConst))
	}
	return metricsToScrape
}
