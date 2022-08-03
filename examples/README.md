# Observability Exporter Example


Please refer to the Unified Observability in Grafana with converged Oracle Database Workshop at http://bit.ly/unifiedobservability and it's corresponding repos https://github.com/oracle/microservices-datadriven/tree/main/grabdish/observability/db-metrics-exporter for complete examples.

A simple setup in Kubernetes involves the following steps (with the assumption that Prometheus is already installed)

1. Change the %EXPORTER_NAME% value in all yaml files in this directory. This can be any value such as "helloworld".

2. Change the database connection information in the unified-observability-exporter-deployment.yaml file.
   - The only value required is the DATA_SOURCE_NAME which takes the format `USER/PASSWORD@DB_SERVICE_URL`
   - In the example the connection information is obtained from a mount created from the wallet obtained from a Kubernetes secret named `%db-wallet-secret%`
   - In the example the password is obtained from a Kubernetes secret named `dbuser`

3. Copy a config file to unified-observability-%EXPORTER_NAME%-exporter-metrics.toml in currently directly
   - Eg, `cp ../metrics/aq-metrics.toml unified-observability-helloworld-exporter-metrics.toml`
   - This will be used to create a configmap that is referenced in the deployment.
   
4. Run `./update-and-redeploy-unified-observabiity-exporter.sh`

5. You should see metrics being exported from within the container at http://localhost:9161/metrics and likewise from the Kubnernetes service at http://unified-observability-exporter-service-%EXPORTER_NAME%:9161/metrics

More examples will be provided here in the near future.
