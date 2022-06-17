package oracle.observability.tracing;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.SpanKind;
import io.opentelemetry.api.trace.Tracer;

import io.opentelemetry.context.Context;
import io.opentelemetry.context.Scope;
import io.opentelemetry.context.propagation.ContextPropagators;
import io.opentelemetry.context.propagation.TextMapPropagator;

import java.io.File;
import java.io.FileInputStream;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;

import java.sql.Connection;

import oracle.observability.ObservabilityExporter;
import oracle.observability.metrics.MetricsExporterConfigEntry;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;
import org.slf4j.LoggerFactory;
import org.springframework.web.bind.annotation.RestController;

import javax.annotation.PostConstruct;


@RestController
public final class TraceExporter extends ObservabilityExporter {

    List<String> processedTraces = new ArrayList<String>();

    private static final org.slf4j.Logger LOG = LoggerFactory.getLogger(TraceExporter.class);

    @PostConstruct
    public void init() throws Exception {
        LOG.debug("DBTracingExporter DEFAULT_METRICS:" + DEFAULT_METRICS);
        LOG.debug("DBTracingExporter CUSTOM_METRICS:" + CUSTOM_METRICS); //todo only default traces are processed currently
        File tomlfile = new File(DEFAULT_METRICS);
        TomlMapper mapper = new TomlMapper();
        JsonNode jsonNode = mapper.readerFor(TraceExporterConfigEntry.class).readTree(new FileInputStream(tomlfile));
        JsonNode trace = jsonNode.get("trace");
        if(trace==null) {
            LOG.info("No trace records configured");
            System.out.println("No trace records configured");
            return;
        }
        Iterator<JsonNode> traces = trace.iterator();
        try (Connection connection = getPoolDataSource().getConnection()) {
            while (traces.hasNext()) { //for each "log" entry in toml/config...
                JsonNode next = traces.next();
                String request = next.get("request").asText(); // the sql query
                String template = next.get("template").asText(); // the sql query
                System.out.println("DBTracingExporter.request:" + request);
                ResultSet resultSet = connection.prepareStatement(request).executeQuery();
                while (resultSet.next()) {
                    int columnCount = resultSet.getMetaData().getColumnCount();
                    String logString = "";
                    for (int i = 0; i < columnCount; i++) { //for each column...
                        logString += resultSet.getMetaData().getColumnName(i + 1) + "=" + resultSet.getObject(i + 1) + " ";
                    }
                    System.out.println(logString);
                }
//				int queryRetryInterval = queryRetryIntervalString == null ||
//						queryRetryIntervalString.trim().equals("") ?
//						DEFAULT_RETRY_INTERVAL : Integer.parseInt(queryRetryIntervalString.trim());
//				Thread.sleep(1000 * queryRetryInterval);
            }
        }
    }


    private void doMain(String traceid) throws Exception {
        OpenTelemetry openTelemetry = OpenTelemetryInitializer.initOpenTelemetry();
        Tracer tracer = openTelemetry.getTracer("oracle.OracleDBTracer");
        try (Connection connection = getPoolDataSource().getConnection()) {
            System.out.println("OracleDBTracingExporter querying for tracing info using connection:" + connection);
            if (traceid != null && !traceid.trim().equals("")) {
                System.out.println("OracleDBTracingExporter added to provided traceid:" + traceid);
                Span parentSpan = tracer.spanBuilder(traceid).startSpan();
                Span childSpan = tracer.spanBuilder("childaddedbytraceexporterprovided")
                        .setParent(Context.current().with(parentSpan))
                        .startSpan();
                childSpan.end();
                parentSpan.end();
                System.exit(0);
            }
            System.out.println("OracleDBTracingExporter querying for tracing info using connection:" + connection);
            while(true) {
                PreparedStatement preparedStatement = connection.prepareStatement("select ECID, SQL_ID from V$SESSION  where ECID IS NOT NULL");
                ResultSet rs = preparedStatement.executeQuery();
                while(rs.next() ) {
                    String spanName = rs.getString("ECID");
                    String sqlID = rs.getString("SQL_ID");
                    if (processedTraces.contains(spanName)) {
                        System.out.println("ecid/spanName already processed:" + spanName);
                    } else {
                        System.out.println("~~~~~~~~~~~~~~processing ecid/spanName:" + spanName);
                        System.out.println("~~~~~~~~~~~~~~processing SQL_ID:" + sqlID);
                  //      Span parentSpan = tracer.spanBuilder(spanName).startSpan();
                        Span parentSpan = tracer.spanBuilder("asdf98921a0e1e21a8ed94dasdfasdf").startSpan();
                        Span childSpan = tracer.spanBuilder("childaddedbytraceexporter")
                                .setParent(Context.current().with(parentSpan))
                                .startSpan();
                        childSpan.end();
                        parentSpan.end();
                        processedTraces.add(spanName);
                    }
                }
                Thread.sleep(1 * 1000);
            }
    //        span.addEvent("OracleDB test span. addEvent connection:" + connection);
            //  span.addEvent("OracleDB test span event2 connection:" + connection);
     //       parentOne(span.getSpanContext().getTraceId());  // it was using "parent2"
   //         span.end();
        }
    }

}
