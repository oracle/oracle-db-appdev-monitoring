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
import org.slf4j.LoggerFactory;
import org.springframework.web.bind.annotation.RestController;

import javax.annotation.PostConstruct;


@RestController
public final class TracingExporter extends ObservabilityExporter implements Runnable {

    private static final org.slf4j.Logger LOG = LoggerFactory.getLogger(TracingExporter.class);
    public static final String ECID_BIND_VALUES = "ECID_BIND_VALUES";
    private static final String ECID_BIND_VALUES_GETSQLID_SQL =
            "select ECID, SQL_ID from GV$SESSION where ECID IS NOT NULL";
    private static final String ECID_BIND_VALUES_GETBINDING_SQL =
            "SELECT sql_id, t.sql_text sql_text, b.name bind_name, b.value_string bind_value " +
            "FROM gv$sql t " +
            "JOIN gv$sql_bind_capture b using (sql_id) " +
            "WHERE b.value_string is not null AND sql_id = ? ";
    public static final String OTEL_JAEGER_ORACLEDBTRACER = "otel-jaeger-oracledbtracer";
    public static final String HTTP_JAEGER_COLLECTOR_MSDATAWORKSHOP_14268 = "http://jaeger-collector.msdataworkshop:14268"; //default
    public String TRACE_COLLECTOR_ADDRESS = System.getenv("TRACE_COLLECTOR_ADDRESS"); // "http://jaeger-collector.msdataworkshop:14268"  "http://localhost:14250"
    public String TRACE_INTERVAL = System.getenv("TRACE_INTERVAL"); // "30s"
    private int traceInterval = 30;
    private static OpenTelemetry openTelemetry;
    private static Tracer tracer;
    public static  TextMapPropagator TEXT_MAP_PROPAGATOR;
    List<String> processedTraces = new ArrayList<String>();

    @PostConstruct
    public void init() {
        openTelemetry = initOpenTelemetry();tracer =
        openTelemetry.getTracer("oracle.opentelemetry.trace.exporter.tracer");
        TEXT_MAP_PROPAGATOR = openTelemetry.getPropagators().getTextMapPropagator();
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
        JsonNode jsonNode;
        try {
            jsonNode = mapper.readerFor(TracingExporterConfigEntry.class).readTree(new FileInputStream(tomlfile));
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
        JsonNode trace = jsonNode.get("trace");
        if (trace == null || trace.isEmpty()) {
            LOG.info("No trace records configured");
            return;
        }
        Iterator<JsonNode> traces = trace.iterator();
        if(!traces.hasNext()) return;
        while (true) {
            try (Connection connection = getPoolDataSource().getConnection()) {
                while (traces.hasNext()) { //for each "log" entry in toml/config...
                    JsonNode next = traces.next();
                    String context = next.get("context").asText(); // the sql query
                    String request = next.get("request").asText(); // the sql query
                    String template = next.get("template").asText(); // the sql query
                    LOG.debug("DBTracingExporter.request:" + request);
                    LOG.debug("DBTracingExporter.template:" + template);
                    if (template != null && template.equals(ECID_BIND_VALUES)) {
                        ecidTraces(connection, context);
                    }
                }
                Thread.sleep(traceInterval * 1000);
            } catch (Exception e) {
                throw new RuntimeException(e);
            }
        }
    }

    void ecidTraces(Connection connection, String configContextName) throws SQLException {
        PreparedStatement preparedStatement = connection.prepareStatement(ECID_BIND_VALUES_GETSQLID_SQL);
        ResultSet rs = preparedStatement.executeQuery();
        while (rs.next()) {
            String traceparent = rs.getString("ECID");
            String SQL_ID = rs.getString("SQL_ID");
            String getbindingSQL = ECID_BIND_VALUES_GETBINDING_SQL;
            PreparedStatement sqlTextPS =  connection.prepareStatement(getbindingSQL);
            sqlTextPS.setString(1, SQL_ID);
            ResultSet sqlTextPSrs = sqlTextPS.executeQuery();
            String SQL_TEXT = "";
            String SQL_BIND = "";
            while (sqlTextPSrs.next()) {
                SQL_TEXT = sqlTextPSrs.getString("sql_text");
                SQL_BIND = sqlTextPSrs.getString("bind_value");
            }
            if (!processedTraces.contains(traceparent)) {
                LOG.debug("processing ecid/traceparent:" + traceparent);
                LOG.debug("processing SQL_ID:" + SQL_ID);
                LOG.debug("processing SQL_TEXT:" + SQL_TEXT);
                LOG.debug("processing SQL_BIND:" + SQL_BIND);
                Context context = TEXT_MAP_PROPAGATOR.extract(Context.current(), null, getTextMapGetter(traceparent));
                LOG.debug("context:" + context);
                Span childSpan =
                        tracer.spanBuilder("oracledb_traceexporter_" + configContextName).setParent(context).setSpanKind(SpanKind.SERVER).startSpan();
                LOG.debug("childSpan:" + childSpan);
                try (Scope scope = childSpan.makeCurrent()) {
                    childSpan.setAttribute("SQL_ID", SQL_ID);
                    childSpan.setAttribute("SQL_TEXT", SQL_TEXT);
                    childSpan.setAttribute("SQL_BIND", SQL_BIND);
                    childSpan.addEvent("SQL_ID:" + SQL_ID);
                    childSpan.addEvent("SQL_TEXT:" + SQL_TEXT);
                    childSpan.addEvent("SQL_BIND:" + SQL_BIND);
                    processedTraces.add(traceparent);
                } finally {
                    childSpan.end();
                }
            }
        }
    }

    private TextMapGetter<HttpExchange> getTextMapGetter(String traceparent) {
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

    private OpenTelemetry initOpenTelemetry() {
        String traceCollectorAddress = TRACE_COLLECTOR_ADDRESS == null || TRACE_COLLECTOR_ADDRESS.trim().equals("") ?
                HTTP_JAEGER_COLLECTOR_MSDATAWORKSHOP_14268 :TRACE_COLLECTOR_ADDRESS;
        JaegerGrpcSpanExporter jaegerExporter =
                JaegerGrpcSpanExporter.builder()
                        .setEndpoint(traceCollectorAddress)
                        .setTimeout(30, TimeUnit.SECONDS)
                        .build();
        Resource serviceNameResource =
                Resource.create(Attributes.of(ResourceAttributes.SERVICE_NAME, OTEL_JAEGER_ORACLEDBTRACER));
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
