/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

package oracle.observability.tracing;

import io.opentelemetry.api.OpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.SpanContext;
import io.opentelemetry.api.trace.SpanKind;
import io.opentelemetry.api.trace.StatusCode;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.Scope;
import io.opentelemetry.context.propagation.TextMapPropagator;
import io.opentelemetry.context.propagation.TextMapSetter;
import io.opentelemetry.semconv.trace.attributes.SemanticAttributes;
import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.URL;
import java.net.URLConnection;
import java.nio.charset.Charset;

import oracle.jdbc.driver.OracleConnection;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;

import java.io.IOException;
import java.net.HttpURLConnection;
import java.net.URL;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;

public final class HttpClient {

  // it's important to initialize the OpenTelemetry SDK as early in your applications lifecycle as
  // possible.
//  private static final OpenTelemetry openTelemetry = ExampleConfiguration.initOpenTelemetry();
  private static final OpenTelemetry openTelemetry = OpenTelemetryInitializer.initOpenTelemetry();

  private static final Tracer tracer =
      openTelemetry.getTracer("io.opentelemetry.example.http.HttpClient");
  private static final TextMapPropagator textMapPropagator =
      openTelemetry.getPropagators().getTextMapPropagator();

  // Export traces to log
  // Inject the span context into the request
  private static final TextMapSetter<HttpURLConnection> setter = URLConnection::setRequestProperty;

  private void makeRequest() throws IOException, URISyntaxException {
    int port = 8080;
    URL url = new URL("http://127.0.0.1:" + port);
    HttpURLConnection con = (HttpURLConnection) url.openConnection();

    int status = 0;
    StringBuilder content = new StringBuilder();

    // Name convention for the Span is not yet defined.
    // See: https://github.com/open-telemetry/opentelemetry-specification/issues/270
    Span span = tracer.spanBuilder("/").setSpanKind(SpanKind.CLIENT).startSpan();
    try (Scope scope = span.makeCurrent()) {
      span.setAttribute(SemanticAttributes.HTTP_METHOD, "GET");
      span.setAttribute("component", "http");
      /*
       Only one of the following is required
         - http.url
         - http.scheme, http.host, http.target
         - http.scheme, peer.hostname, peer.port, http.target
         - http.scheme, peer.ip, peer.port, http.target
      */
      URI uri = url.toURI();
      url =
          new URI(
                  uri.getScheme(),
                  null,
                  uri.getHost(),
                  uri.getPort(),
                  uri.getPath(),
                  uri.getQuery(),
                  uri.getFragment())
              .toURL();

      span.setAttribute(SemanticAttributes.HTTP_URL, url.toString());

      // Inject the request with the current Context/Span.
      textMapPropagator.inject(Context.current(), con, setter);

      try {
        SpanContext spanContext = span.getSpanContext();
//       ((RecordEventsReadableSpan) span).getSpanContext().getTraceState() just 00
//       ((RecordEventsReadableSpan) span).getSpanContext().getTraceId()
//       ((RecordEventsReadableSpan) span).getSpanContext().getSpanId()
//       ((RecordEventsReadableSpan) span).getSpanContext().getTraceFlags().asHe
//       String traceparent = ((HttpURLConnection) con).getHeaderField("traceparent");
       String traceparent =
               "00" + "-" + spanContext.getTraceId() + "-" + spanContext.getSpanId() + "-" + spanContext.getTraceFlags().asHex();
        span.end();
        if (true) {
          System.out.println("HttpClient.makeRequest JDBC traceparent:" + traceparent);
          doDB(traceparent);
          return;
        }
        con.setRequestMethod("GET");
        status = con.getResponseCode();
        BufferedReader in =
            new BufferedReader(
                new InputStreamReader(con.getInputStream(), Charset.defaultCharset()));
        String inputLine;
        while ((inputLine = in.readLine()) != null) {
          content.append(inputLine);
        }
        in.close();
      } catch (Exception e) {
        span.setStatus(StatusCode.ERROR, "HTTP Code: " + status);
      }
    } finally {
      span.end();
    }

    // Output the result of the request
    System.out.println("Response Code: " + status);
    System.out.println("Response Msg: " + content);
  }

