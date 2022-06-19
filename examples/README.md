# Observability Exporter Example


Please refer to the Unified Observability in Grafana with converged Oracle Database Workshop at http://bit.ly/unifiedobservability and it's corresponding repos https://github.com/oracle/microservices-datadriven/tree/main/grabdish/observability/db-metrics-exporter for complete examples.

More examples will be provided here in the near future.

# Metrics exporter 

1. Pre-req. Run setup for the GrabDish workshop including observability lab steps to install and configure Grafana and Prometheus
2. Run `./deploy.sh` in this directory
3. `curl http://observability-exporter-example:8080/metrics` from within cluster to see Prometheus stats
4. View same stats from within Grafana by loading AQ dashboard

The same can be done above for TEW by simply replace `aq` with `teq` in the deployment and configmap yamls

Troubleshooting...

kubectl port-forward prometheus-stable-kube-prometheus-sta-prometheus-0 -n msdataworkshop 9090:9090

# Logs exporter

# Trace exporter

# Combined Metrics, Logs, and Trace exporter 