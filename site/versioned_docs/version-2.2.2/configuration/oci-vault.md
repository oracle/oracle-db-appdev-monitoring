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

### OCI Vault CLI Configuration

If using the default database with CLI parameters, the exporter will read the username and password from a secret stored in OCI Vault if you set these two environment variables:

- `OCI_VAULT_ID` should be set to the OCID of the OCI vault that you wish to use
- `OCI_VAULT_USERNAME_SECRET` should be set to the name of the secret in the OCI vault which contains the database username
- `OCI_VAULT_PASSWORD_SECRET` should be set to the name of the secret in the OCI vault which contains the database password

> Note that the process must be running under a user that has the OCI CLI installed and configured correctly to access the desired tenancy and region. The OCI Profile used is `DEFAULT`.
