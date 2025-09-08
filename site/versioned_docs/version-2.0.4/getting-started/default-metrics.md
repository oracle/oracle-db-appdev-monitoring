---
title: Default Metrics
sidebar_position: 2
---

# Default Metrics

The exporter includes [default metrics](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/collector/default_metrics.toml) for Oracle Database, and process-specific metrics on the `go` runtime.

You can find the exporter's metric schema in the [Custom Metrics configuration](../configuration/custom-metrics.md#metric-schema).

The following metrics are included by default. The values given are a sample for a single database, "db1":

```bash
# HELP oracledb_activity_execute_count Generic counter metric from gv$sysstat view in Oracle.
# TYPE oracledb_activity_execute_count gauge
oracledb_activity_execute_count{database="db1"} 6.212049e+06
# HELP oracledb_activity_parse_count_total Generic counter metric from gv$sysstat view in Oracle.
# TYPE oracledb_activity_parse_count_total gauge
oracledb_activity_parse_count_total{database="db1"} 1.054178e+06
# HELP oracledb_activity_user_commits Generic counter metric from gv$sysstat view in Oracle.
# TYPE oracledb_activity_user_commits gauge
oracledb_activity_user_commits{database="db1"} 86538
# HELP oracledb_activity_user_rollbacks Generic counter metric from gv$sysstat view in Oracle.
# TYPE oracledb_activity_user_rollbacks gauge
oracledb_activity_user_rollbacks{database="db1"} 18
# HELP oracledb_db_platform_value Database platform
# TYPE oracledb_db_platform_value gauge
oracledb_db_platform_value{database="db1",platform_name="Linux OS (AARCH64)"} 1
# HELP oracledb_db_system_value Database system resources metric
# TYPE oracledb_db_system_value gauge
oracledb_db_system_value{database="db1",name="cpu_count"} 2
oracledb_db_system_value{database="db1",name="pga_aggregate_limit"} 2.147483648e+09
oracledb_db_system_value{database="db1",name="sga_max_size"} 1.610612736e+09
# HELP oracledb_dbtype Type of database the exporter is connected to (0=non-CDB, 1=CDB, >1=PDB).
# TYPE oracledb_dbtype gauge
oracledb_dbtype{database="db1"} 3
# HELP oracledb_exporter_build_info A metric with a constant '1' value labeled by version, revision, branch, goversion from which oracledb_exporter was built, and the goos and goarch for the build.
# TYPE oracledb_exporter_build_info gauge
oracledb_exporter_build_info{branch="",goarch="arm64",goos="darwin",goversion="go1.24.5",revision="unknown",tags="unknown",version=""} 1
# HELP oracledb_exporter_last_scrape_duration_seconds Duration of the last scrape of metrics from Oracle DB.
# TYPE oracledb_exporter_last_scrape_duration_seconds gauge
oracledb_exporter_last_scrape_duration_seconds 0.05714725
# HELP oracledb_exporter_last_scrape_error Whether the last scrape of metrics from Oracle DB resulted in an error (1 for error, 0 for success).
# TYPE oracledb_exporter_last_scrape_error gauge
oracledb_exporter_last_scrape_error 0
# HELP oracledb_exporter_scrapes_total Total number of times Oracle DB was scraped for metrics.
# TYPE oracledb_exporter_scrapes_total counter
oracledb_exporter_scrapes_total 2
# HELP oracledb_process_count Gauge metric with count of processes.
# TYPE oracledb_process_count gauge
oracledb_process_count{database="db1"} 85
# HELP oracledb_sessions_value Gauge metric with count of sessions by status and type.
# TYPE oracledb_sessions_value gauge
oracledb_sessions_value{database="db1",status="ACTIVE",type="BACKGROUND"} 61
oracledb_sessions_value{database="db1",status="ACTIVE",type="USER"} 2
oracledb_sessions_value{database="db1",status="INACTIVE",type="USER"} 19
# HELP oracledb_tablespace_bytes Generic counter metric of tablespaces bytes in Oracle.
# TYPE oracledb_tablespace_bytes gauge
oracledb_tablespace_bytes{database="db1",tablespace="SYSAUX",type="PERMANENT"} 7.7430784e+08
oracledb_tablespace_bytes{database="db1",tablespace="SYSTEM",type="PERMANENT"} 3.18963712e+08
oracledb_tablespace_bytes{database="db1",tablespace="TEMP",type="TEMPORARY"} 7.340032e+06
oracledb_tablespace_bytes{database="db1",tablespace="UNDOTBS1",type="UNDO"} 2.1364736e+07
oracledb_tablespace_bytes{database="db1",tablespace="USERS",type="PERMANENT"} 7.340032e+06
# HELP oracledb_tablespace_free Generic counter metric of tablespaces free bytes in Oracle.
# TYPE oracledb_tablespace_free gauge
oracledb_tablespace_free{database="db1",tablespace="SYSAUX",type="PERMANENT"} 7.5289739264e+10
oracledb_tablespace_free{database="db1",tablespace="SYSTEM",type="PERMANENT"} 7.524491264e+10
oracledb_tablespace_free{database="db1",tablespace="TEMP",type="TEMPORARY"} 1.3631488e+07
oracledb_tablespace_free{database="db1",tablespace="UNDOTBS1",type="UNDO"} 3.518435069952e+13
oracledb_tablespace_free{database="db1",tablespace="USERS",type="PERMANENT"} 3.4352381952e+10
# HELP oracledb_tablespace_max_bytes Generic counter metric of tablespaces max bytes in Oracle.
# TYPE oracledb_tablespace_max_bytes gauge
oracledb_tablespace_max_bytes{database="db1",tablespace="SYSAUX",type="PERMANENT"} 7.6064047104e+10
oracledb_tablespace_max_bytes{database="db1",tablespace="SYSTEM",type="PERMANENT"} 7.5563876352e+10
oracledb_tablespace_max_bytes{database="db1",tablespace="TEMP",type="TEMPORARY"} 2.097152e+07
oracledb_tablespace_max_bytes{database="db1",tablespace="UNDOTBS1",type="UNDO"} 3.5184372064256e+13
oracledb_tablespace_max_bytes{database="db1",tablespace="USERS",type="PERMANENT"} 3.4359721984e+10
# HELP oracledb_tablespace_used_percent Gauge metric showing as a percentage of how much of the tablespace has been used.
# TYPE oracledb_tablespace_used_percent gauge
oracledb_tablespace_used_percent{database="db1",tablespace="SYSAUX",type="PERMANENT"} 1.0179682379262742
oracledb_tablespace_used_percent{database="db1",tablespace="SYSTEM",type="PERMANENT"} 0.4221113677574824
oracledb_tablespace_used_percent{database="db1",tablespace="TEMP",type="TEMPORARY"} 0.35
oracledb_tablespace_used_percent{database="db1",tablespace="UNDOTBS1",type="UNDO"} 6.072223190734319e-05
oracledb_tablespace_used_percent{database="db1",tablespace="USERS",type="PERMANENT"} 0.021362314873845517
# HELP oracledb_top_sql_elapsed SQL statement elapsed time running
# TYPE oracledb_top_sql_elapsed gauge
oracledb_top_sql_elapsed{database="db1",sql_id="0npm6czzaj44m",sql_text="SELECT idx_objn FROM vecsys.vector$index WHERE JSON_VAL"} 6.118614
oracledb_top_sql_elapsed{database="db1",sql_id="0sbbcuruzd66f",sql_text="select /*+ rule */ bucket_cnt, row_cnt, cache_cnt, null"} 1.538687
oracledb_top_sql_elapsed{database="db1",sql_id="121ffmrc95v7g",sql_text="select i.obj#,i.ts#,i.file#,i.block#,i.intcols,i.type#,"} 2.200984
oracledb_top_sql_elapsed{database="db1",sql_id="61znfd8fvgha6",sql_text="SELECT  new.sql_seq, old.plan_hash_value, sqlset_row(ne"} 2.628263
oracledb_top_sql_elapsed{database="db1",sql_id="68dw2nt8wtunk",sql_text="select originating_timestamp, module_id, execution_cont"} 2.296924
oracledb_top_sql_elapsed{database="db1",sql_id="9bd61v53p81sk",sql_text="begin prvt_hdm.auto_execute( :dbid , :inst_num , :end_s"} 1.67611
oracledb_top_sql_elapsed{database="db1",sql_id="aba13jkkk3fts",sql_text="SELECT idx_objn, json_value(IDX_SPARE2, '$.counter') FR"} 3.010397
oracledb_top_sql_elapsed{database="db1",sql_id="afcz0dh295hzp",sql_text=" SELECT /*+ first_rows(1) */ sql_id, force_matching_sig"} 2.246092
oracledb_top_sql_elapsed{database="db1",sql_id="ampw9ddqufjd3",sql_text="begin /*KAPI:capture*/ dbms_auto_index_internal.capture"} 4.102646
oracledb_top_sql_elapsed{database="db1",sql_id="avzy19hxu6gg4",sql_text="SELECT VALUE(P) FROM TABLE(DBMS_SQLTUNE.SELECT_CURSOR_C"} 2.564301
oracledb_top_sql_elapsed{database="db1",sql_id="b39m8n96gxk7c",sql_text="call dbms_autotask_prvt.run_autotask ( :0,:1 )"} 4.418653
oracledb_top_sql_elapsed{database="db1",sql_id="bj9ajtpfh9f41",sql_text=" declare                                    purge_scn  "} 6.425015
oracledb_top_sql_elapsed{database="db1",sql_id="bq819r502v7u2",sql_text="select originating_timestamp, module_id, execution_cont"} 3.676572
oracledb_top_sql_elapsed{database="db1",sql_id="ddrfu7d7hbkym",sql_text=" select count(1), partition_id                         "} 1.870379
oracledb_top_sql_elapsed{database="db1",sql_id="f6w8rqdkx0bnv",sql_text="SELECT * FROM ( SELECT /*+ ordered use_nl(o c cu h) ind"} 1.895947
# HELP oracledb_up Whether the Oracle database server is up.
# TYPE oracledb_up gauge
oracledb_up{database="db1"} 1
# HELP oracledb_wait_time_administrative counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_administrative counter
oracledb_wait_time_administrative{database="db1"} 0
# HELP oracledb_wait_time_application counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_application counter
oracledb_wait_time_application{database="db1"} 0.73
# HELP oracledb_wait_time_commit counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_commit counter
oracledb_wait_time_commit{database="db1"} 0.17
# HELP oracledb_wait_time_concurrency counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_concurrency counter
oracledb_wait_time_concurrency{database="db1"} 6.8
# HELP oracledb_wait_time_configuration counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_configuration counter
oracledb_wait_time_configuration{database="db1"} 19.71
# HELP oracledb_wait_time_network counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_network counter
oracledb_wait_time_network{database="db1"} 0.29
# HELP oracledb_wait_time_other counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_other counter
oracledb_wait_time_other{database="db1"} 6.02
# HELP oracledb_wait_time_scheduler counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_scheduler counter
oracledb_wait_time_scheduler{database="db1"} 4.01
# HELP oracledb_wait_time_system_io counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_system_io counter
oracledb_wait_time_system_io{database="db1"} 0.13
# HELP oracledb_wait_time_user_io counter metric from system_wait_class view in Oracle.
# TYPE oracledb_wait_time_user_io counter
oracledb_wait_time_user_io{database="db1"} 12.38
```