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
        mountType: "kvv1", "kvv2", "database" or "logical"
        mountName: secret engine mount path
        secretPath: path of the secret or database role name
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

### Dynamic database credentials

Instead of fixed database credentials Vault also supports dynamic credentials that are created every time application requests them. This
makes sure the credentials always have a short time-to-live and even if they leak, they quickly become invalid.

Follow [Vault documentation on how to set up Oracle database plugin for Vault](https://developer.hashicorp.com/vault/docs/secrets/databases/oracle).

A few additional notes about connecting exporter to CDB. NB! Below are just example commands, adjust them to fit your environment.

When setting up connection to CDB, then also need to edit "username_template" parameter, so Vault would create a C## common user for exporter.

```sh
vault write database/config/mydb \
    plugin_name=vault-plugin-database-oracle \
    allowed_roles="mydb_exporter" \
    connection_url='{{username}}/{{password}}@//172.17.0.3:1521/FREE' \
    username_template='{{ printf "C##V_%s_%s_%s_%s" (.DisplayName | truncate 8) (.RoleName | truncate 8) (random 20) (unix_time) | truncate 30 | uppercase | replace "-" "_" | replace "." "_" }}' \
    username='c##vaultadmin' \
    password='vaultadmin'
```

Since Vault is creating common users in CDB, it needs to have CREATE/ALTER/DROP USER privileges on all containers. Here is a modification of the documented Vault Oracle plugin admin user privileges.

```sql
GRANT CREATE USER to c##vaultadmin WITH ADMIN OPTION container=all;
GRANT ALTER USER to c##vaultadmin WITH ADMIN OPTION container=all;
GRANT DROP USER to c##vaultadmin WITH ADMIN OPTION container=all;
GRANT CREATE SESSION to c##vaultadmin WITH ADMIN OPTION;
GRANT SELECT on gv_$session to c##vaultadmin;
GRANT SELECT on v_$sql to c##vaultadmin;
GRANT ALTER SYSTEM to c##vaultadmin WITH ADMIN OPTION;
```

Create no authentication user in Oracle database, that has actual monitoring privileges.

```sql
CREATE USER c##exporter NO AUTHENTICATION;
GRANT create session TO c##exporter;
GRANT all necessary privileges that Exporter needs TO c##exporter;
```

Create database role in Vault:

```sh
vault write database/roles/mydb_exporter \
    db_name=mydb \
    creation_statements='CREATE USER {{username}} IDENTIFIED BY "{{password}}"; GRANT CREATE SESSION TO {{username}}; ALTER USER c##exporter GRANT CONNECT THROUGH {{username}};' \
    default_ttl="7d" \
    max_ttl="10d"
```

NB! Make sure to restart Exporter before TTL above expires, this will fetch new database credentials. When TTL expires, Vault will drop the dynamically created database users.

And create database config in Exporter:

```yaml
databases:
  mydb:
    vault:
      hashicorp:
        proxySocket: /var/run/vault/vault.sock
        mountType: database
        mountName: database
        secretPath: mydb_exporter
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
