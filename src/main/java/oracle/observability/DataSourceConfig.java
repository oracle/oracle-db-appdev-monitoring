package oracle.observability;

import lombok.Data;

@Data
public class DataSourceConfig {
    private String dataSourceName;
    private String serviceName;
    private String userName;
    private String password;
    private String TNS_ADMIN;
    private String passwordOCID;
    private String ociConfigFile;
    private String ociRegion;
    private String ociProfile;
}
