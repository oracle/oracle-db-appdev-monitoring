package oracle.observability.metrics;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import io.prometheus.client.Collector;
import io.prometheus.client.CollectorRegistry;
import io.prometheus.client.Gauge;
import oracle.observability.DataSourceConfig;
import oracle.observability.ObservabilityExporter;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.yaml.snakeyaml.Yaml;

import javax.annotation.PostConstruct;
import java.io.*;
import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.*;

@RestController
public class MetricsExporter extends ObservabilityExporter {

    private static final Logger LOGGER = LoggerFactory.getLogger(MetricsExporter.class);
    public static final String UP = "up";
    public static final String METRICSTYPE = "metricstype";
    public static final String METRICSDESC = "metricsdesc";
    public static final String LABELS = "labels";
    public static final String IGNOREZERORESULT = "ignorezeroresult";
    public static final String FALSE = "false";
    public static final String ORACLEDB_METRIC_PREFIX = "oracledb_";

    //This map is used for multi-datasource scraping, both when using dns target string and config
    Map<String, CollectorRegistryWithGaugeMap> dnsToCollectorRegistryMap = new HashMap<>();

    CollectorRegistryWithGaugeMap defaultRegistry = new CollectorRegistryWithGaugeMap();
    /**
     * The endpoint that prometheus will scrape
     *
     * @return Prometheus metric
     */
    @GetMapping(value = "/metrics", produces = "text/plain")
    public String metrics() throws Exception {
        processMetrics(DATA_SOURCE_NAME, defaultRegistry, false);
        return getMetricsString(CollectorRegistry.defaultRegistry);
    }

    @GetMapping(value = "/scrape", produces = "text/plain")
    public String scrape(@RequestParam("target") String target) throws Exception {
        CollectorRegistryWithGaugeMap collectorRegistry = dnsToCollectorRegistryMap.get(target);
        if (collectorRegistry == null) {
            collectorRegistry = new CollectorRegistryWithGaugeMap();
            dnsToCollectorRegistryMap.put(target, collectorRegistry);
        }
        processMetrics(target, dnsToCollectorRegistryMap.get(target), false);
        return getMetricsString(collectorRegistry);
    }

    @GetMapping(value = "/scrapeByName", produces = "text/plain")
    public String scrapeByConfigName(@RequestParam("name") String name) throws Exception {
        CollectorRegistryWithGaugeMap collectorRegistry = dnsToCollectorRegistryMap.get(name);
        if (collectorRegistry == null) {
            collectorRegistry = new CollectorRegistryWithGaugeMap();
            dnsToCollectorRegistryMap.put(name, collectorRegistry);
        }
        processMetrics(name, dnsToCollectorRegistryMap.get(name), true);
        return getMetricsString(collectorRegistry);
    }

    @PostConstruct
    public void init() throws Exception {
        boolean isGlobalDataSourceSpecified = DATA_SOURCE_NAME != null && !DATA_SOURCE_NAME.trim().equals("");
        boolean isMultiDataSourceConfigSpecified = MULTI_DATASOURCE_CONFIG != null || !MULTI_DATASOURCE_CONFIG.trim().equals("");
        if (!isMultiDataSourceConfigSpecified && !isGlobalDataSourceSpecified)
            throw new Exception(
                    "Neither global datasource (DATA_SOURCE_NAME) nor multi-datasource (MULTI_DATASOURCE_CONFIG) " +
                            "specified. One or both are required.");
        if (isMultiDataSourceConfigSpecified) parseMultiDataSourceConfig();
        if (isGlobalDataSourceSpecified) processMetrics(DATA_SOURCE_NAME, defaultRegistry, false);
    }

