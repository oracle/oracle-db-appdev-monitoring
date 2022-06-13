package oracle.observability.metrics;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import io.prometheus.client.Gauge;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import com.oracle.bmc.auth.ConfigFileAuthenticationDetailsProvider;
import com.oracle.bmc.auth.InstancePrincipalsAuthenticationDetailsProvider;
import com.oracle.bmc.secrets.SecretsClient;
import com.oracle.bmc.secrets.model.Base64SecretBundleContentDetails;
import com.oracle.bmc.secrets.requests.GetSecretBundleRequest;
import com.oracle.bmc.secrets.responses.GetSecretBundleResponse;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.apache.commons.codec.binary.Base64;

import javax.annotation.PostConstruct;
import java.io.File;
import java.io.FileInputStream;
import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.*;
import org.json.*;

@RestController
public class OracleDBMetricsExporter extends OracleDBMetricsExporterEnvConfig {

    public static final String ORACLEDB_METRIC_PREFIX = "oracledb_";
    Map<String, Gauge> gaugeMap = new HashMap<>();
    PoolDataSource observabilityDB;

    private static final Logger LOG = LoggerFactory.getLogger(OracleDBMetricsExporter.class);

    /**
     * The endpoint that prometheus will scrape
     * @return Prometheus metric
     * @throws Exception
     */
    @GetMapping("/metrics")
    public String metrics() throws Exception {
        System.out.println("OracleDBMetricsExporter.metrics just returning metricsString...");
        return getMetricsStaticString();
//        return MetricsHandler.getMetricsString();
    }

 //   @PostConstruct
    public void init() throws Exception {
        Connection connection = getPoolDataSource().getConnection();
        LOG.debug("Successfully loaded default metrics from:" + DEFAULT_METRICS);
        LOG.debug("OracleDBMetricsExporter CUSTOM_METRICS:" + CUSTOM_METRICS); //todo only default metrics are processed currently
        File tomlfile = new File(DEFAULT_METRICS);
        TomlMapper mapper = new TomlMapper();
        JsonNode jsonNode = mapper.readerFor(MetricEntry.class).readTree(new FileInputStream(tomlfile));
        Iterator<JsonNode> metric = jsonNode.get("metric").iterator();
        while (metric.hasNext()) {
            processMetric(connection, metric);
        }
    }
    
    /**
     * Process Metric config, issue SQL query, and translate into Prometheus format for publish at /metric
     * Context          string
     * Labels           []string
     * MetricsDesc      map[string]string
     * MetricsType      map[string]string
     * MetricsBuckets   map[string]map[string]string
     * FieldToAppend    string
     * Request          string
     * IgnoreZeroResult bool
     */
    private void processMetric(Connection connection, Iterator<JsonNode> metric) throws SQLException {
        String ignorezeroresult;
        String context;
        String metricsType;
        String request;
        JsonNode next = metric.next();
        //todo ignore case
        context = next.get("context").asText(); // eg context = "teq"
        metricsType = next.get("metricstype") == null ? "" :next.get("metricstype").asText(); // gauge, counter, histogram, or unspecified
        JsonNode metricsdescNode = next.get("metricsdesc");
        Iterator<Map.Entry<String, JsonNode>> metricsdescIterator = metricsdescNode.fields();
        Map<String, String> metricsDescMap = new HashMap<>();
        while(metricsdescIterator.hasNext()) {
            Map.Entry<String, JsonNode> metricsdesc = metricsdescIterator.next();
            metricsDescMap.put(metricsdesc.getKey(), metricsdesc.getValue().asText());
        }
        // eg metricsdesc = { enqueued_msgs = "Total enqueued messages.", dequeued_msgs = "Total dequeued messages.", remained_msgs = "Total remained messages."}
        LOG.debug("----context:" + context);
        String[] labelNames = new String[0];
        if (next.get("labels") != null) {
            int size = next.get("labels").size();
            Iterator<JsonNode> labelIterator = next.get("labels").iterator();
            labelNames = new String[size];
            for (int i = 0; i < size; i++) {
                labelNames[i] = labelIterator.next().asText();
            }
            LOG.debug("\n");
        }
        request = next.get("request").asText(); // the sql query
        ignorezeroresult = next.get("ignorezeroresult") == null ? "false" : next.get("ignorezeroresult").asText();
        ResultSet resultSet;
        try {
             resultSet = connection.prepareStatement(request).executeQuery();
        } catch(SQLException e) { //this can be due to table not existing etc.
            LOG.debug("OracleDBMetricsExporter.processMetric  during:" + request);
            LOG.debug("OracleDBMetricsExporter.processMetric  exception:" + e);
            return;
        }
        while (resultSet.next()) { //should only be one row
            translateQueryToPrometheusMetric(context,  metricsDescMap, labelNames, resultSet);
        }
    }

