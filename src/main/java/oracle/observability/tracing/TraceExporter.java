package oracle.observability.tracing;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import com.sun.net.httpserver.HttpExchange;
import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.common.Attributes;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.SpanKind;
import io.opentelemetry.api.trace.Tracer;

import io.opentelemetry.api.trace.propagation.W3CTraceContextPropagator;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.Scope;
import io.opentelemetry.context.propagation.ContextPropagators;
import io.opentelemetry.context.propagation.TextMapGetter;
import io.opentelemetry.context.propagation.TextMapPropagator;

import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;

import java.sql.Connection;
import java.util.concurrent.TimeUnit;

import io.opentelemetry.exporter.jaeger.JaegerGrpcSpanExporter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.resources.Resource;
import io.opentelemetry.sdk.trace.SdkTracerProvider;
import io.opentelemetry.sdk.trace.export.SimpleSpanProcessor;
import io.opentelemetry.semconv.resource.attributes.ResourceAttributes;
import oracle.observability.ObservabilityExporter;
import oracle.observability.logs.LogExporter;
import oracle.observability.metrics.MetricsExporterConfigEntry;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;
import org.slf4j.LoggerFactory;
import org.springframework.web.bind.annotation.RestController;

import javax.annotation.PostConstruct;


@RestController
public final class TraceExporter extends ObservabilityExporter implements Runnable {

    private static final org.slf4j.Logger LOG = LoggerFactory.getLogger(TraceExporter.class);

    public String TRACE_INTERVAL = System.getenv("TRACE_INTERVAL"); // "30s"
    private int traceInterval = 30;

    private static final OpenTelemetry openTelemetry = initOpenTelemetry();
    private static final Tracer tracer =
            openTelemetry.getTracer("oracle.opentelemetry.trace.exporter.tracer");

    public static final TextMapPropagator TEXT_MAP_PROPAGATOR =
            openTelemetry.getPropagators().getTextMapPropagator();
    List<String> processedTraces = new ArrayList<String>();

    @PostConstruct
    public void init() throws Exception {
        new Thread(this).start();
    }