    //Currently this is only supported for metrics and so is called from here
    //If/when it is supported by other exporters it should be moved to common/Observability exporter
    //Failure to find file, if specified, results in exit
    public void parseMultiDataSourceConfig() throws FileNotFoundException {
        File file = new File(MULTI_DATASOURCE_CONFIG);
        InputStream inputStream = new FileInputStream(file);
        Yaml yaml = new Yaml();
        Map<String, Map<String, String>> config = yaml.load(inputStream);
        for (Map.Entry<String, Map<String, String>> entry : config.entrySet()) {
            DataSourceConfig dataSourceConfigForMap = new DataSourceConfig();
            String dataSourceName = entry.getKey();
            Map<String, String> dataSourceConfig = entry.getValue();
            dataSourceConfigForMap.setDataSourceName(dataSourceName); //the key is also part of the config for convenience
            dataSourceConfigForMap.setServiceName(dataSourceConfig.get(SERVICE_NAME_STRING));
            dataSourceConfigForMap.setUserName(dataSourceConfig.get(USER_NAME_STRING));
            dataSourceConfigForMap.setPassword(dataSourceConfig.get(PASSWORD_STRING));
            dataSourceConfigForMap.setTNS_ADMIN(dataSourceConfig.get(TNS_ADMIN_STRING));
            dataSourceConfigForMap.setPasswordOCID(dataSourceConfig.get(PASSWORD_OCID_STRING));
            dataSourceConfigForMap.setOciRegion(dataSourceConfig.get(OCI_CONFIG_FILE_STRING));
            dataSourceConfigForMap.setOciRegion(dataSourceConfig.get(OCI_REGION_STRING));
            dataSourceConfigForMap.setOciProfile(dataSourceConfig.get(OCI_PROFILE_STRING));
            LOGGER.info("adding dataSource from config:" + dataSourceName);
            dataSourceNameToDataSourceConfigMap.put(dataSourceName, dataSourceConfigForMap);
        }
    }

