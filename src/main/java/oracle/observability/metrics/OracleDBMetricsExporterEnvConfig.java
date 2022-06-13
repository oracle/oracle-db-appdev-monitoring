package oracle.observability.metrics;

public class OracleDBMetricsExporterEnvConfig {
    /**

     - name: DEFAULT_METRICS
    value: /msdataworkshop/observability/db-metrics-%EXPORTER_NAME%-exporter-metrics.toml
     #            - name: CUSTOM_METRICS
     #              value: /msdataworkshop/observability/db-metrics-%EXPORTER_NAME%-exporter-metrics.toml
     - name: TNS_ADMIN
    value: "/msdataworkshop/creds"
            #          value: "/lib/oracle/instantclient_19_3/client64/lib/network/admin"
            - name: dbpassword
    valueFrom:
    secretKeyRef:
    name: dbuser
    key: dbpassword
    optional: true
            - name: DATA_SOURCE_NAME
     #              value: "admin/$(dbpassword)@%PDB_NAME%_tp"
    value: "%USER%/$(dbpassword)@%PDB_NAME%_tp"
    volumeMounts:
            - name: creds
    mountPath: /msdataworkshop/creds
     #          mountPath: /lib/oracle/instantclient_19_3/client64/lib/network/admin # 19.10

            *
            *
            *
            *
            * need to support all of these combos as doced for backward compat...
            * # export Oracle location:
            * export DATA_SOURCE_NAME=system/password@oracle-sid
     * # or using a complete url:
            * export DATA_SOURCE_NAME=user/password@//myhost:1521/service
     * # 19c client for primary/standby configuration
     * export DATA_SOURCE_NAME=user/password@//primaryhost:1521,standbyhost:1521/service
     * # 19c client for primary/standby configuration with options
     * export DATA_SOURCE_NAME=user/password@//primaryhost:1521,standbyhost:1521/service?connect_timeout=5&transport_connect_timeout=3&retry_count=3
     * # 19c client for ASM instance connection (requires SYSDBA)
     * export DATA_SOURCE_NAME=user/password@//primaryhost:1521,standbyhost:1521/+ASM?as=sysdba
     *
             * Usage of oracledb_exporter:
            *   --log.format value
     *        	If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.
     *   --log.level value
     *        	Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal].
            *   --custom.metrics string
     *         File that may contain various custom metrics in a TOML file.
     *   --default.metrics string
     *         Default TOML file metrics.
            *   --web.listen-address string
     *        	Address to listen on for web interface and telemetry. (default ":9161")
            *   --web.telemetry-path string
     *        	Path under which to expose metrics. (default "/metrics")
            *   --database.maxIdleConns string
     *         Number of maximum idle connections in the connection pool. (default "0")
            *   --database.maxOpenConns string
     *         Number of maximum open connections in the connection pool. (default "10")
            *   --web.secured-metrics  boolean
     *         Expose metrics using https server. (default "false")
            *   --web.ssl-server-cert string
     *         Path to the PEM encoded certificate file.
     *   --web.ssl-server-key string
     *         Path to the PEM encoded key file.
     *
     *
     *
     listenAddress      = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry. (env: LISTEN_ADDRESS)").Default(getEnv("LISTEN_ADDRESS", ":9161")).String()
     metricPath         = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics. (env: TELEMETRY_PATH)").Default(getEnv("TELEMETRY_PATH", "/metrics")).String()
     defaultFileMetrics = kingpin.Flag("default.metrics", "File with default metrics in a TOML file. (env: DEFAULT_METRICS)").Default(getEnv("DEFAULT_METRICS", "default-metrics.toml")).String()
     customMetrics      = kingpin.Flag("custom.metrics", "File that may contain various custom metrics in a TOML file. (env: CUSTOM_METRICS)").Default(getEnv("CUSTOM_METRICS", "")).String()
     queryTimeout       = kingpin.Flag("query.timeout", "Query timeout (in seconds). (env: QUERY_TIMEOUT)").Default(getEnv("QUERY_TIMEOUT", "5")).String()
     maxIdleConns       = kingpin.Flag("database.maxIdleConns", "Number of maximum idle connections in the connection pool. (env: DATABASE_MAXIDLECONNS)").Default(getEnv("DATABASE_MAXIDLECONNS", "0")).Int()
     maxOpenConns       = kingpin.Flag("database.maxOpenConns", "Number of maximum open connections in the connection pool. (env: DATABASE_MAXOPENCONNS)").Default(getEnv("DATABASE_MAXOPENCONNS", "10")).Int()
     securedMetrics     = kingpin.Flag("web.secured-metrics", "Expose metrics using https.").Default("false").Bool()
     serverCert         = kingpin.Flag("web.ssl-server-cert", "Path to the PEM encoded certificate").ExistingFile()
     serverKey          = kingpin.Flag("web.ssl-server-key", "Path to the PEM encoded key").ExistingFile()
     scrapeInterval     = kingpin.Flag("scrape.interval", "Interval between each scrape. Default is to scrape on collect requests").Default("0s").Duration()

     *
             */

    //    String dbuser = System.getenv("dbuser");
//    String dbpassword = System.getenv("dbpassword");
    String DATA_SOURCE_NAME = System.getenv("DATA_SOURCE_NAME"); //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
    String TNS_ADMIN = System.getenv("TNS_ADMIN");  //eg /msdataworkshop/creds
    String DEFAULT_METRICS = System.getenv("DEFAULT_METRICS");  //eg /msdataworkshop/observability/default-metrics.toml
    String CUSTOM_METRICS = System.getenv("CUSTOM_METRICS");  //eg /msdataworkshop/observability/custom-metrics.toml
    String OCI_REGION = System.getenv("OCI_REGION");  //eg us-ashburn-1
    String VAULT_SECRET_OCID = System.getenv("VAULT_SECRET_OCID");  //eg ocid....
    String OCI_CONFIG_FILE = System.getenv("OCI_CONFIG_FILE");  //eg "~/.oci/config"

}
