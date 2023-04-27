package oracle.observability;

import com.oracle.bmc.auth.ConfigFileAuthenticationDetailsProvider;
import com.oracle.bmc.auth.InstancePrincipalsAuthenticationDetailsProvider;
import com.oracle.bmc.secrets.SecretsClient;
import com.oracle.bmc.secrets.model.Base64SecretBundleContentDetails;
import com.oracle.bmc.secrets.requests.GetSecretBundleRequest;
import com.oracle.bmc.secrets.responses.GetSecretBundleResponse;
import oracle.observability.metrics.MetricsExporter;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;
import org.apache.commons.codec.binary.Base64;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.File;
import java.io.IOException;
import java.sql.SQLException;
import java.util.HashMap;
import java.util.Map;

public class ObservabilityExporter {

    private static final Logger LOGGER = LoggerFactory.getLogger(ObservabilityExporter.class);
    public String DEFAULT_METRICS = System.getenv("DEFAULT_METRICS"); // "default-metrics.toml"
    public File DEFAULT_METRICS_FILE;
    public String CUSTOM_METRICS      = System.getenv("CUSTOM_METRICS"); //
    public String QUERY_TIMEOUT       = System.getenv("QUERY_TIMEOUT"); // "5"
    public String DATABASE_MAXIDLECONNS       = System.getenv("DATABASE_MAXIDLECONNS"); // "0"
    public String DATABASE_MAXOPENCONNS       = System.getenv("DATABASE_MAXOPENCONNS"); // "10"
    public static String DATA_SOURCE_NAME = System.getenv("DATA_SOURCE_NAME"); //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
    public static String DATA_SOURCE_USER = System.getenv("DATA_SOURCE_USER"); //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
    public static String DATA_SOURCE_PASSWORD = System.getenv("DATA_SOURCE_PASSWORD"); //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
    public static String DATA_SOURCE_SERVICENAME = System.getenv("DATA_SOURCE_SERVICENAME"); //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
    public String TNS_ADMIN = System.getenv("TNS_ADMIN");  //eg /msdataworkshop/creds
    public String OCI_REGION = System.getenv("OCI_REGION");  //eg us-ashburn-1
    public String VAULT_SECRET_OCID = System.getenv("VAULT_SECRET_OCID");  //eg ocid....
    public String OCI_CONFIG_FILE = System.getenv("OCI_CONFIG_FILE");  //eg "~/.oci/config"
    public String OCI_PROFILE = System.getenv("OCI_PROFILE");  //eg "DEFAULT"
    public static final String CONTEXT = "context";
    public static final String REQUEST = "request";

    static {

        if (DATA_SOURCE_USER != null && DATA_SOURCE_PASSWORD != null && DATA_SOURCE_SERVICENAME != null) {
            DATA_SOURCE_NAME = DATA_SOURCE_USER + "/" + DATA_SOURCE_PASSWORD + "@" + DATA_SOURCE_SERVICENAME;
            LOGGER.info("DATA_SOURCE_NAME = DATA_SOURCE_USER + \"/\" + DATA_SOURCE_PASSWORD + \"@\" + DATA_SOURCE_SERVICENAME");
            //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
        }
    }
    PoolDataSource observabilityDB;
    Map<String, PoolDataSource> dataSourceNameToDataSourceMap = new HashMap<>();

    public PoolDataSource getPoolDataSource() throws SQLException {
        return getPoolDataSource(DATA_SOURCE_NAME);
    }
    public PoolDataSource getPoolDataSource(String dataSourceName) throws SQLException {
        if (dataSourceName.equals(DATA_SOURCE_NAME)) {
            if (observabilityDB != null) return observabilityDB;
            return observabilityDB = getDataSource(DATA_SOURCE_NAME);
        } else {
            if(dataSourceNameToDataSourceMap.containsKey(dataSourceName) && dataSourceNameToDataSourceMap.get(dataSourceName) != null)
                return dataSourceNameToDataSourceMap.get(dataSourceName);
            PoolDataSource poolDataSource = getDataSource(dataSourceName);
            dataSourceNameToDataSourceMap.put(dataSourceName, poolDataSource);
            return poolDataSource;
        }
    }

    private PoolDataSource getDataSource(String dataSourceName) throws SQLException {
        PoolDataSource poolDataSource = PoolDataSourceFactory.getPoolDataSource();
        poolDataSource.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
        String user = dataSourceName.substring(0, dataSourceName.indexOf("/"));
        String pw = dataSourceName.substring(dataSourceName.indexOf("/") + 1, dataSourceName.indexOf("@"));
        String serviceName = dataSourceName.substring(dataSourceName.indexOf("@") + 1);
        String url = "jdbc:oracle:thin:@" + serviceName + "?TNS_ADMIN=" + TNS_ADMIN;
        poolDataSource.setURL(url);
        poolDataSource.setUser(user);
        if (VAULT_SECRET_OCID == null || VAULT_SECRET_OCID.trim().equals("") || !dataSourceName.equals(DATA_SOURCE_NAME)) {
            poolDataSource.setPassword(pw);
        } else {
            try {
                poolDataSource.setPassword(getPasswordFromVault());
            } catch (IOException e) {
                throw new SQLException(e);
            }
        }
        return poolDataSource;
    }


    public String getPasswordFromVault() throws IOException {
        SecretsClient secretsClient;
        if (OCI_CONFIG_FILE == null || OCI_CONFIG_FILE.trim().equals("")) {
            secretsClient = new SecretsClient(InstancePrincipalsAuthenticationDetailsProvider.builder().build());
        } else {
            String profile = OCI_PROFILE==null || OCI_PROFILE.trim().equals("") ? "DEFAULT": OCI_PROFILE;
            secretsClient = new SecretsClient(new ConfigFileAuthenticationDetailsProvider(OCI_CONFIG_FILE, profile));
        }
        secretsClient.setRegion(OCI_REGION);
        GetSecretBundleRequest getSecretBundleRequest = GetSecretBundleRequest
                .builder()
                .secretId(VAULT_SECRET_OCID)
                .stage(GetSecretBundleRequest.Stage.Current)
                .build();
        GetSecretBundleResponse getSecretBundleResponse = secretsClient.getSecretBundle(getSecretBundleRequest);
        Base64SecretBundleContentDetails base64SecretBundleContentDetails =
                (Base64SecretBundleContentDetails) getSecretBundleResponse.getSecretBundle().getSecretBundleContent();
        byte[] secretValueDecoded = Base64.decodeBase64(base64SecretBundleContentDetails.getContent());
        return new String(secretValueDecoded);
    }
}
