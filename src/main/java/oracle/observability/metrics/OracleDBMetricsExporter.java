package oracle.observability.metrics;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import io.prometheus.client.CollectorRegistry;
import io.prometheus.client.Gauge;
import io.prometheus.client.Collector;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import javax.annotation.PostConstruct;
import java.io.File;
import java.io.FileInputStream;
import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.*;

@RestController
public class OracleDBMetricsExporter extends OracleDBMetricsExporterEnvConfig {


    /**
     * Context          string
     * Labels           []string
     * MetricsDesc      map[string]string
     * MetricsType      map[string]string
     * MetricsBuckets   map[string]map[string]string
     * FieldToAppend    string
     * Request          string
     * IgnoreZeroResult bool
     */

    Map<String, Gauge> gaugeMap = new HashMap<>();
    PoolDataSource atpBankPDB;
    boolean isInitComplete = false;

    private PoolDataSource getPoolDataSource() throws Exception {
        if (atpBankPDB != null) return atpBankPDB;
        atpBankPDB = PoolDataSourceFactory.getPoolDataSource();
        atpBankPDB.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
        //todo parse DATA_SOURCE_NAME
        String user = DATA_SOURCE_NAME.substring(0, DATA_SOURCE_NAME.indexOf("/"));
        String pw = DATA_SOURCE_NAME.substring(DATA_SOURCE_NAME.indexOf("/") + 1, DATA_SOURCE_NAME.indexOf("@"));
        String serviceName = DATA_SOURCE_NAME.substring(DATA_SOURCE_NAME.indexOf("@") + 1);
        String bankdburl = "jdbc:oracle:thin:@" + serviceName + "?TNS_ADMIN=" + TNS_ADMIN;
        System.out.println("OracleDBMetricsExporter.getPoolDataSource bankdburl:" + bankdburl);
        atpBankPDB.setURL(bankdburl);
        atpBankPDB.setUser(user);
        atpBankPDB.setPassword(pw);
        System.out.println("bank atpBankPDB:" + atpBankPDB);
        return atpBankPDB;
    }

    @GetMapping("/init")
    public String start() throws Exception {
        init();
        return "init complete";
    }

    @GetMapping("/metrics")
    public String metrics() throws Exception {
        return MetricsHandler.getMetricsString();
    }

    @PostConstruct
    public void init() throws Exception {
//        int port = 8080;
//        HttpServer httpServer = com.sun.net.httpserver.HttpServer.create(new InetSocketAddress(port), 0);
//        httpServer.createContext("/", new MetricsHandler());
//        httpServer.start();
//        System.out.println("Server ready on http://127.0.0.1:" + port);
        Connection connection = getPoolDataSource().getConnection();
        System.out.println("OracleDBMetricsExporter connection:" + connection);
        System.out.println("OracleDBMetricsExporter DEFAULT_METRICS:" + DEFAULT_METRICS);
        System.out.println("OracleDBMetricsExporter CUSTOM_METRICS:" + CUSTOM_METRICS);
        File tomlfile = new File(DEFAULT_METRICS);
        TomlMapper mapper = new TomlMapper();
        JsonNode jsonNode = mapper.readerFor(MetricEntry.class).readTree(new FileInputStream(tomlfile));
        Iterator<JsonNode> metric = jsonNode.get("metric").iterator();
        String context, request, ignorezeroresult;
        while (metric.hasNext()) {
            processMetric(connection, metric);
        }
        isInitComplete = true;

    }

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
        System.out.println("----context:" + context + "----metricsdesc:" + metricsdesc);
        String[] labelNames = null;
        if (next.get("labels") != null) {
            int size = next.get("labels").size();
            Iterator<JsonNode> labelIterator = next.get("labels").iterator();
            labelNames = new String[size];
            System.out.print("----labels:");
            for (int i = 0; i < size; i++) {
                labelNames[i] = labelIterator.next().asText();
                System.out.print(" " + labelNames[i] );
            }
            System.out.println();
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
            System.out.print(columnName + "=" + columnValue + " ");
            columnTypeName = resultSet.getMetaData().getColumnTypeName(i + 1);
            //.  typename is 2/NUMBER or 12/VARCHAR2
            if (columnTypeName.equals("VARCHAR2"))
                ; // sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getString(i + 1));
            else
                sqlQueryResults.put(resultSet.getMetaData().getColumnName(i + 1), resultSet.getDouble(i + 1));
//            if (isInitComplete) return;
            String gaugeName = "oracledb_" + context + "_" + columnName;
            System.out.println("---gaugeName:" + gaugeName);
            Gauge gauge = gaugeMap.get(gaugeName);
            if (gauge == null) {
                if (labelNames.length != 0) {
                    gauge = Gauge.build().name(gaugeName).help(metricsdesc).labelNames(labelNames).register(); // todo register only once
                } else gauge = Gauge.build().name(gaugeName).help(metricsdesc).register();
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
                gaugeMap.get("oracledb_" + context + "_" + entry.getKey().toLowerCase()).labels(labelValues).set(entry.getValue());
            }
        }
    }

    private List<String> collectorNames(Collector collector) {
        List<Collector.MetricFamilySamples> mfs;
        boolean autoDescribe = true;
        if (collector instanceof Collector.Describable) {
            mfs = ((Collector.Describable) collector).describe();
        } else if (autoDescribe) {
            mfs = collector.collect();
        } else {
            mfs = Collections.emptyList();
        }
        List<String> names = new ArrayList<String>();
        for (Collector.MetricFamilySamples family : mfs) {
            switch (family.type) {
                case SUMMARY:
                    names.add(family.name + "_count");
                    names.add(family.name + "_sum");
                    names.add(family.name);
                    break;
                case HISTOGRAM:
                    names.add(family.name + "_count");
                    names.add(family.name + "_sum");
                    names.add(family.name + "_bucket");
                    names.add(family.name);
                    break;
                default:
                    names.add(family.name);
            }
        }
        return names;
    }

    public Map<Map<String, String>, Double> getValues(Collector metric) {
        Map<Map<String, String>, Double> result = new HashMap<>();
        for (Collector.MetricFamilySamples samples : metric.collect()) {
            for (Collector.MetricFamilySamples.Sample sample : samples.samples) {
                Map<String, String> labels = new HashMap<>();
                for (int i = 0; i < sample.labelNames.size(); i++) {
                    labels.put(sample.labelNames.get(i), sample.labelValues.get(i));
                }
                result.put(labels, sample.value);
            }
        }
        return result;
    }


}
