package oracle.observability.metrics;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import io.prometheus.client.Collector;
import io.prometheus.client.CollectorRegistry;

import java.io.IOException;
import java.io.OutputStream;
import java.nio.charset.Charset;
import java.util.Enumeration;
import java.util.HashSet;
import java.util.Set;

public class MetricsHandler implements HttpHandler {
    @Override
    public void handle(HttpExchange exchange) throws IOException {
        String response =  getMetricsString();
        exchange.sendResponseHeaders(200, response.length());
        OutputStream os = exchange.getResponseBody();
        os.write(response.getBytes(Charset.defaultCharset()));
        os.close();
    }


    public static String getMetricsString() {
        CollectorRegistry collectorRegistry = CollectorRegistry.defaultRegistry;
        Enumeration<Collector.MetricFamilySamples> mfs = collectorRegistry.filteredMetricFamilySamples(new HashSet<>());
        return compose(mfs);
    }

    /**
     * Compose the text version 0.0.4 of the given MetricFamilySamples.
     */
    private static String compose(Enumeration<Collector.MetricFamilySamples> mfs) {
        /* See http://prometheus.io/docs/instrumenting/exposition_formats/
         * for the output format specification. */
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
//                System.out.println("MetricsHandler.compose sample.value:" + sample.value);
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
