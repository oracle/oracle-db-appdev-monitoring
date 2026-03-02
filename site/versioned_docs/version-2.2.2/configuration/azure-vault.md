---
title: Azure Vault
sidebar_position: 7
---

# Azure Vault

Securely load database credentials from Azure Vault.

Each database in the config file may be configured to use Azure Vault. To load the database username and/or password from Azure Vault, set the `vault.azure` property to contain the Azure Vault ID, and secret names for the database username/password:

```yaml
databases:
  mydb:
    vault:
      azure:
        id: <VAULT ID>
        usernameSecret: <Secret containing DB username>
        passwordSecret: <Secret containing DB password>
```

### Authentication

If you are running the exporter outside Azure, we recommend using [application service principal](https://learn.microsoft.com/en-us/azure/developer/go/sdk/authentication/authentication-on-premises-apps).

If you are running the exporter inside Azure, we recommend using a [managed identity](https://learn.microsoft.com/en-us/azure/developer/go/sdk/authentication/authentication-azure-hosted-apps).

You should set the following additional environment variables to allow the exporter to authenticate to Azure:

- `AZURE_TENANT_ID` should be set to your tenant ID
- `AZURE_CLIENT_ID` should be set to the client ID to authenticate to Azure
- `AZURE_CLIENT_SECRET` should be set to the client secret to authenticate to Azure

### Azure Vault CLI Configuration (without exporter config file)

If using the default database with CLI parameters, the exporter will read the database username and password from secrets stored in Azure Key Vault if you set these environment variables:

- `AZ_VAULT_ID` should be set to the ID of the Azure Key Vault that you wish to use
- `AZ_VAULT_USERNAME_SECRET` should be set to the name of the secret in the Azure Key Vault which contains the database username
- `AZ_VAULT_PASSWORD_SECRET` should be set to the name of the secret in the Azure Key Vault which contains the database password