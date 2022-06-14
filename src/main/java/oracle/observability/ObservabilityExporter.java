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

import java.io.IOException;
import java.sql.SQLException;

public class ObservabilityExporter {

    public String DEFAULT_METRICS = System.getenv("DEFAULT_METRICS"); // "default-metrics.toml"
    public String CUSTOM_METRICS      = System.getenv("CUSTOM_METRICS"); //
    public String QUERY_TIMEOUT       = System.getenv("QUERY_TIMEOUT"); // "5"
    public String DATABASE_MAXIDLECONNS       = System.getenv("DATABASE_MAXIDLECONNS"); // "0"
    public String DATABASE_MAXOPENCONNS       = System.getenv("DATABASE_MAXOPENCONNS"); // "10"
    public String DATA_SOURCE_NAME = System.getenv("DATA_SOURCE_NAME"); //eg %USER%/$(dbpassword)@%PDB_NAME%_tp
    public String TNS_ADMIN = System.getenv("TNS_ADMIN");  //eg /msdataworkshop/creds
    public String OCI_REGION = System.getenv("OCI_REGION");  //eg us-ashburn-1
    public String VAULT_SECRET_OCID = System.getenv("VAULT_SECRET_OCID");  //eg ocid....
    public String OCI_CONFIG_FILE = System.getenv("OCI_CONFIG_FILE");  //eg "~/.oci/config"

    PoolDataSource observabilityDB;
    public PoolDataSource getPoolDataSource() throws SQLException {
        if (observabilityDB != null) return observabilityDB;
        observabilityDB = PoolDataSourceFactory.getPoolDataSource();
        observabilityDB.setConnectionFactoryClassName("oracle.jdbc.pool.OracleDataSource");
        String user = DATA_SOURCE_NAME.substring(0, DATA_SOURCE_NAME.indexOf("/"));
        String pw = DATA_SOURCE_NAME.substring(DATA_SOURCE_NAME.indexOf("/") + 1, DATA_SOURCE_NAME.indexOf("@"));
        String serviceName = DATA_SOURCE_NAME.substring(DATA_SOURCE_NAME.indexOf("@") + 1);
        String url = "jdbc:oracle:thin:@" + serviceName + "?TNS_ADMIN=" + TNS_ADMIN;
        observabilityDB.setURL(url);
        observabilityDB.setUser(user);
        if (VAULT_SECRET_OCID == null || VAULT_SECRET_OCID.trim().equals("")) {
            observabilityDB.setPassword(pw);
        } else {
            try {
                observabilityDB.setPassword(getPasswordFromVault());
            } catch (IOException e) {
                throw new SQLException(e);
            }
        }
        return observabilityDB;
    }


    public String getPasswordFromVault() throws IOException {
        SecretsClient secretsClient;
        if (OCI_CONFIG_FILE == null || OCI_CONFIG_FILE.trim().equals("")) {
            secretsClient = new SecretsClient(InstancePrincipalsAuthenticationDetailsProvider.builder().build());
        } else {
            secretsClient = new SecretsClient(new ConfigFileAuthenticationDetailsProvider(OCI_CONFIG_FILE, "DEFAULT")); //todo allow profile override as well
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
