package oracle.observability.metrics;

import io.prometheus.client.CollectorRegistry;
import io.prometheus.client.Gauge;

import java.util.HashMap;
import java.util.Map;

public class CollectorRegistryWithGaugeMap extends CollectorRegistry {
    Map<String, Gauge> gaugeMap = new HashMap<>();

}
