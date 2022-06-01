# Observability Exporter Example

# Metrics exporter 

1. Pre-req. Run setup for the GrabDish workshop including observability lab steps to install and configure Grafana and Prometheus
2. Run `./deploy.sh` in this directory
3. `curl http://observability-exporter-example:8080/metrics` from within cluster to see Prometheus stats
4. View same stats from within Grafana by loading AQ dashboard

The same can be done above for teq by simply replace `aq` with `teq` in the deployment and configmap yamls

Troubleshooting...

kubectl port-forward prometheus-stable-kube-prometheus-sta-prometheus-0 -n msdataworkshop 9090:9090

# Logs exporter

# Trace exporter


# Combined Metrics, Logs, and Trace exporter 