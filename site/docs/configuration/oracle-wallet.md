---
title: Authentication and Oracle Wallet (mTLS)
sidebar_position: 4
---

# Authentication Overview

The Oracle AI Database Metrics Exporter supports the following authenticaiton mechanisms:

- Username/password authentication using the config file.
- Wallet authentication with TLS/mTLS, including using the wallet as a Secure External Password Store (SEPS).
- Loading database passwords from a file or external secret storage (OCI Vault, Azure Vault, or HashiCorp Vault).
- OS Auth, or "External Auth".

### Username/password authentication

```yaml
databases:
  mydb:
    url: mydb:1521/service_name
    username: myuser
    password: mypassword
```

You may also supply the username and password with environment variables:

```yaml
databases:
  mydb:
    url: mydb:1521/service_name
    username: ${DB_USERNAME}
    password: ${DB_PASSWORD}
```

### Using a password file

The exporter may also read the database password from a file. The file should contain only the database password with no leading or trailing characters.
```yaml
databases:
  mydb:
    url: mydb:1521/service_name
    username: ${DB_USERNAME}
    ## Database password file
    ## If specified, will load the database password from a file.
    passwordFile: ${DB_PASSWORD_FILE}
```

### Using an external secret store

The exporter supports loading database username and password information from the following secret store implementations. Refer to the corresponding documentation links for individual configuration:

- [OCI Vault](./oci-vault.md)
- [Azure Vault](./azure-vault.md)
- [HashiCorp Vault](./hashicorp-vault.md)

### Using a Wallet 

For mutual TLS (mTLS) connections, you must use an Oracle Wallet.

If you are using Oracle Autonomous Database with mTLS, for example, you can download and unzip the wallet from the Oracle Cloud Infrastructure (OCI) console.

To configure the exporter to use an Oracle Wallet,

1. Set the `TNS_ADMIN` environment variable to the directory containing the unzipped wallet
2. Configure your database instance with the appropriate database TNS name:

```yaml
databases:
  mydb:
    username: admin
    password: <REPLACE ME>
    # TNS Name from wallet tnsnames.ora file, e.g., mydb_high
    url: <TNS Name>
```

If you are running the exporter as a container, you can mount the wallet as a volume. For example, mounting the `./wallet` directory to the `/wallet` location:

```bash
docker run -it --rm \
    -e DB_USERNAME=pdbadmin \
    -e DB_PASSWORD=Welcome12345 \
    -e DB_CONNECT_STRING=devdb_tp \
    -v ./wallet:/wallet \
    -p 9161:9161 \
    container-registry.oracle.com/database/observability-exporter:2.2.2
```

### Oracle Wallet SEPS configuration



```bash
# Create the wallet
orapki wallet create -wallet <wallet location> -pwd <wallet_password> -auto_login

mkstore -wrl <wallet location> -createCredential <alias name> <common user> <account_password>
```

```yaml
databases:
  mydb:
    url: mydb_high
    # Do not provide a username and password, indicating to the exporter
    # that credentials should be loaded from the Oracle Wallet
    # username:
    # password:
   
    # ... remaining database configuration for "mydb"
```

### mTLS for multiple databases with Oracle Wallet

The Oracle AI Database Metrics exporter uses ODPI-C, which can only initalize the TNS aliases from a `tnsnames.ora` file once per process. To work around this, the exporter can be configured to read from a "combined" `tnsnames.ora` file containing all TNS aliases for connections in a multi-database configuration.

1. For each database the exporter will connect to, download the corresponding wallet files. If you're using ADB/ATP-S, download the regional wallet instead of the instance wallet if the databases are in the same region.

2. Copy the TNS aliases the `tnsnames.ora` file from each wallet, and combine them into one file, so all your database service names are in one file together

3. In the combined `tnsnames.ora` file, and add the following snippet to each TNS alias connection string, to tell the client where the wallet directory is:

```
(security=(MY_WALLET_DIRECTORY=/path/to/this/database/wallet))
```

The combined `tnsnames.ora` file, which contains the TNS aliases for both databases, and their corresponding wallet location in the `security` configuration will look something like the following:

```sql
db1_high = (description= (retry_count=20)(retry_delay=3)(address=(protocol=tcps)(port=1522)(host=adb.****.oraclecloud.com))(connect_data=(service_name=****.adb.oraclecloud.com))(security=(MY_WALLET_DIRECTORY=/wallets/db1)(ssl_server_dn_match=yes)))

db2_high = (description= (retry_count=20)(retry_delay=3)(address=(protocol=tcps)(port=1522)(host=adb.****.oraclecloud.com))(connect_data=(service_name=****.adb.oraclecloud.com))(security=(MY_WALLET_DIRECTORY=/wallets/db2)(ssl_server_dn_match=yes)))
```

4. Take wallet files (cwallet.sso, ewallet.p12, & ewallet.pem) for each database, and place them in separate directories. For example, `db1` gets its own directory, `db2` gets its own directory, and so forth.

The resulting directory structure should look like the following, with wallet information separate from the combined `tnsnames.ora` file:

```
wallets
├── combined
│   ├── sqlnet.ora
│   └── tnsnames.ora // Combined tnsnames.ora
├── db1
│   ├── cwallet.sso
│   ├── ewallet.p12
│   └── ewallet.pem
└── db2
├── cwallet.sso
├── ewallet.p12
└── ewallet.pem
```

5. Set the `TNS_ADMIN` environment variable where the exporter is running to the directory containing your combined `tnsnames.ora` file:

```
export TNS_ADMIN=/wallets/combined
```

6. Finally, update the exporter configuration file to include the TNS aliases for all databases you will be connecting to. Ensure your database configuration file does not use the `tnsAdmin` property, as we are using the global `TNS_ADMIN` environment variable to point to the combined `tnsnames.ora` file:

```yaml
databases:
    db2:
        username: ****
        password: ****
        url: db2_high
        queryTimeout: 5
        maxOpenConns: 10
        maxIdleConns: 10
    db1:
        username: ****
        password: ****
        url: db1_high
        queryTimeout: 5
        maxOpenConns: 10
        maxIdleConns: 10
```

Then, run the exporter with the config file:

```shell
./oracledb_exporter --config.file=my-config-file.yaml
```

### OS Auth, or "External Auth"

The exporter supports Oracle AI Databaes OS Auth for passwordless authentication. To enable OS Auth from exporter, provide only the database username:

```yaml
databases:
  mydb:
    url: mydb_high
    username: exporter_user
    # Do not provide a password, indicating to the exporter
    # that OS Auth should be used
    # password:
   
    # ... remaining database configuration for "mydb"
```