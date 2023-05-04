package oracle.observability;

import com.oracle.bmc.auth.ConfigFileAuthenticationDetailsProvider;
import com.oracle.bmc.auth.InstancePrincipalsAuthenticationDetailsProvider;
import com.oracle.bmc.secrets.SecretsClient;
import com.oracle.bmc.secrets.model.Base64SecretBundleContentDetails;
import com.oracle.bmc.secrets.requests.GetSecretBundleRequest;
import com.oracle.bmc.secrets.responses.GetSecretBundleResponse;
import oracle.ucp.jdbc.PoolDataSource;
import oracle.ucp.jdbc.PoolDataSourceFactory;
import org.apache.commons.codec.binary.Base64;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.*;
import java.sql.SQLException;
import java.util.HashMap;
import java.util.Map;

public class ObservabilityExporter {

    private static final Logger LOGGER = LoggerFactory.getLogger(ObservabilityExporter.class);
    public String DEFAULT_METRICS = System.getenv("DEFAULT_METRICS"); // "default-metrics.toml"
    public File DEFAULT_METRICS_FILE;
    public String CUSTOM_METRICS      = System.getenv("CUSTOM_METRICS"); //
    public String QUERY_TIMEOUT       = System.getenv("QUERY_TIMEOUT"); // "5"
    public static final String CONTEXT = "context";
    public static final String REQUEST = "request";

    //Single/global datasource config related....
    public String DATABASE_MAXIDLECONNS       = System.getenv("DATABASE_MAXIDLECONNS"); // "0"
    public String DATABASE_MAXOPENCONNS       = System.getenv("DATABASE_MAXOPENCONNS"); // "10"
    public static String DATA_SOURCE_NAME = System.getenv("DATA_SOURCE_NAME"); //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
    //if all three of the following exist, they are internally concatenated and override/used as DATA_SOURCE_NAME
    public static String DATA_SOURCE_USER = System.getenv("DATA_SOURCE_USER"); //eg %USER%
    public static String DATA_SOURCE_PASSWORD = System.getenv("DATA_SOURCE_PASSWORD"); //eg $(dbpassword)
    public static String DATA_SOURCE_SERVICENAME = System.getenv("DATA_SOURCE_SERVICENAME"); //eg %PDB_NAME%_tp
    public static String TNS_ADMIN = System.getenv("TNS_ADMIN");  //eg /msdataworkshop/creds

    public static String OCI_REGION = System.getenv("OCI_REGION");  //eg us-ashburn-1
    public static String VAULT_SECRET_OCID = System.getenv("VAULT_SECRET_OCID");  //eg ocid....
    public static String OCI_CONFIG_FILE = System.getenv("OCI_CONFIG_FILE");  //eg "~/.oci/config"
    public static String OCI_PROFILE = System.getenv("OCI_PROFILE");  //eg "DEFAULT"

    //MULTI_DATASOURCE_CONFIG related....
    public static String MULTI_DATASOURCE_CONFIG = System.getenv("MULTI_DATASOURCE_CONFIG");
    public static final String SERVICE_NAME_STRING = "serviceName";
    public static final String USER_NAME_STRING = "userName";
    public static final String PASSWORD_STRING = "password";
    public static final String TNS_ADMIN_STRING = "TNS_ADMIN";
    public static final String PASSWORD_OCID_STRING = "passwordOCID";
    public static final String OCI_CONFIG_FILE_STRING = "ociConfigFile";
    public static final String OCI_REGION_STRING = "ociRegion";
    public static final String OCI_PROFILE_STRING = "ociProfile";

    static { // not really necessary but gives information that a global datasource is in use
        if (DATA_SOURCE_USER != null && DATA_SOURCE_PASSWORD != null && DATA_SOURCE_SERVICENAME != null) {
            DATA_SOURCE_NAME = DATA_SOURCE_USER + "/" + DATA_SOURCE_PASSWORD + "@" + DATA_SOURCE_SERVICENAME;
            LOGGER.info("DATA_SOURCE_NAME = DATA_SOURCE_USER + \"/\" + DATA_SOURCE_PASSWORD + \"@\" + DATA_SOURCE_SERVICENAME");
            //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
        }
    }
    PoolDataSource globalObservabilityDB;

    //This map is used for multi-datasource scraping, both when using dns target string and config
    Map<String, PoolDataSource> dataSourceNameToDataSourceMap = new HashMap<>();

    //This map is used for multi-datasource scraping when using config only
    public static Map<String, DataSourceConfig> dataSourceNameToDataSourceConfigMap = new HashMap<>();

    //used by logs and tracing exporters as they do not currently support multi-datasource config
    public PoolDataSource getPoolDataSource() throws SQLException {
        return getPoolDataSource(DATA_SOURCE_NAME, false);
    }