    private void processMetrics(String datasourceName, CollectorRegistryWithGaugeMap registry, boolean isScrapeByName) throws IOException, SQLException {
        if (DEFAULT_METRICS == null || DEFAULT_METRICS.trim().equals(""))
            throw new FileNotFoundException("DEFAULT_METRICS file location must be specified");
        File tomlfile = new File(DEFAULT_METRICS);
        TomlMapper mapper = new TomlMapper();
        JsonNode jsonNode = mapper.readerFor(MetricsExporterConfigEntry.class).readTree(new FileInputStream(tomlfile));
        JsonNode metric = jsonNode.get("metric");
        if (metric == null || metric.isEmpty()) {
            LOGGER.info("No logs records configured");
            return;
        }
        Iterator<JsonNode> metrics = metric.iterator();
        int isConnectionSuccessful = 0;
        try (Connection connection = getPoolDataSource(datasourceName, isScrapeByName).getConnection()) {
            isConnectionSuccessful = 1;
            while (metrics.hasNext()) {
                processMetric(registry, connection, metrics);
            }
        } finally { //always set the db health/up metric - if a connection is unobtainable the metric is set to down
            Gauge gauge = registry.gaugeMap.get(ORACLEDB_METRIC_PREFIX + UP);
            if (gauge == null) {
                Gauge upgauge = Gauge.build().name(ORACLEDB_METRIC_PREFIX + UP).help("Whether the Oracle database server is up.").register(registry);
                upgauge.set(isConnectionSuccessful);
                registry.gaugeMap.put(ORACLEDB_METRIC_PREFIX + UP, upgauge);
            } else gauge.set(isConnectionSuccessful);
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
    private void processMetric(CollectorRegistryWithGaugeMap registry, Connection connection, Iterator<JsonNode> metric) {
        JsonNode next = metric.next();
        String context = next.get(CONTEXT).asText(); // eg context = "teq"
        String metricsType = next.get(METRICSTYPE) == null ? "" : next.get(METRICSTYPE).asText();
        JsonNode metricsdescNode = next.get(METRICSDESC);
        // eg metricsdesc = { enqueued_msgs = "Total enqueued messages.", dequeued_msgs = "Total dequeued messages.", remained_msgs = "Total remained messages."}
        Iterator<Map.Entry<String, JsonNode>> metricsdescIterator = metricsdescNode.fields();
        Map<String, String> metricsDescMap = new HashMap<>();
        while (metricsdescIterator.hasNext()) {
            Map.Entry<String, JsonNode> metricsdesc = metricsdescIterator.next();
            metricsDescMap.put(metricsdesc.getKey(), metricsdesc.getValue().asText());
        }
        LOGGER.debug("context:" + context);
        String[] labelNames = new String[0];
        if (next.get(LABELS) != null) {
            int size = next.get(LABELS).size();
            Iterator<JsonNode> labelIterator = next.get(LABELS).iterator();
            labelNames = new String[size];
            for (int i = 0; i < size; i++) {
                labelNames[i] = labelIterator.next().asText();
            }
            LOGGER.debug("\n");
        }
        String request = next.get(REQUEST).asText(); // the sql query
        String ignorezeroresult = next.get(IGNOREZERORESULT) == null ? FALSE : next.get(IGNOREZERORESULT).asText(); //todo, currently defaults to true
        ResultSet resultSet;
        try {
            resultSet = connection.prepareStatement(request).executeQuery();
            while (resultSet.next()) {
                translateQueryToPrometheusMetric(registry, context, metricsDescMap, labelNames, resultSet);
            }
        } catch (SQLException e) { //this can be due to table not existing etc.
            LOGGER.warn("MetricsExporter.processMetric  during:" + request + " exception:" + e);
        }
    }

    private void translateQueryToPrometheusMetric(CollectorRegistryWithGaugeMap registry, String context, Map<String, String> metricsDescMap,
                                                  String[] labelNames,
                                                  ResultSet resultSet) throws SQLException {
        String[] labelValues = new String[labelNames.length];
        Map<String, Object> sqlQueryResults =
                extractGaugesAndLabelValues(registry, context, metricsDescMap, labelNames, resultSet, labelValues, resultSet.getMetaData().getColumnCount());
        if(sqlQueryResults == null || sqlQueryResults.entrySet() == null || sqlQueryResults.entrySet().isEmpty()) {
            LOGGER.error("Description for column is missing");
        }
        setLabelValues(registry, context, labelNames, labelValues, sqlQueryResults.entrySet().iterator());
    }

    /**
     * Creates Gauges and gets label values
     */
    private Map<String, Object> extractGaugesAndLabelValues(CollectorRegistryWithGaugeMap registry,
                                                          String context, Map<String, String> metricsDescMap, String[] labelNames, ResultSet resultSet,
                                                          String[] labelValues, int columnCount) throws SQLException {
        Map<String, Object> sqlQueryResults = new HashMap<>();
        String columnName;
        String columnTypeName;
        for (int i = 0; i < columnCount; i++) { //for each column...
            columnName = resultSet.getMetaData().getColumnName(i + 1).toLowerCase();
            columnTypeName = resultSet.getMetaData().getColumnTypeName(i + 1);
            if (columnTypeName.equals("VARCHAR2") || columnTypeName.equals("CHAR"))  //.  typename is 2/NUMBER or 12/VARCHAR2
                sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getString(i + 1));
            else {
                LOGGER.debug("columnTypeName:" + columnTypeName);
                sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getLong(i + 1));
            }
            String gaugeName = ORACLEDB_METRIC_PREFIX + context + "_" + columnName;
            LOGGER.debug("---gaugeName:" + gaugeName);
            Gauge gauge = registry.gaugeMap.get(gaugeName);
            if (gauge == null) {
                if (metricsDescMap.containsKey(columnName)) {
                    if (labelNames.length > 0) {
                        gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsDescMap.get(columnName)).labelNames(labelNames).register(registry);
                    } else
                        gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsDescMap.get(columnName)).register(registry);
                    registry.gaugeMap.put(gaugeName, gauge);
                }
            }
            for (int ii = 0; ii < labelNames.length; ii++) {
                if (labelNames[ii].equals(columnName)) labelValues[ii] = resultSet.getString(i + 1);
            }
        }
        return sqlQueryResults;
    }

    private void setLabelValues(CollectorRegistryWithGaugeMap registry, String context, String[] labelNames, String[] labelValues,
                                Iterator<Map.Entry<String, Object>> sqlQueryRestulsEntryIterator) {
        while (sqlQueryRestulsEntryIterator.hasNext()) { //for each column
            Map.Entry<String, Object> sqlQueryResultsEntry = sqlQueryRestulsEntryIterator.next();
            boolean isLabel = false;
            for (String labelName : labelNames) {
                if (labelName.equals(sqlQueryResultsEntry.getKey())) isLabel = true;  // continue
            }
            if (!isLabel) {
                Object valueToSet = sqlQueryResultsEntry.getValue();
                if (labelValues.length > 0)
                    try {
                        if(valueToSet instanceof Integer) registry.gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" +
                                sqlQueryResultsEntry.getKey().toLowerCase()).labels(labelValues).set(Math.rint((Integer)valueToSet));
                    } catch (Exception ex) { //todo filter to avoid unnecessary exception handling
                        LOGGER.debug("OracleDBMetricsExporter.translateQueryToPrometheusMetric Exc:" + ex);
                    }
                else
                    registry.gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" +
                            sqlQueryResultsEntry.getKey().toLowerCase()).labels(labelValues).set(Integer.parseInt("" + valueToSet));
            }
        }
    }

    public static String getMetricsString(CollectorRegistry collectorRegistry) {
        Enumeration<Collector.MetricFamilySamples> mfs = collectorRegistry.filteredMetricFamilySamples(new HashSet<>());
        return compose(mfs);
    }

    private static String compose(Enumeration<Collector.MetricFamilySamples> mfs) {
        StringBuilder result = new StringBuilder();
        while (mfs.hasMoreElements()) {
            Collector.MetricFamilySamples metricFamilySamples = mfs.nextElement();
            result.append("# HELP ")
                    .append(metricFamilySamples.name)
                    .append(' ');
            appendEscapedHelp(result, metricFamilySamples.help);
            result.append('\n');

            result.append("# TYPE ")
                    .append(metricFamilySamples.name)
                    .append(' ')
                    .append(typeString(metricFamilySamples.type))
                    .append('\n');

//            result.append("# DEBUG ")
//                    .append("metricFamilySamples.samples.size()")
//                    .append(' ')
//                    .append(metricFamilySamples.samples.size())
//                    .append('\n');

            for (Collector.MetricFamilySamples.Sample sample : metricFamilySamples.samples) {
                result.append(sample.name);
                if (!sample.labelNames.isEmpty()) {
                    result.append('{');
                    for (int i = 0; i < sample.labelNames.size(); ++i) {
                        result.append(sample.labelNames.get(i))
                                .append("=\"");
                        appendEscapedLabelValue(result, sample.labelValues.get(i));
                        result.append("\"");
                        if (i != sample.labelNames.size() - 1) result.append(",");
                    }
                    result.append('}');
                }
                result.append(' ')
                        .append(Collector.doubleToGoString(sample.value))
                        .append('\n');
            }
        }
        return result.toString();
    }

    private static void appendEscapedHelp(StringBuilder sb, String s) {
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '\\':
                    sb.append("\\\\");
                    break;
                case '\n':
                    sb.append("\\n");
                    break;
                default:
                    sb.append(c);
            }
        }
    }

    private static void appendEscapedLabelValue(StringBuilder sb, String s) {
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '\\':
                    sb.append("\\\\");
                    break;
                case '\"':
                    sb.append("\\\"");
                    break;
                case '\n':
                    sb.append("\\n");
                    break;
                default:
                    sb.append(c);
            }
        }
    }

    private static String typeString(Collector.Type t) {
        switch (t) {
            case GAUGE:
                return "gauge";
            case COUNTER:
                return "counter";
            case SUMMARY:
                return "summary";
            case HISTOGRAM:
                return "histogram";
            default:
                return "untyped";
        }
    }
}
