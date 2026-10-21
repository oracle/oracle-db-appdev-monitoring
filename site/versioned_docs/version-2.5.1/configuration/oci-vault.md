---
title: OCI Vault
sidebar_position: 6
---

# Oracle Cloud Infrastructure (OCI) Vault

Securely load database credentials from OCI Vault.

Each database in the config file may be configured to use OCI Vault. To load the database username and/or password from OCI Vault, set the `vault.oci` property to contain the OCI Vault OCID, and secret names for the database username/password:

```yaml
databases:
  mydb:
    vault:
      oci:
        id: <VAULT OCID>
        usernameSecret: <Secret containing DB username>
        passwordSecret: <Secret containing DB password>
```

## Config file

Use `config_file` when the exporter should authenticate with the OCI Go SDK default configuration provider. This reads the local OCI config file, such as `$HOME/.oci/config`, unless the SDK is configured otherwise. If `auth` is omitted, `config_file` is used for backward compatibility.

### Configuration in exporter

```yaml
databases:
  mydb:
    vault:
      oci:
        id: <VAULT OCID>
        auth: config_file
        usernameSecret: <Secret containing DB username>
        passwordSecret: <Secret containing DB password>
```

### Sample policy

```text
Allow group metrics-exporter-users to read secret-bundles in compartment observability
```

## Instance principal

Use `instance_principal` when the exporter runs on an OCI Compute instance and should authenticate as the instance.

### Configuration in exporter

```yaml
databases:
  mydb:
    vault:
      oci:
        id: <VAULT OCID>
        auth: instance_principal
        usernameSecret: <Secret containing DB username>
        passwordSecret: <Secret containing DB password>
```

### Sample policy

```text
Allow dynamic-group metrics-exporter-instances to read secret-bundles in compartment observability
```

## Resource principal

Use `resource_principal` when the exporter runs in an OCI service that exposes resource principal credentials.

### Configuration in exporter

```yaml
databases:
  mydb:
    vault:
      oci:
        id: <VAULT OCID>
        auth: resource_principal
        usernameSecret: <Secret containing DB username>
        passwordSecret: <Secret containing DB password>
```

### Sample policy

```text
Allow dynamic-group metrics-exporter-resources to read secret-bundles in compartment observability
```

## Workload identity

Use `workload_identity` when the exporter runs in OKE with workload identity configured.

### Configuration in exporter

```yaml
databases:
  mydb:
    vault:
      oci:
        id: <VAULT OCID>
        auth: workload_identity
        usernameSecret: <Secret containing DB username>
        passwordSecret: <Secret containing DB password>
```

When `auth: workload_identity` is selected, the exporter uses the OCI Go SDK workload identity provider. The exporter does not set OCI SDK environment variables at runtime, so configure the exporter process with:

- `OCI_RESOURCE_PRINCIPAL_VERSION`: set to `2.2`
- `OCI_RESOURCE_PRINCIPAL_REGION`: set to the OCI region that contains the Vault, such as `us-ashburn-1`

For example, in a Kubernetes deployment:

```yaml
env:
  - name: OCI_RESOURCE_PRINCIPAL_VERSION
    value: "2.2"
  - name: OCI_RESOURCE_PRINCIPAL_REGION
    value: us-ashburn-1
```

In OKE, the OCI SDK also uses the pod service account token, service account CA certificate, and Kubernetes service host provided by the runtime:

- `KUBERNETES_SERVICE_HOST`
- `/var/run/secrets/kubernetes.io/serviceaccount/token`
- `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`

If the service account CA certificate is mounted at a custom path, set `OCI_KUBERNETES_SERVICE_ACCOUNT_CERT_PATH` to that path.

### Sample policy

```text
Allow any-user to read secret-bundles in compartment observability where all {
  request.principal.type = 'workload',
  request.principal.namespace = 'monitoring',
  request.principal.service_account = 'metrics-exporter',
  request.principal.cluster_id = 'ocid1.cluster.oc1.iad.example'
}
```

## Policy conditions

The exporter reads secret bundle values from OCI Vault, so the principal used by the selected authentication mode needs permission to read `secret-bundles` in the compartment that contains the secrets.

To limit access to a specific vault or secret name, add a policy condition:

```text
Allow dynamic-group metrics-exporter-instances to read secret-bundles in compartment observability
  where target.vault.id = 'ocid1.vault.oc1..example'

Allow dynamic-group metrics-exporter-instances to read secret-bundles in compartment observability
  where target.secret.name = 'metrics-exporter-db-password'
```

Replace the compartment, group, dynamic group, namespace, service account, cluster OCID, vault OCID, and secret name with values from your environment.