  /**
   * Main method to run the example.
   *
   * @param args It is not required.
   */
  public static void main(String[] args) {
    HttpClient httpClient = new HttpClient();

    // Perform request every 5s
    Thread t =
        new Thread(
            () -> {
              while (true) {
                try {
                  httpClient.makeRequest();
                  Thread.sleep(5000);
                } catch (Exception e) {
                  System.out.println(e.getMessage());
                }
              }
            });
    t.start();
  }


  private void doDB(String traceId) throws Exception {
    PoolDataSource atpBankApp1PDB = PoolDataSourceFactory.getPoolDataSource();
    atpBankApp1PDB.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
    atpBankApp1PDB.setURL("jdbc:oracle:thin:/@sagadb1_tp?TNS_ADMIN=/Users/pparkins/Downloads/Wallet_sagadb1");
    atpBankApp1PDB.setUser("admin");
    atpBankApp1PDB.setPassword("Welcome12345");
    System.out.println("datasourcr:" + atpBankApp1PDB);
    try (Connection connection = atpBankApp1PDB.getConnection()) {
      System.out.println("connection:" + connection);
      System.out.println("OracleDBTracingExporter.queryOracleDBForSpans span.getSpanContext().getTraceId()" + traceId);
      String[] metric = new String[OracleConnection.END_TO_END_STATE_INDEX_MAX];
      metric[OracleConnection.END_TO_END_ACTION_INDEX] = "orderservice_action_placeOrder";
      metric[OracleConnection.END_TO_END_MODULE_INDEX] = "orderservice_module";
      metric[OracleConnection.END_TO_END_CLIENTID_INDEX] = "orderservice_clientid";
      metric[OracleConnection.END_TO_END_ECID_INDEX] = traceId; //for log to trace
      //     activeSpan.setBaggageItem("ecid", spanIdForECID); //for trace to log
      short seqnum = 20;
      connection.setClientInfo("CLIENTCONTEXT.ECID", traceId);
      connection.setClientInfo("E2E_CONTEXT.ECID_UID", traceId);
      connection.setClientInfo("E2E_CONTEXT.ECID", traceId);
      connection.setClientInfo("OCSID.ECID", traceId);
      connection.setClientInfo("OCSID.CLIENTID", traceId);
      connection.setClientInfo("E2E_CONTEXT.DBOP", traceId);
      connection.setClientInfo("E2E_CONTEXT.DBOP_NAME", traceId);
      //    PreparedStatement preparedStatement = connection.prepareStatement("insert into app_trace_test_table values(?)");
      for (int i=0;i<1000;i++ ) {
        System.out.println("OracleDBTracingExporter.update statistics_level=all  traceid="+ traceId);
//        connection.prepareStatement("ALTER SESSION SET SQL_TRACE = TRUE").execute();
//        connection.prepareStatement("alter system set statistics_level=all").execute();
//        connection.prepareStatement("exec dbms_monitor.session_trace_enable();").execute();
//        connection.prepareStatement("exec dbms_monitor.session_trace_enable( binds => true );").execute();
        PreparedStatement preparedStatement = connection.prepareStatement("update app_trace_test_table set id = ? where id = ?");
        preparedStatement.setString(1, traceId);
        preparedStatement.setString(2, traceId);
        preparedStatement.execute();
      }
    //  span.addEvent("OracleDB test span. addEvent connection:" + connection);
      //  span.addEvent("OracleDB test span event2 connection:" + connection);
      //       parentOne(span.getSpanContext().getTraceId());  // it was using "parent2"
  //    span.end();
      System.out.println("Sleep 300 and exit");
      Thread.sleep(300 * 1000);
    }
  }

}
