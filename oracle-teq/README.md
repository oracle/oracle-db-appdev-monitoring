# Monitoring your Oracle Transactional Event Queues

As you may know [Oracle Transactional Event Queues](https://docs.oracle.com/en/database/oracle/oracle-database/21/adque/index.html) is a fault-tolerant, scalable, real-time messaging backbone offered by converged Oracle Database that allows you to build an enterprise-class event-driven architectures. 

Have access to the real-time broker, producer, and consumer metrics in a single-pane dashboard and receive alerts for only important issues is key for allows teams to watch and understand the state of their systems.

Oracle Observability Exporter gather the Oracle TEQ metrics and expose through HTTP endpoint as a _target_ to Prometheus collecting and handle them.

## Solution overview

The solution in this session is part of the Unified Observability strategy for Oracle Database but will focus only in Oracle TEQ Metrics handling the two following issues:

- Gather the Oracle TEQ metrics and expose them through an HTTP endpoint using Prometheus standard using Oracle Observability Exporter.

- Deploy Prometheus and Grafana stack to create a single-pane of glass to present Oracle TEQ Metrics on a Grafana Dashboard.

![Solution overview for Metrics](images/unified-observability-metrics.png " ")

## Walkthrough

We provide a method to configure this solution setting up the resources following high-level steps:

1. Create an Oracle TEQ topic.
2. Create Oracle Database secret inside Kubernetes needed for the solution.
3. Create ConfigMap to store TNS_ADMIN data needed to connect with Oracle Database.
4. Create ConfigMap to store Oracle TEQ default metrics configuration file.
5. Deploy Oracle Database Observability Exporter inside Kubernetes.
6. Deploy Service and Monitor Services.
7. Set up Grafana Dashboard to load Oracle TEQ Metrics.

## Prerequisites

For this walkthrough, the following prerequisites are necessary:

- An Oracle Databsase 21c instance running.
- An Oracle User with needed grants to create Oracle TEQ and execute AQ JMS commands.
- A Kubernetes infrastructure running with Grafana and Prometheus instances running and configured.

---

## Set up proactive monitoring in Oracle TEQ

In this section, we walk through the process to set up the resources for active monitoring Oracle TEQ.

### Create an Oracle TEQ topic

We first create a standard Oracle TEQ topic and subscribe to it in order to receive events. We will connect to Oracle Database and execute the script [createTEQ.sql](scripts/createTEQ.sql)

```bash
# Access Oracle Database using SQLcl tool.
sql /nolog
```

```sql
-- Connect using Oracle Database User with needed grants
SQLcl: Release 21.4 Production on Tue Jan 25 00:10:07 2022

Copyright (c) 1982, 2022, Oracle.  All rights reserved.

SQL> connect 'DB USER'@'DATABASE HOST:PORT'/'DB SERVICE NAME''
Password? (**********?) ****************
Connected.
```

```sql
--- Run script to create Oracle TEQ Topic.
SQL> @/scripts/createTEQ.sql
SQL>
SQL> begin
 2   -- create the TEQ
 3   dbms_aqadm.create_transactional_event_queue(
 4       queue_name         => 'my_teq',
 5       -- when mutiple_consumers is true, this will create a pub/sub "topic" - the default is false
 6       multiple_consumers => true
 7   );
 8   
 9   -- start the TEQ
10    dbms_aqadm.start_queue(
11        queue_name         => 'my_teq'
12    ); 
13 end;
14 /
...
...
...

PL/SQL procedure successfully completed.
```

If you already have an existing TEQ topic that you want to use, you can skip to the next step.

### Create Oracle Database secret

This steps illustrates how to create the K8s secret to store the Oracle Database User credentials that is used by Oracle Observability Exporter.

```bash
kubectl create secret generic db-secret         \
        --from-literal=username=<"DB USERNAME"> \
        --from-literal=password=<"DB PASSWORD"> \
        --namespace <"NAMESPACE">
```

> Attention: We assume that Exporter will be deployed in the K8s namespace "NAMESPACE."

### Create ConfigMap to store TNS_ADMIN

This steps illustrates how to create the K8s ConfigMap to store the Oracle Database TNS data that is used by Oracle Observability Exporter.

```bash
kubectl create configmap db-metrics-tns-admin \
        --from-file=TNS_ADMIN/tnsnames.ora    \
        --namespace <"NAMESPACE">
```

### Create ConfigMap to store TEQ Metrics Configuration

This steps illustrates how to create the K8s ConfigMap to store the TEQ metrics configuration file that is used by Oracle Observability Exporter.

```bash
kubectl create configmap db-metrics-teq-exporter-config  \
        --from-file=metrics/default-metrics-teq.toml     \
        --namespace <"NAMESPACE">
```

### Deploy Oracle Database Observability Exporter

This steps illustrates how to create the K8s ConfigMap to store the TEQ metrics configuration file that is used by Oracle Observability Exporter.

```bash
kubectl create -f teq-metrics-exporter-deployment.yaml  \
        --namespace <"NAMESPACE">
```

### Deploy Service and Monitor Services

```bash
kubectl create -f teq-metrics-exporter-service.yaml  \
        --namespace <"NAMESPACE">
```

```bash
kubectl create -f teq-metrics-exporter-monitor.yaml  \
        --namespace <"NAMESPACE">
```

### Set up Grafana Dashboard
