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

### Authentication

By default, OCI Vault authentication uses the OCI Go SDK default configuration provider. This reads the local OCI config file, such as `$HOME/.oci/config`, unless the SDK is configured otherwise:

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

If the exporter runs on an OCI Compute instance, you may use instance principal authentication instead:

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

If the exporter runs in an OCI service that exposes resource principal credentials, use resource principal authentication:

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

If the exporter runs in OKE with workload identity configured, use workload identity authentication:

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

The accepted `auth` values are `config_file`, `instance_principal`, `resource_principal`, and `workload_identity`. If `auth` is omitted, `config_file` is used for backward compatibility.

Whichever OCI authentication mode you choose, the principal must have IAM policy permission to read the target Vault secret bundle.

### Deprecated OCI Vault CLI Configuration

If using the default database with CLI parameters, the exporter will read the username and password from a secret stored in OCI Vault if you set these environment variables:

- `OCI_VAULT_ID` should be set to the OCID of the OCI vault that you wish to use
- `OCI_VAULT_USERNAME_SECRET` should be set to the name of the secret in the OCI vault which contains the database username
- `OCI_VAULT_PASSWORD_SECRET` should be set to the name of the secret in the OCI vault which contains the database password

This CLI environment-variable configuration path is deprecated and kept only for backward compatibility. It always uses the OCI SDK default config-file authentication behavior and does not support the `auth` selector. Use the exporter config file to select instance principal, resource principal, or workload identity authentication.
