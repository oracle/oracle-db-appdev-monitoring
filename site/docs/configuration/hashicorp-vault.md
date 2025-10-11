---
title: HashiCorp Vault
sidebar_position: 8
---

# HashiCorp Vault

Securely load database credentials from HashiCorp Vault.

Each database in the config file may be configured to use HashiCorp Vault. To load the database username and/or password from HashiCorp Vault, set the `vault.hashicorp` property to contain the following information:

```yaml
databases:
  mydb:
    vault:
      hashicorp:
        proxySocket: /var/run/vault/vault.sock
        mountType: secret engine type, currently either "kvv1" or "kvv2"
        mountName: secret engine mount path
        secretPath: path of the secret
        usernameAttribute: name of the JSON attribute, where to read the database username, if ommitted defaults to "username"
        passwordAttribute: name of the JSON attribute, where to read the database password, if ommitted defaults to "password"
```

Example

```yaml
databases:
  mydb:
    vault:
      hashicorp:
        proxySocket: /var/run/vault/vault.sock
        mountType: kvv2
        mountName: dev
        secretPath: oracle/mydb/monitoring
```

### Authentication

In this first version it currently only supports queries via HashiCorp Vault Proxy configured to run on the local host and listening on a Unix socket. Currently also required use_auto_auth_token option to be set.
Will expand the support for other methods in the future.

Example Vault Proxy configuration snippet:

```
listener "unix" {
    address = "/var/run/vault/vault.sock"
    socket_mode = "0660"
    socket_user = "vault"
    socket_group = "vaultaccess"
    tls_disable = true
}

api_proxy {
    # This always uses the auto_auth token when communicating with Vault server, even if client does not send a token
    use_auto_auth_token = true
}
```
