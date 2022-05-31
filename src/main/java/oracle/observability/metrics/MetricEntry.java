package oracle.observability.metrics;

public class MetricEntry {
//    public String getMetric() {
//        return metric;
//    }
//
//    public void setMetric(String metric) {
//        this.metric = metric;
//    }

//    String metric;
    String context;
    String metricsdesc;
    String request;

    @Override
    public String toString() {
        return "MetricEntry{" +
                "context='" + context + '\'' +
                ", metricsdesc='" + metricsdesc + '\'' +
                ", request='" + request + '\'' +
                '}';
    }

    public MetricEntry() {}

//    public MetricEntry(String metric, String context, String metricsdesc, String request) {
//        this.metric = metric;
//        this.context = context;
//        this.metricsdesc = metricsdesc;
//        this.request = request;
//    }

    public MetricEntry(String context, String metricsdesc, String request) {
        this.context = context;
        this.metricsdesc = metricsdesc;
        this.request = request;
    }

    public String getContext() {
        return context;
    }

    public void setContext(String context) {
        this.context = context;
    }

    public String getMetricsdesc() {
        return metricsdesc;
    }

    public void setMetricsdesc(String metricsdesc) {
        this.metricsdesc = metricsdesc;
    }

    public String getRequest() {
        return request;
    }

    public void setRequest(String request) {
        this.request = request;
    }
}
