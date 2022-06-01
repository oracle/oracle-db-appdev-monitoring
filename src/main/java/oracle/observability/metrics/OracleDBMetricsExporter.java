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
        return MetricsHandler.getMetricsString();
    }

    @PostConstruct
    public void init() throws Exception {
        Connection connection = getPoolDataSource().getConnection();
        LOG.debug("OracleDBMetricsExporter DEFAULT_METRICS:" + DEFAULT_METRICS);
        LOG.debug("OracleDBMetricsExporter CUSTOM_METRICS:" + CUSTOM_METRICS); //todo only default metrics are processed currently
        File tomlfile = new File(DEFAULT_METRICS);
        TomlMapper mapper = new TomlMapper();
        JsonNode jsonNode = mapper.readerFor(MetricEntry.class).readTree(new FileInputStream(tomlfile));
        Iterator<JsonNode> metric = jsonNode.get("metric").iterator();
        String context, request, ignorezeroresult;
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
        Iterator<Map.Entry<String, JsonNode>> metricsdescIterator = next.get("metricsdesc").fields();
        String metricsdesc = metricsdescIterator.next().getValue().asText(); // eg metricsdesc = { enqueued_msgs = "Total enqueued messages.", dequeued_msgs = "Total dequeued messages.", remained_msgs = "Total remained messages.", time_since_last_dequeue = "Time since last dequeue.", estd_time_to_drain_no_enq = "Estimated time to drain if no enqueue.", message_latency_1 = "Message latency for last 5 mins.", message_latency_2 = "Message latency for last 1 hour.", message_latency_3 = "Message latency for last 5 hours."}
        LOG.debug("----context:" + context + "----metricsdesc:" + metricsdesc);
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
        ResultSet resultSet = connection.prepareStatement(request).executeQuery();
        while (resultSet.next()) { //should only be one row
            translateQueryToPrometheusMetric(context, metricsdesc, labelNames, resultSet);
        }
    }

    private void translateQueryToPrometheusMetric(String context, String metricsdesc,
                                                  String[] labelNames,
                                                  ResultSet resultSet) throws SQLException {
        String[] labelValues = new String[labelNames.length];
        int columnCount = resultSet.getMetaData().getColumnCount();
        String columnName, columnTypeName;
        Object columnValue;
        Map<String, Double> sqlQueryResults = new HashMap<>();
        for (int i = 0; i < columnCount; i++) { //for each column...
            columnValue =  resultSet.getObject(i + 1);
            columnName = resultSet.getMetaData().getColumnName(i + 1).toLowerCase();
            columnTypeName = resultSet.getMetaData().getColumnTypeName(i + 1);
            //.  typename is 2/NUMBER or 12/VARCHAR2
            if (columnTypeName.equals("VARCHAR2"))
                ; // sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getString(i + 1));
            else
                sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getDouble(i + 1));
            String gaugeName = ORACLEDB_METRIC_PREFIX + context + "_" + columnName;
            LOG.debug("---gaugeName:" + gaugeName);
            Gauge gauge = gaugeMap.get(gaugeName);
            if (gauge == null) {
                if (labelNames.length > 0) {
                    gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsdesc).labelNames(labelNames).register();
                } else gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsdesc).register();
                gaugeMap.put(gaugeName, gauge);
            }
            for (int ii =0 ;ii<labelNames.length;ii++) {
                if(labelNames[ii].equals(columnName)) labelValues[ii] = resultSet.getString(i+1);
            }
        } //by this time labels are set
        Iterator<Map.Entry<String, Double>> entryIterator = sqlQueryResults.entrySet().iterator();
        while(entryIterator.hasNext()) { //for each column
            Map.Entry<String, Double> entry =   entryIterator.next();
            boolean isLabel = false;
            for (int ii =0 ;ii<labelNames.length;ii++) {
                if(labelNames[ii].equals(entry.getKey())) isLabel =true;  // continue
            }
            if(!isLabel) {
                if(labelValues.length >0 )
                    try {
                        gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" + entry.getKey().toLowerCase()).labels(labelValues).set(entry.getValue());
                    } catch (Exception ex) {
                        LOG.debug("OracleDBMetricsExporter.translateQueryToPrometheusMetric labelValues.length:" + labelValues.length);
                        ex.printStackTrace();
                    }
                else gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" + entry.getKey().toLowerCase()).set(entry.getValue());
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
        observabilityDB.setPassword(pw);
        return observabilityDB;
    }
    public void initFromVault() throws Exception {
        boolean isInstancePrincipal = false;
        String regionIdString = "us-ashburn-1";
        String secretOcid = "ocid.whatev";
        LOG.debug("XRResource isInstancePrincipal:" + isInstancePrincipal);
        SecretsClient secretsClient;
        if (isInstancePrincipal) {
            secretsClient = new SecretsClient(InstancePrincipalsAuthenticationDetailsProvider.builder().build());
        } else {
            secretsClient = new SecretsClient(new ConfigFileAuthenticationDetailsProvider("~/.oci/config", "DEFAULT"));
        }
        secretsClient.setRegion(regionIdString);
        GetSecretBundleRequest getSecretBundleRequest = GetSecretBundleRequest
                .builder()
                .secretId(secretOcid)
                .stage(GetSecretBundleRequest.Stage.Current)
                .build();
        LOG.debug("XRResource isInstancePrincipal:" + getSecretBundleRequest);
        GetSecretBundleResponse getSecretBundleResponse = secretsClient.getSecretBundle(getSecretBundleRequest);
        LOG.debug("XRResource isInstancePrincipal:" + getSecretBundleRequest);
        Base64SecretBundleContentDetails base64SecretBundleContentDetails =
                (Base64SecretBundleContentDetails) getSecretBundleResponse.getSecretBundle().getSecretBundleContent();
        LOG.debug("XRResource isInstancePrincipal:" + getSecretBundleRequest);
        byte[] secretValueDecoded = Base64.decodeBase64(base64SecretBundleContentDetails.getContent());
        LOG.debug("XRResource.init:" + new String(secretValueDecoded));
    }
}
