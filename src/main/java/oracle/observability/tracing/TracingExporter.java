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

    private static final org.slf4j.Logger LOGGER = LoggerFactory.getLogger(TracingExporter.class);
    public static final String ECID = "ECID";
    public static final String ECID_BIND_VALUES = "ECID_BIND_VALUES";
    private static final String ECID_BIND_VALUES_GETSQLID_SQL =
            "select ECID, SQL_ID from GV$ACTIVE_SESSION_HISTORY where ECID IS NOT NULL";
    private static final String ECID_BIND_VALUES_GETBINDING_SQL =
            "SELECT sql_id, t.sql_text sql_text, b.name bind_name, b.value_string bind_value " +
            "FROM gv$sql t " +
            "JOIN gv$sql_bind_capture b using (sql_id) " +
            "WHERE b.value_string is not null AND sql_id = ? ";
    public static final String OTEL_JAEGER_ORACLEDBTRACER = "otel-jaeger-oracledbtracer";
    public static final String HTTP_JAEGER_COLLECTOR_MSDATAWORKSHOP_14268 = "http://jaeger-collector.msdataworkshop:14268"; //default
    public static final String TEMPLATE = "template";
    public static final String SQL_ID = "SQL_ID";
    public static final String SQL_TEXT = "sql_text";
    public static final String BIND_VALUE = "bind_value";
    public static final String ORACLEDB_TracingExporter = "oracledb_TracingExporter_";
    public String TRACE_COLLECTOR_ADDRESS = System.getenv("TRACE_COLLECTOR_ADDRESS"); // "http://jaeger-collector.msdataworkshop:14268"  "http://localhost:14250"
    public String TRACE_INTERVAL = System.getenv("TRACE_INTERVAL"); // "30s"
    private int traceInterval;
    private int traceIntervalDefault = 30;
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
        LOGGER.debug("TracingExporter DEFAULT_METRICS:" + DEFAULT_METRICS);
        if (TRACE_INTERVAL != null && !TRACE_INTERVAL.trim().equals(""))
            traceInterval = Integer.getInteger(TRACE_INTERVAL);
        else traceInterval = traceIntervalDefault;
        LOGGER.debug("TracingExporter traceInterval:" + traceInterval);
        //todo move to common/ObservabilityExporter location and log something friendly if it does not exist and exit, ie fast fail startup
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
            LOGGER.info("No trace records configured");
            return;
        }
        Iterator<JsonNode> traces = trace.iterator();
        if(!traces.hasNext()) return;
        while (true) {
            try (Connection connection = getPoolDataSource().getConnection()) {
                while (traces.hasNext()) {
                    JsonNode next = traces.next();
                    String context = next.get(CONTEXT).asText();
                    String request = next.get(REQUEST).asText();
                    String template = next.get(TEMPLATE).asText();
                    LOGGER.debug("TracingExporter request:" + request);
                    if (template != null && template.equals(ECID_BIND_VALUES)) {
                        ecidTraces(connection, context);
                    }
                }
                Thread.sleep(traceInterval * 1000);
            } catch (Exception e) {
                LOGGER.warn("TracingExporter.processMetric exception:" + e);
            }
        }
    }

    void ecidTraces(Connection connection, String configContextName) throws SQLException {
        PreparedStatement preparedStatement = connection.prepareStatement(ECID_BIND_VALUES_GETSQLID_SQL);
        ResultSet rs = preparedStatement.executeQuery();
//        while (rs.next()) {
        rs.next();
            String traceparent = rs.getString(ECID);
            LOGGER.debug("TracingExporter traceparent:" + traceparent);
            String sqlID = rs.getString(SQL_ID);
            String getbindingSQL = ECID_BIND_VALUES_GETBINDING_SQL;
            PreparedStatement sqlTextPS =  connection.prepareStatement(getbindingSQL);
            sqlTextPS.setString(1, sqlID);
            ResultSet sqlTextPSrs = sqlTextPS.executeQuery();
            String sqlText = "";
            String sqlBind = "";
            while (sqlTextPSrs.next()) {
                sqlText = sqlTextPSrs.getString(SQL_TEXT);
                sqlBind = sqlTextPSrs.getString(BIND_VALUE);
            }
            if (!processedTraces.contains(traceparent)) { //todo check contents as well
                LOGGER.debug("processing ecid/traceparent:" + traceparent);
                LOGGER.debug("processing SQL_ID:" + sqlID);
                LOGGER.debug("processing SQL_TEXT:" + sqlText);
                LOGGER.debug("processing SQL_BIND:" + sqlBind);
                Context context = TEXT_MAP_PROPAGATOR.extract(Context.current(), null, getTextMapGetter(traceparent));
                LOGGER.debug("context:" + context);
                Span childSpan =
                        tracer.spanBuilder(ORACLEDB_TracingExporter + configContextName)
                                .setParent(context).setSpanKind(SpanKind.SERVER).startSpan();
                LOGGER.debug("childSpan:" + childSpan);
                try (Scope scope = childSpan.makeCurrent()) {
                    childSpan.setAttribute(SQL_ID, sqlID);
                    childSpan.setAttribute("SQL_TEXT", sqlText);
                    childSpan.setAttribute("SQL_BIND", sqlBind);
                    childSpan.addEvent("SQL_ID:" + sqlID);
                    childSpan.addEvent("SQL_TEXT:" + sqlText);
                    childSpan.addEvent("SQL_BIND:" + sqlBind);
                    processedTraces.add(traceparent);
                } finally {
                    childSpan.end();
                }
            }
//        }
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
        LOGGER.warn("TracingExporter traceCollectorAddress:" + traceCollectorAddress);
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
