#!/bin/bash
## Copyright (c) 2021 Oracle and/or its affiliates.
## Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

SCRIPT_DIR=$(dirname $0)

if [ -z "$DOCKER_REGISTRY" ]; then
    echo "DOCKER_REGISTRY not set. Will get it with state_get"
  export DOCKER_REGISTRY=$(state_get DOCKER_REGISTRY)
fi

if [ -z "$DOCKER_REGISTRY" ]; then
    echo "Error: DOCKER_REGISTRY env variable needs to be set!"
    exit 1
fi

if [ -z "$ORDER_DB_NAME" ]; then
    echo "ORDER_DB_NAME not set. Will get it with state_get"
  export ORDER_DB_NAME=$(state_get ORDER_DB_NAME)
fi

if [ -z "$ORDER_DB_NAME" ]; then
    echo "Error: ORDER_DB_NAME env variable needs to be set!"
    exit 1
fi

echo create configmap for db-metrics-banka-exporter...
kubectl delete configmap db-metrics-banka-exporter-config -n msdataworkshop
kubectl create configmap db-metrics-banka-exporter-config --from-file=db-metrics-banka-exporter-metrics.toml -n msdataworkshop
echo
echo create db-metrics-exporter deployment and service...
export CURRENTTIME=generated
#export CURRENTTIME=$( date '+%F_%H:%M:%S' )
echo CURRENTTIME is $CURRENTTIME  ...this will be appended to generated deployment yaml

cp db-metrics-exporter-deployment.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml

#sed -e  "s|%DOCKER_REGISTRY%|${DOCKER_REGISTRY}|g" db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml > /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
#mv -- /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
sed -e  "s|%EXPORTER_NAME%|example|g" db-metrics-exporter-banka-deployment-${CURRENTTIME}.yaml > /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
mv -- /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
sed -e  "s|%PDB_NAME%|${ORDER_DB_NAME}|g" db-metrics-exporter-banka-deployment-${CURRENTTIME}.yaml > /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
mv -- /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
sed -e  "s|%USER%|aquser|g" db-metrics-exporter-banka-deployment-${CURRENTTIME}.yaml > /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
mv -- /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
sed -e  "s|%db-wallet-secret%|order-db-tns-admin-secret|g" db-metrics-exporter-banka-deployment-${CURRENTTIME}.yaml > /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
mv -- /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
#sed -e  "s|${OCI_REGION-}|${OCI_REGION}|g" db-metrics-exporter-banka-deployment-${CURRENTTIME}.yaml > /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
#mv -- /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
#sed -e  "s|${VAULT_SECRET_OCID-}|${VAULT_SECRET_OCID}|g" db-metrics-exporter-banka-deployment-${CURRENTTIME}.yaml > /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml
#mv -- /tmp/db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml db-metrics-exporter-banka-deployment-$CURRENTTIME.yaml


kubectl delete configmap observability-exporter-example-config -n msdataworkshop

kubectl create configmap observability-exporter-example-config --from-file=aq-metrics.toml -n msdataworkshop

kubectl apply -f observability-exporter-example-deployment-test.yaml -n msdataworkshop

kubectl apply -f observability-exporter-example-service.yaml -n msdataworkshop

kubectl apply -f observability-exporter-example-service-monitor.yaml -n msdataworkshop