    private void translateQueryToPrometheusMetric(String context, Map<String, String> metricsDescMap,
                                                  String[] labelNames,
                                                  ResultSet resultSet) throws SQLException {
        String[] labelValues = new String[labelNames.length];
        int columnCount = resultSet.getMetaData().getColumnCount();
        String columnName, columnTypeName;
        Map<String, Integer> sqlQueryResults = new HashMap<>();
        for (int i = 0; i < columnCount; i++) { //for each column...
         //   columnValue =  resultSet.getObject(i + 1);
            columnName = resultSet.getMetaData().getColumnName(i + 1).toLowerCase();
            columnTypeName = resultSet.getMetaData().getColumnTypeName(i + 1);
            //.  typename is 2/NUMBER or 12/VARCHAR2
            if (columnTypeName.equals("VARCHAR2"))
                ; // sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getString(i + 1));
            else
                sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getInt(i + 1));
            String gaugeName = ORACLEDB_METRIC_PREFIX + context + "_" + columnName;
            LOG.debug("---gaugeName:" + gaugeName);
            Gauge gauge = gaugeMap.get(gaugeName);
            if (gauge == null) { //todo this is creating gauge for every field, should be gauge for every field that is in metricsdesc
                // each value in metricsdesc gets a label
                if(metricsDescMap.containsKey(columnName)) {
                    if (labelNames.length > 0) {
                        gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsDescMap.get(columnName)).labelNames(labelNames).register();
                    } else gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsDescMap.get(columnName)).register();
                    gaugeMap.put(gaugeName, gauge);
                }
            }
            for (int ii =0 ;ii<labelNames.length;ii++) {
                if(labelNames[ii].equals(columnName)) labelValues[ii] = resultSet.getString(i+1);
            }
        } //by this time labels are set
        Iterator<Map.Entry<String, Integer>> sqlQueryRestulsEntryIterator = sqlQueryResults.entrySet().iterator();
        while(sqlQueryRestulsEntryIterator.hasNext()) { //for each column
            Map.Entry<String, Integer> sqlQueryResultsEntry =   sqlQueryRestulsEntryIterator.next();
            boolean isLabel = false;
            for (int ii =0 ;ii<labelNames.length;ii++) {
                if(labelNames[ii].equals(sqlQueryResultsEntry.getKey())) isLabel =true;  // continue
            }
            if(!isLabel) {
                int valueToSet = (int) Math.rint(sqlQueryResultsEntry.getValue().intValue());
                if(labelValues.length >0 )
                    try {
                        System.out.println("~~~~~~~~~~~~~~~~~~~~~~~~");
                        System.out.println("labelNames"+Arrays.toString(labelNames));
                        System.out.println("labelValues"+Arrays.toString(labelValues));
                        System.out.println("valueToSet:"+valueToSet);
                        System.out.println("~~~~~~~~~~~~~~~~~~~~~~~~");
                        gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" + sqlQueryResultsEntry.getKey().toLowerCase()).labels(labelValues).set(valueToSet);
                    } catch (Exception ex) {
                        //todo gate the get above as is done with if(metricsDescMap.containsKey(columnName)) previously
                        LOG.error("OracleDBMetricsExporter.translateQueryToPrometheusMetric Exc:" + labelValues.length);
                        ex.printStackTrace();
                    }
                else gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" + sqlQueryResultsEntry.getKey().toLowerCase()).set(valueToSet);
            }
        }
    }

    private PoolDataSource getPoolDataSource() throws Exception {
        if (observabilityDB != null) return observabilityDB;
        observabilityDB = PoolDataSourceFactory.getPoolDataSource();
        observabilityDB.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
        String user = DATA_SOURCE_NAME.substring(0, DATA_SOURCE_NAME.indexOf("/"));
        String pw = DATA_SOURCE_NAME.substring(DATA_SOURCE_NAME.indexOf("/") + 1, DATA_SOURCE_NAME.indexOf("@"));
        String serviceName = DATA_SOURCE_NAME.substring(DATA_SOURCE_NAME.indexOf("@") + 1);
        String url = "jdbc:oracle:thin:@" + serviceName + "?TNS_ADMIN=" + TNS_ADMIN;
        observabilityDB.setURL(url);
        observabilityDB.setUser(user);
        if (VAULT_SECRET_OCID == null || VAULT_SECRET_OCID.trim().equals("")) {
            observabilityDB.setPassword(pw);
        } else {
            observabilityDB.setPassword(getPasswordFromVault());
        }
        return observabilityDB;
    }
    
    
    public String getPasswordFromVault() throws Exception {
        SecretsClient secretsClient;
        if (OCI_CONFIG_FILE == null || OCI_CONFIG_FILE.trim().equals("")) {
            secretsClient = new SecretsClient(InstancePrincipalsAuthenticationDetailsProvider.builder().build());
        } else {
            secretsClient = new SecretsClient(new ConfigFileAuthenticationDetailsProvider(OCI_CONFIG_FILE, "DEFAULT")); //todo allow profile override as well
        }
        secretsClient.setRegion(OCI_REGION);
        GetSecretBundleRequest getSecretBundleRequest = GetSecretBundleRequest
                .builder()
                .secretId(VAULT_SECRET_OCID)
                .stage(GetSecretBundleRequest.Stage.Current)
                .build();
        GetSecretBundleResponse getSecretBundleResponse = secretsClient.getSecretBundle(getSecretBundleRequest);
        Base64SecretBundleContentDetails base64SecretBundleContentDetails =
                (Base64SecretBundleContentDetails) getSecretBundleResponse.getSecretBundle().getSecretBundleContent();
        byte[] secretValueDecoded = Base64.decodeBase64(base64SecretBundleContentDetails.getContent());
        return new String(secretValueDecoded);
    }

    int metricCounter = 1;
    String getMetricsStaticString(){
        return "# HELP oracledb_context_no_label_value_1 Simple example returning always 1.\n" +
                "# TYPE oracledb_context_no_label_value_1 gauge\n" +
                "oracledb_context_no_label_value_1 " + (metricCounter++) + "\n" +
                "# HELP oracledb_context_no_label_value_2 Same but returning always 2.\n" +
                "# TYPE oracledb_context_no_label_value_2 gauge\n" +
                "oracledb_context_no_label_value_2 2\n" +
                "# HELP oracledb_context_with_labels_value_1 Simple example returning always 1.\n" +
                "# TYPE oracledb_context_with_labels_value_1 gauge\n" +
                "oracledb_context_with_labels_value_1{label_1=\"First label\",label_2=\"Second label\"} 1\n" +
                "# HELP oracledb_context_with_labels_value_2 Same but returning always 2.\n" +
                "# TYPE oracledb_context_with_labels_value_2 gauge\n" +
                "oracledb_context_with_labels_value_2{label_1=\"First label\",label_2=\"Second label\"} 2";
    }
    String metricsString = "# HELP oracledb_context_no_label_value_1 Simple example returning always 1.\n" +
            "# TYPE oracledb_context_no_label_value_1 gauge\n" +
            "oracledb_context_no_label_value_1 " + (metricCounter++) + "\n" +
            "# HELP oracledb_context_no_label_value_2 Same but returning always 2.\n" +
            "# TYPE oracledb_context_no_label_value_2 gauge\n" +
            "oracledb_context_no_label_value_2 2\n" +
            "# HELP oracledb_context_with_labels_value_1 Simple example returning always 1.\n" +
            "# TYPE oracledb_context_with_labels_value_1 gauge\n" +
            "oracledb_context_with_labels_value_1{label_1=\"First label\",label_2=\"Second label\"} 1\n" +
            "# HELP oracledb_context_with_labels_value_2 Same but returning always 2.\n" +
            "# TYPE oracledb_context_with_labels_value_2 gauge\n" +
            "oracledb_context_with_labels_value_2{label_1=\"First label\",label_2=\"Second label\"} 2";
    String metricsString0 =
            "# HELP go_gc_duration_seconds A summary of the GC invocation durations.\n" +
            "# TYPE go_gc_duration_seconds summary\n" +
            "go_gc_duration_seconds{quantile=\"0\"} 4.805e-05\n" +
            "go_gc_duration_seconds{quantile=\"0.25\"} 4.805e-05\n" +
            "go_gc_duration_seconds{quantile=\"0.5\"} 4.805e-05\n" +
            "go_gc_duration_seconds{quantile=\"0.75\"} 4.805e-05\n" +
            "go_gc_duration_seconds{quantile=\"1\"} 4.805e-05\n" +
            "go_gc_duration_seconds_sum 4.805e-05\n" +
            "go_gc_duration_seconds_count 1\n" +
            "# HELP go_goroutines Number of goroutines that currently exist.\n" +
            "# TYPE go_goroutines gauge\n" +
            "go_goroutines 10\n" +
            "# HELP go_info Information about the Go environment.\n" +
            "# TYPE go_info gauge\n" +
            "go_info{version=\"go1.14.15\"} 1\n" +
            "# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.\n" +
            "# TYPE go_memstats_alloc_bytes gauge\n" +
            "go_memstats_alloc_bytes 2.180736e+06\n" +
            "# HELP go_memstats_alloc_bytes_total Total number of bytes allocated, even if freed.\n" +
            "# TYPE go_memstats_alloc_bytes_total counter\n" +
            "go_memstats_alloc_bytes_total 4.124512e+06\n" +
            "# HELP go_memstats_buck_hash_sys_bytes Number of bytes used by the profiling bucket hash table.\n" +
            "# TYPE go_memstats_buck_hash_sys_bytes gauge\n" +
            "go_memstats_buck_hash_sys_bytes 1.444226e+06\n" +
            "# HELP go_memstats_frees_total Total number of frees.\n" +
            "# TYPE go_memstats_frees_total counter\n" +
            "go_memstats_frees_total 19635\n" +
            "# HELP go_memstats_gc_cpu_fraction The fraction of this program's available CPU time used by the GC since the program started.\n" +
            "# TYPE go_memstats_gc_cpu_fraction gauge\n" +
            "go_memstats_gc_cpu_fraction 1.4771481870838605e-06\n" +
            "# HELP go_memstats_gc_sys_bytes Number of bytes used for garbage collection system metadata.\n" +
            "# TYPE go_memstats_gc_sys_bytes gauge\n" +
            "go_memstats_gc_sys_bytes 3.508488e+06\n" +
            "# HELP go_memstats_heap_alloc_bytes Number of heap bytes allocated and still in use.\n" +
            "# TYPE go_memstats_heap_alloc_bytes gauge\n" +
            "go_memstats_heap_alloc_bytes 2.180736e+06\n" +
            "# HELP go_memstats_heap_idle_bytes Number of heap bytes waiting to be used.\n" +
            "# TYPE go_memstats_heap_idle_bytes gauge\n" +
            "go_memstats_heap_idle_bytes 6.3307776e+07\n" +
            "# HELP go_memstats_heap_inuse_bytes Number of heap bytes that are in use.\n" +
            "# TYPE go_memstats_heap_inuse_bytes gauge\n" +
            "go_memstats_heap_inuse_bytes 3.244032e+06\n" +
            "# HELP go_memstats_heap_objects Number of allocated objects.\n" +
            "# TYPE go_memstats_heap_objects gauge\n" +
            "go_memstats_heap_objects 2721\n" +
            "# HELP go_memstats_heap_released_bytes Number of heap bytes released to OS.\n" +
            "# TYPE go_memstats_heap_released_bytes gauge\n" +
            "go_memstats_heap_released_bytes 6.2357504e+07\n" +
            "# HELP go_memstats_heap_sys_bytes Number of heap bytes obtained from system.\n" +
            "# TYPE go_memstats_heap_sys_bytes gauge\n" +
            "go_memstats_heap_sys_bytes 6.6551808e+07\n" +
            "# HELP go_memstats_last_gc_time_seconds Number of seconds since 1970 of last garbage collection.\n" +
            "# TYPE go_memstats_last_gc_time_seconds gauge\n" +
            "go_memstats_last_gc_time_seconds 1.6550506320875754e+09\n" +
            "# HELP go_memstats_lookups_total Total number of pointer lookups.\n" +
            "# TYPE go_memstats_lookups_total counter\n" +
            "go_memstats_lookups_total 0\n" +
            "# HELP go_memstats_mallocs_total Total number of mallocs.\n" +
            "# TYPE go_memstats_mallocs_total counter\n" +
            "go_memstats_mallocs_total 22356\n" +
            "# HELP go_memstats_mcache_inuse_bytes Number of bytes in use by mcache structures.\n" +
            "# TYPE go_memstats_mcache_inuse_bytes gauge\n" +
            "go_memstats_mcache_inuse_bytes 3472\n" +
            "# HELP go_memstats_mcache_sys_bytes Number of bytes used for mcache structures obtained from system.\n" +
            "# TYPE go_memstats_mcache_sys_bytes gauge\n" +
            "go_memstats_mcache_sys_bytes 16384\n" +
            "# HELP go_memstats_mspan_inuse_bytes Number of bytes in use by mspan structures.\n" +
            "# TYPE go_memstats_mspan_inuse_bytes gauge\n" +
            "go_memstats_mspan_inuse_bytes 44472\n" +
            "# HELP go_memstats_mspan_sys_bytes Number of bytes used for mspan structures obtained from system.\n" +
            "# TYPE go_memstats_mspan_sys_bytes gauge\n" +
            "go_memstats_mspan_sys_bytes 49152\n" +
            "# HELP go_memstats_next_gc_bytes Number of heap bytes when next garbage collection will take place.\n" +
            "# TYPE go_memstats_next_gc_bytes gauge\n" +
            "go_memstats_next_gc_bytes 4.194304e+06\n" +
            "# HELP go_memstats_other_sys_bytes Number of bytes used for other system allocations.\n" +
            "# TYPE go_memstats_other_sys_bytes gauge\n" +
            "go_memstats_other_sys_bytes 765558\n" +
            "# HELP go_memstats_stack_inuse_bytes Number of bytes in use by the stack allocator.\n" +
            "# TYPE go_memstats_stack_inuse_bytes gauge\n" +
            "go_memstats_stack_inuse_bytes 557056\n" +
            "# HELP go_memstats_stack_sys_bytes Number of bytes obtained from system for stack allocator.\n" +
            "# TYPE go_memstats_stack_sys_bytes gauge\n" +
            "go_memstats_stack_sys_bytes 557056\n" +
            "# HELP go_memstats_sys_bytes Number of bytes obtained from system.\n" +
            "# TYPE go_memstats_sys_bytes gauge\n" +
            "go_memstats_sys_bytes 7.2892672e+07\n" +
            "# HELP go_threads Number of OS threads created.\n" +
            "# TYPE go_threads gauge\n" +
            "go_threads 10\n" +
            "# HELP oracledb_exporter_last_scrape_duration_seconds Duration of the last scrape of metrics from Oracle DB.\n" +
            "# TYPE oracledb_exporter_last_scrape_duration_seconds gauge\n" +
            "oracledb_exporter_last_scrape_duration_seconds 1.412728837\n" +
            "# HELP oracledb_exporter_last_scrape_error Whether the last scrape of metrics from Oracle DB resulted in an error (1 for error, 0 for success).\n" +
            "# TYPE oracledb_exporter_last_scrape_error gauge\n" +
            "oracledb_exporter_last_scrape_error 0\n" +
            "# HELP oracledb_exporter_scrapes_total Total number of times Oracle DB was scraped for metrics.\n" +
            "# TYPE oracledb_exporter_scrapes_total counter\n" +
            "oracledb_exporter_scrapes_total 13\n" +
            "# HELP oracledb_orderpdb_orders_by_status_value Total number of orders by status\n" +
            "# TYPE oracledb_orderpdb_orders_by_status_value gauge\n" +
            "oracledb_orderpdb_orders_by_status_value{status=\"success inventory exists\"} 1\n" +
            "# HELP oracledb_orderpdb_propagation_disabled_status_value Total number of propagation schedules disabled by queue name\n" +
            "# TYPE oracledb_orderpdb_propagation_disabled_status_value gauge\n" +
            "oracledb_orderpdb_propagation_disabled_status_value{value=\"1\"} 1\n" +
            "# HELP oracledb_orderpdb_sessions_value Gauge metric with count of orderpdb sessions by status and type.\n" +
            "# TYPE oracledb_orderpdb_sessions_value gauge\n" +
            "oracledb_orderpdb_sessions_value{inst_id=\"6\",status=\"ACTIVE\",type=\"USER\"} 6\n" +
            "oracledb_orderpdb_sessions_value{inst_id=\"6\",status=\"INACTIVE\",type=\"USER\"} 2\n" +
            "# HELP oracledb_up Whether the Oracle database server is up.\n" +
            "# TYPE oracledb_up gauge\n" +
            "oracledb_up 1\n" +
            "# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.\n" +
            "# TYPE process_cpu_seconds_total counter\n" +
            "process_cpu_seconds_total 3.49\n" +
            "# HELP process_max_fds Maximum number of open file descriptors.\n" +
            "# TYPE process_max_fds gauge\n" +
            "process_max_fds 1.048576e+06\n" +
            "# HELP process_open_fds Number of open file descriptors.\n" +
            "# TYPE process_open_fds gauge\n" +
            "process_open_fds 11\n" +
            "# HELP process_resident_memory_bytes Resident memory size in bytes.\n" +
            "# TYPE process_resident_memory_bytes gauge\n" +
            "process_resident_memory_bytes 5.5693312e+07\n" +
            "# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.\n" +
            "# TYPE process_start_time_seconds gauge\n" +
            "process_start_time_seconds 1.65505043913e+09\n" +
            "# HELP process_virtual_memory_bytes Virtual memory size in bytes.\n" +
            "# TYPE process_virtual_memory_bytes gauge\n" +
            "process_virtual_memory_bytes 1.589772288e+09\n" +
            "# HELP process_virtual_memory_max_bytes Maximum amount of virtual memory available in bytes.\n" +
            "# TYPE process_virtual_memory_max_bytes gauge\n";
}