    public PoolDataSource getPoolDataSource(String dataSourceName, boolean isScrapeByName) throws SQLException {
        if (DATA_SOURCE_NAME != null && dataSourceName.equals(DATA_SOURCE_NAME)) {
            if (globalObservabilityDB != null) return globalObservabilityDB;
            return globalObservabilityDB = getDataSource(DATA_SOURCE_NAME);
        } else {
            if(dataSourceNameToDataSourceMap.containsKey(dataSourceName) && dataSourceNameToDataSourceMap.get(dataSourceName) != null)
                return dataSourceNameToDataSourceMap.get(dataSourceName);

            System.out.println("putting dataSourceName:" + dataSourceName + " isScrapeByName:" + isScrapeByName +
                    " ObservabilityExporter.dataSourceNameToDataSourceConfigMap.get(dataSourceName):"+
                    ObservabilityExporter.dataSourceNameToDataSourceConfigMap.get(dataSourceName));
            PoolDataSource poolDataSource = isScrapeByName?
                getDataSource(ObservabilityExporter.dataSourceNameToDataSourceConfigMap.get(dataSourceName))
                :getDataSource(dataSourceName);
            dataSourceNameToDataSourceMap.put(dataSourceName, poolDataSource);
            return poolDataSource;
        }
    }

    private PoolDataSource getDataSource(String dataSourceName) throws SQLException {
        String user = dataSourceName.substring(0, dataSourceName.indexOf("/"));
        String pw = dataSourceName.substring(dataSourceName.indexOf("/") + 1, dataSourceName.indexOf("@"));
        String serviceName = dataSourceName.substring(dataSourceName.indexOf("@") + 1);
        return getPoolDataSource(dataSourceName, user, pw, serviceName, TNS_ADMIN,
                VAULT_SECRET_OCID, OCI_CONFIG_FILE, OCI_PROFILE, OCI_REGION, false);
    }
    private PoolDataSource getDataSource(DataSourceConfig dataSourceConfig) throws SQLException {
        return getPoolDataSource(dataSourceConfig.getDataSourceName(),
                dataSourceConfig.getUserName(),
                dataSourceConfig.getPassword(),
                dataSourceConfig.getServiceName(),
                dataSourceConfig.getTNS_ADMIN(),
                dataSourceConfig.getPasswordOCID(),
                dataSourceConfig.getOciConfigFile(),
                dataSourceConfig.getOciProfile(),
                dataSourceConfig.getOciRegion(),
                true);
    }

    private PoolDataSource getPoolDataSource(
            String dataSourceName, String user, String pw, String serviceName, String tnsAdmin,
            String vaultSecretOcid, String ociConfigFile, String ociProfile, String ociRegion, boolean isScrapeByName) throws SQLException {
        System.out.println("getPoolDataSource dataSourceName = " + dataSourceName + ", user = " + user + ", pw = " + pw + ", serviceName = " + serviceName + ", vaultSecretOcid = " + vaultSecretOcid + ", ociConfigFile = " + ociConfigFile + ", ociProfile = " + ociProfile + ", ociRegion = " + ociRegion + ", isScrapeByName = " + isScrapeByName);
        PoolDataSource poolDataSource = PoolDataSourceFactory.getPoolDataSource();
        poolDataSource.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
        String url = "jdbc:oracle:thin:@" + serviceName + "?TNS_ADMIN=" + tnsAdmin;
        poolDataSource.setURL(url);
        poolDataSource.setUser(user);
        if (VAULT_SECRET_OCID == null || VAULT_SECRET_OCID.trim().equals("") ||
                //vault is not supported with scrape by dns currently, only with scrape by datasource name and global datasource
                (!isScrapeByName && !dataSourceName.equals(DATA_SOURCE_NAME)) ) {
            poolDataSource.setPassword(pw);
        } else {
            try {
                poolDataSource.setPassword(getPasswordFromVault(vaultSecretOcid, ociConfigFile, ociProfile, ociRegion));
            } catch (IOException e) {
                throw new SQLException(e);
            }
        }
        return poolDataSource;
    }


    public String getPasswordFromVault(String vaultSecretOcid, String ociConfigFile, String ociProfile, String ociRegion) throws IOException {
        SecretsClient secretsClient;
        if (ociConfigFile == null || ociConfigFile.trim().equals("")) {
            secretsClient = new SecretsClient(InstancePrincipalsAuthenticationDetailsProvider.builder().build());
        } else {
            String profile = ociProfile ==null || ociProfile.trim().equals("") ? "DEFAULT": ociProfile;
            secretsClient = new SecretsClient(new ConfigFileAuthenticationDetailsProvider(ociConfigFile, profile));
        }
        secretsClient.setRegion(ociRegion);
        GetSecretBundleRequest getSecretBundleRequest = GetSecretBundleRequest
                .builder()
                .secretId(vaultSecretOcid )
                .stage(GetSecretBundleRequest.Stage.Current)
                .build();
        GetSecretBundleResponse getSecretBundleResponse = secretsClient.getSecretBundle(getSecretBundleRequest);
        Base64SecretBundleContentDetails base64SecretBundleContentDetails =
                (Base64SecretBundleContentDetails) getSecretBundleResponse.getSecretBundle().getSecretBundleContent();
        byte[] secretValueDecoded = Base64.decodeBase64(base64SecretBundleContentDetails.getContent());
        return new String(secretValueDecoded);
    }
}
