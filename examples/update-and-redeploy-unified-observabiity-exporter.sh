#!/bin/bash
## Copyright (c) 2021 Oracle and/or its affiliates.
## Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# add namespace if/as appropriate, eg `kubectl apply -f unified-observability-exporter-deployment.yaml-n mynamespace`

echo delete previous deployment so that deployment is reapplied/deployed after configmap changes for exporter are made...
kubectl delete deployment db-metrics-exporter-orderpdb

echo create configmap for unified-observability-exporter...
kubectl delete configmap unified-observability-exporter-config
kubectl create configmap unified-observability-exporter-config --from-file=unified-observability-%EXPORTER_NAME%-exporter-metrics.toml

kubectl apply -f unified-observability-exporter-deployment.yaml
# the following are unnecessary after initial deploy but in order to keep to a single bash script...
kubectl apply -f unified-observability-exporter-service.yaml
kubectl apply -f unified-observability-exporter-servicemonitor.yaml
