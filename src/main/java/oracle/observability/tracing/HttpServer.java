/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

package oracle.observability.tracing;

import static io.opentelemetry.api.common.AttributeKey.stringKey;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.common.Attributes;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.SpanKind;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.Scope;
import io.opentelemetry.context.propagation.TextMapGetter;
import io.opentelemetry.context.propagation.TextMapPropagator;
import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.Charset;


import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;

import java.sql.Connection;
import java.sql.SQLException;

import oracle.jdbc.driver.OracleConnection;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.URL;
import java.net.URLConnection;
import java.nio.charset.Charset;

public final class HttpServer {
  // It's important to initialize your OpenTelemetry SDK as early in your application's lifecycle as
  // possible.
//  private static final OpenTelemetry openTelemetry = ExampleConfiguration.initOpenTelemetry();
  private static final OpenTelemetry openTelemetry = OpenTelemetryInitializer.initOpenTelemetry();
  private static final Tracer tracer =
      openTelemetry.getTracer("io.opentelemetry.example.http.HttpServer");

  private static final int port = 8080;
  private final com.sun.net.httpserver.HttpServer server;
  List<String> processedTraces = new ArrayList<String>();

  // Extract the context from http headers
  private static final TextMapGetter<HttpExchange> getter =
      new TextMapGetter<>() {
        @Override
        public Iterable<String> keys(HttpExchange carrier) {
          return carrier.getRequestHeaders().keySet();
        }

        @Override
        public String get(HttpExchange carrier, String key) {
          if (carrier.getRequestHeaders().containsKey(key)) {
            return carrier.getRequestHeaders().get(key).get(0);   //todo make one of these getters for each ecid in resultset
          }
          return "";
        }
      };

  private HttpServer() throws IOException {
    this(port);
  }

  private HttpServer(int port) throws IOException {
    server = com.sun.net.httpserver.HttpServer.create(new InetSocketAddress(port), 0);
    // Test urls
//    server.createContext("/", new TracingHandler());
//    server.start();
    System.out.println("Server ready on http://127.0.0.1:" + port);
  }

  public static final TextMapPropagator TEXT_MAP_PROPAGATOR =
          openTelemetry.getPropagators().getTextMapPropagator();

  private static class TracingHandler implements HttpHandler {

    public static final TextMapPropagator TEXT_MAP_PROPAGATOR =
        openTelemetry.getPropagators().getTextMapPropagator();

    @Override
    public void handle(HttpExchange exchange) throws IOException {
      System.out.println("rhow intentional"); //Traceparent header value 00-555108d22d9443dd598b4439ec449c91-2992f0d38f2fdb88-01
//      if(true) throw new IOException("IOException intentional");
//      System.out.println("sleep 30 seconds...");
//      try {
//        Thread.sleep(1000 * 30);
//      } catch (InterruptedException e) {
//        e.printStackTrace();
//      }
      // Extract the context from the HTTP request
      Context context = TEXT_MAP_PROPAGATOR.extract(Context.current(), exchange, getter);

      Span span =
          tracer.spanBuilder("GET /").setParent(context).setSpanKind(SpanKind.SERVER).startSpan();

      try (Scope scope = span.makeCurrent()) {
        // Set the Semantic Convention
        span.setAttribute("component", "http");
        span.setAttribute("http.method", "GET");
        /*
         One of the following is required:
         - http.scheme, http.host, http.target
         - http.scheme, http.server_name, net.host.port, http.target
         - http.scheme, net.host.name, net.host.port, http.target
         - http.url
        */
        span.setAttribute("http.scheme", "http");
        span.setAttribute("http.host", "localhost:" + HttpServer.port);
        span.setAttribute("http.target", "/");
        // Process the request
        answer(exchange, span);
      } finally {
        // Close the span
        span.end();
      }
    }

    private void answer(HttpExchange exchange, Span span) throws IOException {
      // Generate an Event
      span.addEvent("Start Processing");

      // Process the request
      String response = "hworld paul";
      exchange.sendResponseHeaders(200, response.length());
      OutputStream os = exchange.getResponseBody();
      os.write(response.getBytes(Charset.defaultCharset()));
      os.close();
      System.out.println("Served Client: " + exchange.getRemoteAddress());

      // Generate an Event with an attribute
      Attributes eventAttributes = Attributes.of(stringKey("answer"), response);
      span.addEvent("Finish Processing", eventAttributes);
    }
  }

  private void stop() {
    server.stop(0);
  }

  /**
   * Main method to run the example.
   *
   * @param args It is not required.
   * @throws Exception Something might go wrong.
   */
  public static void main(String[] args) throws Exception {
    final HttpServer s = new HttpServer();
    s.doMain(null);
    // Gracefully close the server
 //   Runtime.getRuntime().addShutdownHook(new Thread(s::stop));
  }


  private void doMain(String traceid) throws Exception {
//    OpenTelemetry openTelemetry = OpenTelemetryInitializer.initOpenTelemetry();
//    Tracer tracer = openTelemetry.getTracer("oracle.OracleDBTracer");
    PoolDataSource atpTraceExporterPDB = PoolDataSourceFactory.getPoolDataSource();
    atpTraceExporterPDB.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
    atpTraceExporterPDB.setURL("jdbc:oracle:thin:/@sagadb1_tp?TNS_ADMIN=/Users/pparkins/Downloads/Wallet_sagadb1");
    atpTraceExporterPDB.setUser("admin");
    atpTraceExporterPDB.setPassword("Welcome12345");
    System.out.println("datasource:" + atpTraceExporterPDB);
    try (Connection connection = atpTraceExporterPDB.getConnection()) {
      System.out.println("OracleDBTracingExporter querying for tracing info using connection:" + connection);
//      if (traceid != null && !traceid.trim().equals("")) {
//        System.out.println("OracleDBTracingExporter added to provided traceid:" + traceid);
//        Span parentSpan = tracer.spanBuilder(traceid).startSpan();
//        Span childSpan = tracer.spanBuilder("childaddedbytraceexporterprovided")
//                .setParent(Context.current().with(parentSpan))
//                .startSpan();
//        childSpan.end();
//        parentSpan.end();
//        System.exit(0);
//      }
      System.out.println("connection:" + connection);
   //   connection.createStatement().execute("ALTER SESSION SET SQL_TRACE = TRUE");
  //    System.out.println("OracleDBTracingExporter ALTER SESSION SET SQL_TRACE = TRUE" );
      while (true) { //select SQL_TEXT from  GV$SQLAREA where SQL_ID= 'bkfz4r6yw5krx';
        try (PreparedStatement preparedStatement =
                         connection.prepareStatement("select ECID, SQL_ID from GV$SESSION where ECID IS NOT NULL")) {
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
                    "sql_id=? "; // +
//                    "/";
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
          Thread.sleep(1 * 1000);
        }
        //        span.addEvent("OracleDB test span. addEvent connection:" + connection);
        //  span.addEvent("OracleDB test span event2 connection:" + connection);
        //       parentOne(span.getSpanContext().getTraceId());  // it was using "parent2"
        //         span.end();
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
       //         if (carrier.getRequestHeaders().containsKey(key)) {
                  return traceparent;
         //         return carrier.getRequestHeaders().get(key).get(0);   //todo make one of these getters for each ecid in resultset
         //       }
        //        return "";
              }
            };
  }

}
