package oracle.observability.metrics;

import java.util.Arrays;

public class Metric {


    MetricEntry[] metricEntries;
    public Metric() {

    }
    public Metric(MetricEntry[] metricEntries) {
        this.metricEntries = metricEntries;
    }

    public MetricEntry[] getMetricEntries() {
        return metricEntries;
    }

    public void setMetricEntries(MetricEntry[] metricEntries) {
        this.metricEntries = metricEntries;
    }

    @Override
    public String toString() {
        return "Metric{" +
                "metricEntries=" + Arrays.toString(metricEntries) +
                '}';
    }
}
