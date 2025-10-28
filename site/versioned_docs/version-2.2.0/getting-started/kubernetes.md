---
title: Kubernetes
sidebar_position: 3
---

# Kubernetes

You can run the exporter in Kubernetes using provided manifests.

To run the exporter in Kubernetes, you must complete the following steps.  All steps must be completed in the same Kunernetes namespace.  The examples below assume you want to use a namespace called `exporter`, you must change the commands if you wish to use a different namespace.

### Create a secret with credentials for connecting to the Oracle Database

Create a secret with the Oracle database user and password that the exporter should use to connect to the database using this command.  You must specify the correct user and password for your environment.  This example uses `pdbadmin` as the user and `Welcome12345` as the password:

```bash
kubectl create secret generic db-secret \
    --from-literal=username=pdbadmin \
    --from-literal=password=Welcome12345 \
    -n exporter
```

### Create a config map for the exporter configuration file (recommended)

Create a config map with the exporter configuration file (if you are using one) using this command:

```bash
kubectl create cm metrics-exporter-config \
    --from-file=metrics-exporter-config.yaml
```

> NOTE: It is strongly recommended to migrate to the new config file if you are running version 2.0.0 or later.


### Create a config map for the wallet (optional)

Create a config map with the wallet (if you are using one) using this command.  Run this command in the `wallet` directory you created earlier.

```bash
kubectl create cm db-metrics-tns-admin \
    --from-file=cwallet.sso \
    --from-file=ewallet.p12 \
    --from-file=ewallet.pem \
    --from-file=keystore.jks \
    --from-file=ojdbc.properties \
    --from-file=sqlnet.ora \
    --from-file=tnsnames.ora \
    --from-file=truststore.jks \
    -n exporter
```

### Create a config map for your metrics definition file (optional)

If you have defined any [custom metrics](../configuration/custom-metrics.md), you must create a config map for the metrics definition file.  For example, if you created a configuration file called `txeventq-metrics.toml`, then create the config map with this command:

```bash
kubectl create cm db-metrics-txeventq-exporter-config \
    --from-file=txeventq-metrics.toml \
    -n exporter
```

### Deploy the Oracle Database Observability exporter

A sample Kubernetes manifest is provided [here](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/kubernetes/metrics-exporter-deployment.yaml).  You must edit this file to set the namespace you wish to use, the database connect string to use, and if you have any custom metrics, you will need to uncomment and customize some sections in this file.

Once you have made the necessary updates, apply the file to your cluster using this command:

```bash
kubectl apply -f metrics-exporter-deployment.yaml
```

You can check the deployment was successful and monitor the exporter startup with this command:

```bash
kubectl get pods -n exporter -w
```

You can view the exporter's logs with this command:

```bash
kubectl logs -f svc/metrics-exporter -n exporter
```

### Create a Kubernetes service for the exporter

Create a Kubernetes service to allow access to the exporter pod(s).  A sample Kubernetes manifest is provided [here](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/kubernetes/metrics-exporter-service.yaml).  You may need to customize this file to update the namespace.

Once you have made any necessary udpates, apply the file to your cluster using this command:

```bash
kubectl apply -f metrics-exporter-service.yaml
```

### Create a Kubernetes service monitor

Create a Kubernetes service monitor to tell Prometheus (for example) to collect metrics from the exporter.  A sample Kubernetes manifest is provided [here](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/kubernetes/metrics-service-monitor.yaml).  You may need to customize this file to update the namespace.

Once you have made any necessary udpates, apply the file to your cluster using this command:

```bash
kubectl apply -f metrics-service-monitor.yaml
```

### Configure a Prometheus target (optional)

You may need to update your Prometheus configuration to add a target.  If so, you can use this example job definition as a guide:

```yaml
  - job_name: 'oracle-exporter'
    metrics_path: '/metrics'
    scrape_interval: 15s
    scrape_timeout: 10s
    static_configs:
    - targets: 
      - metrics-exporter.exporter.svc.cluster.local:9161
```
