package oracle.observability.metrics;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import io.prometheus.client.Collector;
import io.prometheus.client.CollectorRegistry;
import io.prometheus.client.Gauge;
import oracle.observability.ObservabilityExporter;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.annotation.PostConstruct;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
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
    Map<String, Gauge> gaugeMap = new HashMap<>();
    Map<String, CollectorRegistry> dnsToCollectorRegistryMap = new HashMap<>();



    /**
     * The endpoint that prometheus will scrape
     * @return Prometheus metric
     */
    @GetMapping(value = "/metrics", produces = "text/plain")
    public String metrics() throws Exception {
        processMetrics(DATA_SOURCE_NAME, CollectorRegistry.defaultRegistry);
        return getMetricsString(CollectorRegistry.defaultRegistry);
    }
    @GetMapping(value = "/scrape", produces = "text/plain")
    public String scrape(@RequestParam("target") String target) throws Exception {
        CollectorRegistry collectorRegistry = dnsToCollectorRegistryMap.get(target);
        if (collectorRegistry == null) {
            collectorRegistry = new CollectorRegistry();
            dnsToCollectorRegistryMap.put(target, collectorRegistry);
        }
        processMetrics(target, dnsToCollectorRegistryMap.get(target));
        return getMetricsString(collectorRegistry);
    }

    @PostConstruct
    public void init() throws Exception {
        processMetrics(DATA_SOURCE_NAME, CollectorRegistry.defaultRegistry);
    }

    private void processMetrics(String datasourceName, CollectorRegistry registry) throws IOException, SQLException {
        File tomlfile = new File(DEFAULT_METRICS);
        TomlMapper mapper = new TomlMapper();
        JsonNode jsonNode = mapper.readerFor(MetricsExporterConfigEntry.class).readTree(new FileInputStream(tomlfile));
        JsonNode metric = jsonNode.get("metric");
        if(metric == null || metric.isEmpty()) {
            LOGGER.info("No logs records configured");
            return;
        }
        Iterator<JsonNode> metrics = metric.iterator();
        int isConnectionSuccessful = 0;
        try(Connection connection = getPoolDataSource(datasourceName).getConnection()) {
            isConnectionSuccessful = 1;
            while (metrics.hasNext()) {
                processMetric(registry, connection, metrics);
            }
        } finally {
            Gauge gauge = gaugeMap.get(ORACLEDB_METRIC_PREFIX + UP);
            if (gauge == null) {
                Gauge upgauge = Gauge.build().name(ORACLEDB_METRIC_PREFIX + UP).help("Whether the Oracle database server is up.").register(registry);
                upgauge.set(isConnectionSuccessful);
                gaugeMap.put(ORACLEDB_METRIC_PREFIX + UP, upgauge);
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
    private void processMetric(CollectorRegistry registry, Connection connection, Iterator<JsonNode> metric) {
        JsonNode next = metric.next();
        String context = next.get(CONTEXT).asText(); // eg context = "teq"
        String metricsType = next.get(METRICSTYPE) == null ? "" :next.get(METRICSTYPE).asText();
        JsonNode metricsdescNode = next.get(METRICSDESC);
        // eg metricsdesc = { enqueued_msgs = "Total enqueued messages.", dequeued_msgs = "Total dequeued messages.", remained_msgs = "Total remained messages."}
        Iterator<Map.Entry<String, JsonNode>> metricsdescIterator = metricsdescNode.fields();
        Map<String, String> metricsDescMap = new HashMap<>();
        while(metricsdescIterator.hasNext()) {
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
                 translateQueryToPrometheusMetric(registry, context,  metricsDescMap, labelNames, resultSet);
             }
        } catch(SQLException e) { //this can be due to table not existing etc.
            LOGGER.warn("MetricsExporter.processMetric  during:" + request + " exception:" + e);
            return;
        }
    }

    private void translateQueryToPrometheusMetric(CollectorRegistry registry, String context, Map<String, String> metricsDescMap,
                                                  String[] labelNames,
                                                  ResultSet resultSet) throws SQLException {
        String[] labelValues = new String[labelNames.length];
        Map<String, Long> sqlQueryResults =
                extractGaugesAndLabelValues(registry, context, metricsDescMap, labelNames, resultSet, labelValues, resultSet.getMetaData().getColumnCount());
        setLabelValues(context, labelNames, labelValues, sqlQueryResults.entrySet().iterator());
    }

    /**
     * Creates Gauges and gets label values
     */
    private Map<String, Long> extractGaugesAndLabelValues(CollectorRegistry registry,
            String context, Map<String, String> metricsDescMap, String[] labelNames, ResultSet resultSet,
            String[] labelValues, int columnCount) throws SQLException {
        Map<String, Long> sqlQueryResults = new HashMap<>();
        String columnName;
        String columnTypeName;
        for (int i = 0; i < columnCount; i++) { //for each column...
            columnName = resultSet.getMetaData().getColumnName(i + 1).toLowerCase();
            columnTypeName = resultSet.getMetaData().getColumnTypeName(i + 1);
            if (columnTypeName.equals("VARCHAR2"))  //.  typename is 2/NUMBER or 12/VARCHAR2
                ;
            else
                sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getLong(i + 1));
            String gaugeName = ORACLEDB_METRIC_PREFIX + context + "_" + columnName;
            LOGGER.debug("---gaugeName:" + gaugeName);
            Gauge gauge = gaugeMap.get(gaugeName);
            if (gauge == null) {
                if(metricsDescMap.containsKey(columnName)) {
                    if (labelNames.length > 0) {
                        gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsDescMap.get(columnName)).labelNames(labelNames).register(registry);
                    } else gauge = Gauge.build().name(gaugeName.toLowerCase()).help(metricsDescMap.get(columnName)).register(registry);
                    gaugeMap.put(gaugeName, gauge);
                }
            }
            for (int ii = 0; ii< labelNames.length; ii++) {
                if(labelNames[ii].equals(columnName)) labelValues[ii] = resultSet.getString(i+1);
            }
        }
        return sqlQueryResults;
    }

    private void setLabelValues(String context, String[] labelNames, String[] labelValues, Iterator<Map.Entry<String, Long>> sqlQueryRestulsEntryIterator) {
        while(sqlQueryRestulsEntryIterator.hasNext()) { //for each column
            Map.Entry<String, Long> sqlQueryResultsEntry =   sqlQueryRestulsEntryIterator.next();
            boolean isLabel = false;
            for (int ii = 0; ii< labelNames.length; ii++) {
                if(labelNames[ii].equals(sqlQueryResultsEntry.getKey())) isLabel =true;  // continue
            }
            if(!isLabel) {
                int valueToSet = (int) Math.rint(sqlQueryResultsEntry.getValue().intValue());
                if(labelValues.length >0 )
                    try {
                        gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" + sqlQueryResultsEntry.getKey().toLowerCase()).labels(labelValues).set(valueToSet);
                    } catch (Exception ex) { //todo filter to avoid unnecessary exception handling
                        LOGGER.debug("OracleDBMetricsExporter.translateQueryToPrometheusMetric Exc:" + ex);
                    }
                else gaugeMap.get(ORACLEDB_METRIC_PREFIX + context + "_" + sqlQueryResultsEntry.getKey().toLowerCase()).set(valueToSet);
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

            for (Collector.MetricFamilySamples.Sample sample: metricFamilySamples.samples) {
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