    @Override
    public void run() {

        LOG.debug("TraceExporter DEFAULT_METRICS:" + DEFAULT_METRICS);
        if (TRACE_INTERVAL != null && !TRACE_INTERVAL.trim().equals(""))
            traceInterval = Integer.getInteger(TRACE_INTERVAL);
        LOG.debug("TraceExporter traceInterval:" + traceInterval);
        File tomlfile = new File(DEFAULT_METRICS);
        TomlMapper mapper = new TomlMapper();
        JsonNode jsonNode = null;
        try {
            jsonNode = mapper.readerFor(TraceExporterConfigEntry.class).readTree(new FileInputStream(tomlfile));
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
        JsonNode trace = jsonNode.get("trace");
        if (trace == null) {
            LOG.info("No trace records configured");
            System.out.println("No trace records configured");
            return;
        }
        Iterator<JsonNode> traces = trace.iterator();
        while (true) {
            try (Connection connection = getPoolDataSource().getConnection()) {
                while (traces.hasNext()) { //for each "log" entry in toml/config...
                    JsonNode next = traces.next();
                    String request = next.get("request").asText(); // the sql query
                    String template = next.get("template").asText(); // the sql query
                    System.out.println("DBTracingExporter.request:" + request);
                    System.out.println("DBTracingExporter.template:" + template);
                    if (template != null && template.equals("")) {
                        ecidTraces(connection);
                    }
                }
                Thread.sleep(traceInterval * 1000);
            } catch (Exception e) {
                throw new RuntimeException(e);
            }
        }
    }

    void ecidTraces(Connection connection) throws SQLException {
        PreparedStatement preparedStatement = connection.prepareStatement("select ECID, SQL_ID from GV$SESSION where ECID IS NOT NULL");
//          select position, VALUE_STRING from  v$sql_bind_capture where SQL_ID = '8w8sbhtt057gx'
//                     connection.prepareStatement("select ECID, SQL_ID, SQL_TEXT from GV$ACTIVE_SESSION_HISTORY NATURAL JOIN GV$SQLAREA where ECID IS NOT NULL")) {
//                     connection.prepareStatement("select ECID, SQL_ID, SQL_TEXT from GV$SESSION NATURAL JOIN GV$SQLAREA where ECID IS NOT NULL")) {
        ResultSet rs = preparedStatement.executeQuery();
        while (rs.next()) {
            String traceparent = rs.getString("ECID");
            String SQL_ID = rs.getString("SQL_ID");
            String getbindingSQL = "SELECT " +
                    "sql_id, " +
                    "t.sql_text sql_text, " +
                    "b.name bind_name, " +
                    "b.value_string bind_value " +
                    "FROM " +
                    "gv$sql t " +
                    "JOIN " +
                    "gv$sql_bind_capture b using (sql_id) " +
                    "WHERE " +
                    "b.value_string is not null " +
                    "AND " +
                    "sql_id=? ";
//            PreparedStatement sqlTextPS =
//                    connection.prepareStatement("select SQL_TEXT from GV$SQLAREA where SQL_ID = ?");
            PreparedStatement sqlTextPS =
                    connection.prepareStatement(getbindingSQL);
            sqlTextPS.setString(1, SQL_ID);
            ResultSet sqlTextPSrs = sqlTextPS.executeQuery();
            String SQL_TEXT = "";
            String SQL_BIND = "";
            while (sqlTextPSrs.next()) {
//               SQL_TEXT = sqlTextPSrs.getString("1");
                SQL_TEXT = sqlTextPSrs.getString("sql_text");
                SQL_BIND = sqlTextPSrs.getString("bind_value");
            }
            if (processedTraces.contains(traceparent)) {
                System.out.println("ecid/spanName already processed:" + traceparent);
            } else {
                System.out.println("~~~~~~~~~~~~~~processing ecid/traceparent:" + traceparent);
                System.out.println("~~~~~~~~~~~~~~processing SQL_ID:" + SQL_ID);
                System.out.println("~~~~~~~~~~~~~~processing SQL_TEXT:" + SQL_TEXT);
                System.out.println("~~~~~~~~~~~~~~processing SQL_BIND:" + SQL_BIND);
//            Context context = TEXT_MAP_PROPAGATOR.extract(Context.current(), exchange, getter);
                Context context = TEXT_MAP_PROPAGATOR.extract(Context.current(), null, getTextMapGetter(traceparent));
//              Context context = TEXT_MAP_PROPAGATOR.extract(Context.current(), null, getTextMapGetter("ab418bb591ba5123b00a56960d3f8911-fd768cbdeb1f90c4"));
                System.out.println("~~~~~~~~~~~~~~context:" + context);
                Span parentSpan =
                        tracer.spanBuilder("childaddedbytraceexporter").setParent(context).setSpanKind(SpanKind.SERVER).startSpan();
                System.out.println("~~~~~~~~~~~~~~parentSpan:" + parentSpan);
                try (Scope scope = parentSpan.makeCurrent()) {
                    parentSpan.setAttribute("SQL_ID", SQL_ID);
                    parentSpan.setAttribute("SQL_TEXT", SQL_TEXT);
                    parentSpan.setAttribute("SQL_BIND", SQL_BIND);
                    //   Attributes eventAttributes = Attributes.of("SQL_ID", SQL_ID);
                    parentSpan.addEvent("SQL_ID:" + SQL_ID);
                    parentSpan.addEvent("SQL_TEXT:" + SQL_TEXT);
                    parentSpan.addEvent("SQL_BIND:" + SQL_BIND);
//              parentSpan.setAttribute("SQL_TEXT", SQL_TEXT);
                    //      Span parentSpan = tracer.spanBuilder(spanName).startSpan();
//            Span parentSpan = tracer.spanBuilder("asdf98921a0e1e21a8ed94dasdfasdf").startSpan();
                    Span childSpan = tracer.spanBuilder("grandchildaddedbytraceexporter")
                            .setParent(Context.current().with(parentSpan))
                            .startSpan();
                    childSpan.end();
//            parentSpan.end();
                    processedTraces.add(traceparent);
                } finally {
                    // Close the span
                    parentSpan.end();
                }
            }
        }
    }


    TextMapGetter<HttpExchange> getTextMapGetter(String traceparent) {
        return
                new TextMapGetter<>() {
                    @Override
                    public Iterable<String> keys(HttpExchange carrier) {
                        return carrier.getRequestHeaders().keySet();
                    }

                    @Override
                    public String get(HttpExchange carrier, String key) {
                        return traceparent;
                    }
                };
    }

    private void doMain(String traceid) throws Exception {
        OpenTelemetry openTelemetry = initOpenTelemetry();
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
            while (true) {
                PreparedStatement preparedStatement = connection.prepareStatement("select ECID, SQL_ID from V$SESSION  where ECID IS NOT NULL");
                ResultSet rs = preparedStatement.executeQuery();
                while (rs.next()) {
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

    static OpenTelemetry initOpenTelemetry() {
        //todo uses Jaeger currently
        JaegerGrpcSpanExporter jaegerExporter =
                JaegerGrpcSpanExporter.builder()
                        .setEndpoint("http://localhost:14250")
                        .setTimeout(30, TimeUnit.SECONDS)
                        .build();
        Resource serviceNameResource =
                Resource.create(Attributes.of(ResourceAttributes.SERVICE_NAME, "otel-jaeger-oracledbtracer"));
        SdkTracerProvider tracerProvider =
                SdkTracerProvider.builder()
                        .addSpanProcessor(SimpleSpanProcessor.create(jaegerExporter))
                        .setResource(Resource.getDefault().merge(serviceNameResource))
                        .build();
        OpenTelemetrySdk openTelemetry =
                OpenTelemetrySdk.builder().setTracerProvider(tracerProvider)
                        .setPropagators(ContextPropagators.create(W3CTraceContextPropagator.getInstance()))
                        .buildAndRegisterGlobal();
        Runtime.getRuntime().addShutdownHook(new Thread(tracerProvider::close));
        return openTelemetry;
    }
}
