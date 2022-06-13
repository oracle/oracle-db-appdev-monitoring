package oracle.observability.tracing;

import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.SpanKind;
import io.opentelemetry.api.trace.Tracer;

import io.opentelemetry.context.Context;
import io.opentelemetry.context.Scope;
import io.opentelemetry.context.propagation.ContextPropagators;
import io.opentelemetry.context.propagation.TextMapPropagator;

import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.ArrayList;
import java.util.List;

import java.sql.Connection;

import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;

public final class DBTracingExporter {

    List<String> processedTraces = new ArrayList<String>();

    public static void main(String[] args) throws Exception {
//        new DBTracingExporter().doMain(args[0]);
    }

    private void doMain(String traceid) throws Exception {
        OpenTelemetry openTelemetry = OpenTelemetryInitializer.initOpenTelemetry();
        Tracer tracer = openTelemetry.getTracer("oracle.OracleDBTracer");
        PoolDataSource atpInventoryPDB = PoolDataSourceFactory.getPoolDataSource();
        atpInventoryPDB.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
        atpInventoryPDB.setURL("jdbc:oracle:thin:/@sagadb1_tp?TNS_ADMIN=/Users/pparkins/Downloads/Wallet_sagadb1");
        atpInventoryPDB.setUser("admin");
        atpInventoryPDB.setPassword("Welcome12345");
        System.out.println("InventoryResource.init atpInventoryPDB:" + atpInventoryPDB);
        try (Connection connection = atpInventoryPDB.getConnection()) {
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

    //https://javadoc.io/doc/io.opentelemetry/opentelemetry-context/latest/io/opentelemetry/context/propagation/ContextPropagators.html
    void onRequestReceived(OpenTelemetry openTelemetry, String request, Tracer tracer) {
        ContextPropagators propagators = openTelemetry.getPropagators();
        TextMapPropagator textMapPropagator = propagators.getTextMapPropagator();

        // Extract and store the propagated span's SpanContext and other available concerns
        // in the specified Context.
        Context context = textMapPropagator.extract(Context.current(), request,
                new TestTextMapGetter()
        );
        Span span = tracer.spanBuilder("MyRequest")
                .setParent(context)
                .setSpanKind(SpanKind.SERVER).startSpan();
        try (Scope ignored = span.makeCurrent()) {
            // Handle request and send response back.
        } finally {
            span.end();
        }
    }

    public class TestTextMapGetter implements io.opentelemetry.context.propagation.TextMapGetter {

        @Override
        public Iterable<String> keys(Object o) {
            return null;
        }

        @Override
        public String get(Object o, String s) {
            return null;
        }
    }
}

/**
 select SAMPLE_TIME, ecid, DBOP_NAME from V$ACTIVE_SESSION_HISTORY order by SAMPLE_TIME asc;
 select SAMPLE_TIME, ecid, DBOP_NAME from GV$ACTIVE_SESSION_HISTORY order by SAMPLE_TIME asc;
 select * from TABLE(GV$(CURSOR(select SAMPLE_TIME, ecid, DBOP_NAME from V$ACTIVE_SESSION_HISTORY order by SAMPLE_TIME asc)));
 select * from  app_trace_test_table;
 SELECT SQL_ID, SQL_FULLTEXT FROM v$sqlarea;


 // Most I/O intensive sql in last 6hrs
 SELECT sql_id, user_id COUNT(*)
 FROM gv$active_session_history ash, gv$event_name evt
 WHERE ash.sample_time > SYSDATE - 1/24
 AND ash.session_state = 'WAITING'
 AND ash.event_id = evt.event_id
 AND evt.wait_class = 'User I/O'
 GROUP BY sql_id, user_id
 ORDER BY COUNT(*) DESC;



 docker run -d --name jaeger \
 -e COLLECTOR_ZIPKIN_HOST_PORT=:9411 \
 -p 5775:5775/udp \
 -p 6831:6831/udp \
 -p 6832:6832/udp \
 -p 5778:5778 \
 -p 16686:16686 \
 -p 14250:14250 \
 -p 14268:14268 \
 -p 14269:14269 \
 -p 9411:9411 \
 jaegertracing/all-in-one:1.31



 * create table app_trace_test_table (id varchar(64));
 * <p>
 * ../gradlew shadowJar
 * java -cp build/libs/opentelemetry-examples-jaeger-0.1.0-SNAPSHOT-all.jar io.opentelemetry.example.jaeger.OracleDBTracingExporter http://localhost:14250
 * <p>
 * 1 and 2 are most important below...
 * <p>
 * 1.  https://opentelemetry.io/docs/instrumentation/java/manual/
 * Create nested Spans
 * Most of the time, we want to correlate spans for nested operations.
 * OpenTelemetry supports tracing within processes and across remote processes.
 * For more details how to share context between remote processes, see Context Propagation.
 * <p>
 * and
 * 2. https://opentelemetry.io/docs/instrumentation/java/manual/#context-propagation
 * <p>
 * ------------ begin the rest
 * <p>
 * most of this class is taken from https://github.com/open-telemetry/opentelemetry-java-docs/tree/main/jaeger
 * <p>
 * https://github.com/open-telemetry/opentelemetry-java-instrumentation/tree/main/instrumentation/jdbc/library
 * <dependencies>
 * <dependency>
 * <groupId>io.opentelemetry.instrumentation</groupId>
 * <artifactId>opentelemetry-jdbc</artifactId>
 * <version>OPENTELEMETRY_VERSION</version>
 * </dependency>
 * </dependencies>
 * <p>
 * import org.apache.commons.dbcp2.BasicDataSource;
 * import org.springframework.context.annotation.Configuration;
 * import io.opentelemetry.instrumentation.jdbc.datasource.OpenTelemetryDataSource;
 * <p>
 * <p>
 * new OpenTelemetryDataSource(dataSource);
 * <p>
 * various...
 * https://github.com/open-telemetry/opentelemetry-java-instrumentation
 * https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/main/examples/extension/README.md
 * https://github.com/open-telemetry/opentelemetry-java-instrumentation/tree/main/docs
 * https://signoz.io/opentelemetry/java-agent/
 * https://github.com/open-telemetry/opentelemetry-java/blob/main/sdk-extensions/autoconfigure/README.md
 * https://github.com/open-telemetry/opentelemetry-java-instrumentation
 * https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/main/docs/agent-config.md
 * https://github.com/open-telemetry/opentelemetry-java
 * https://www.javadoc.io/doc/io.opentelemetry/opentelemetry-api/latest/io/opentelemetry/api/trace/package-summary.html
 * https://www.javadoc.io/doc/io.opentelemetry/opentelemetry-exporter-jaeger/latest/io/opentelemetry/exporter/jaeger/package-summary.html
 * <p>
 * https://artifactory.oci.oraclecorp.com/libs-release/com/oracle/apm/agent/java/apm-java-agent-observer/1.4.2036/
 * <p>
 * https://community.oracle.com/tech/developers/discussion/1095488/how-do-i-get-the-sql-id-of-a-query-in-my-java-code-jdbc
 */