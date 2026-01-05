---
title: Multiple Databases
sidebar_position: 3
---

# Scraping Multiple Databases

The exporter may be configured to scrape as many databases as needed.

You may scrape as many databases as needed by defining named database configurations in the config file. 

If you're connecting to multiple databases using mTLS, see [mTLS for multiple databases with Oracle Wallet](./oracle-wallet.md#mtls-for-multiple-databases-with-oracle-wallet)

The following settings configure the exporter to scrape multiple databases, "db1", and "db2", simultaneously:

```yaml
# Example Oracle AI Database Metrics Exporter Configuration file.
# Environment variables of the form ${VAR_NAME} will be expanded.

databases:
  ## Path on which metrics will be served
  # metricsPath: /metrics

  ## As many named database configurations may be defined as needed.
  ## It is recommended to define your database config in the config file, rather than using CLI arguments.

  ## Database connection information for the "db1" database.
  db1:
    ## Database username
    username: ${DB1_USERNAME}
    ## Database password
    password: ${DB1_PASSWORD}
    ## Database connection url
    url: localhost:1521/freepdb1

    ## Metrics query timeout for this database, in seconds
    queryTimeout: 5

    ## Rely on Oracle AI Database External Authentication by network or OS
    # externalAuth: false
    ## Database role
    # role: SYSDBA
    ## Path to Oracle AI Database wallet, if using wallet
    # tnsAdmin: /path/to/database/wallet

    ### Connection settings:
    ### Either the go-sql or Oracle AI Database connection pool may be used.
    ### To use the Oracle AI Database connection pool over the go-sql connection pool,
    ### set maxIdleConns to zero and configure the pool* settings.

    ### Connection pooling settings for the go-sql connection pool
    ## Max open connections for this database using go-sql connection pool
    maxOpenConns: 10
    ## Max idle connections for this database using go-sql connection pool
    maxIdleConns: 10

    ### Connection pooling settings for the Oracle AI Database connection pool
    ## Oracle AI Database connection pool increment.
    # poolIncrement: 1
    ## Oracle AI Database Connection pool maximum size
    # poolMaxConnections: 15
    ## Oracle AI Database Connection pool minimum size
    # poolMinConnections: 15

    ### Arbitrary labels to add to each metric scraped from this database
    ## Any labels configured for one database will be added to metrics from
    ## every database, because the same metric names must always have the same
    ## full labelset. If the label isn't set for a particular database, then it
    ## will just be set to an empty string.
    # labels:
    #   label_name1: label_value1
    #   label_name2: label_value2

  db2:
    ## Database username
    username: ${DB2_USERNAME}
    ## Database password
    password: ${DB2_PASSWORD}
    ## Database connection url
    url: localhost:1522/freepdb1

    ## Metrics query timeout for this database, in seconds
    queryTimeout: 5

    ## Rely on Oracle AI Database External Authentication by network or OS
    # externalAuth: false
    ## Database role
    # role: SYSDBA
    ## Path to Oracle AI Database wallet, if using wallet
    # tnsAdmin: /path/to/database/wallet

    ### Connection settings:
    ### Either the go-sql or Oracle AI Database connection pool may be used.
    ### To use the Oracle AI Database connection pool over the go-sql connection pool,
    ### set maxIdleConns to zero and configure the pool* settings.

    ### Connection pooling settings for the go-sql connection pool
    ## Max open connections for this database using go-sql connection pool
    maxOpenConns: 10
    ## Max idle connections for this database using go-sql connection pool
    maxIdleConns: 10

    ### Connection pooling settings for the Oracle AI Database connection pool
    ## Oracle AI Database connection pool increment.
    # poolIncrement: 1
    ## Oracle AI Database Connection pool maximum size
    # poolMaxConnections: 15
    ## Oracle AI Database Connection pool minimum size
    # poolMinConnections: 15

    ### Arbitrary labels to add to each metric scraped from this database
    ## Any labels configured for one database will be added to metrics from
    ## every database, because the same metric names must always have the same
    ## full labelset. If the label isn't set for a particular database, then it
    ## will just be set to an empty string.
    # labels:
    #   label_name1: label_value1
    #   label_name2: label_value2
```

### Scraping specific metrics from specific databases

By default, metrics are scraped from every connected database. To expose only certain metrics on specific databases, configure the `databases` property of a metric. The following metric definition will only be scraped from databases "db2" and "db3":

```toml
[[metric]]
context = "db_platform"
labels = [ "platform_name" ]
metricsdesc = { value = "Database platform" }
request = '''
SELECT platform_name, 1 as value FROM gv$database
'''
databases = [ "db2", "db3" ]
```

If the `databases` array is empty or not provided for a metric, that metric will be scraped from all connected databases.

### Duplicated database configurations

If one or more database configurations are "duplicated", that is, using the same URL and username, a WARN message is logged:

```
msg="duplicated database connections" "database connections"="db1, db2 count=2
```
