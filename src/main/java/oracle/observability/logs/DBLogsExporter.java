package oracle.observability.logs;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import oracle.observability.ObservabilityExporter;
import oracle.observability.metrics.MetricEntry;
import oracle.observability.metrics.DBMetricsExporter;
import oracle.ucp.jdbc.PoolDataSourceFactory;
import org.slf4j.LoggerFactory;
import org.springframework.web.bind.annotation.RestController;

import javax.annotation.PostConstruct;
import java.io.File;
import java.io.FileInputStream;
import java.util.Iterator;
import java.util.logging.Logger;

import java.sql.*;
import java.util.logging.FileHandler;

//@Configuration
//@EnableAutoConfiguration
//@ComponentScan

//@RestController
public class DBLogsExporter extends ObservabilityExporter {

	static boolean isFirstCall = true;
	static String querySQL = "select ORIGINATING_TIMESTAMP, MODULE_ID, EXECUTION_CONTEXT_ID, MESSAGE_TEXT from V$diag_alert_ext"; // System.getenv("QUERY_SQL");
	static String queryRetryIntervalString = System.getenv("QUERY_INTERVAL");
	static int DEFAULT_RETRY_INTERVAL = 30; // in seconds
	private boolean enabled = true;
	//todo currently logs are read from the beginning during startup, instead add config/functionality similar to promtail positions...
//    private LocalDateTime logQueryLastLocalDateTime;
	private java.sql.Timestamp alertLogQueryLastLocalDateTime;
	private int vashQueryLastSampleId = -1;
	private String alertLogDefaultQuery = "select ORIGINATING_TIMESTAMP, MODULE_ID, EXECUTION_CONTEXT_ID, MESSAGE_TEXT from TABLE(GV$(CURSOR(select * from v$diag_alert_ext)))";
	private String vashDefaultQuery = "select SAMPLE_ID, SAMPLE_TIME, SQL_ID, SQL_OPNAME, PROGRAM, MODULE, ACTION, CLIENT_ID, MACHINE, ECID " +
			"from TABLE(GV$(CURSOR(select * from v$active_session_history))) where ECID is not null and SAMPLE_ID > ";
	private static final org.slf4j.Logger LOG = LoggerFactory.getLogger(DBLogsExporter.class);


	@PostConstruct
	public void init() throws Exception {
		LOG.debug("Successfully loaded default metrics from:" + DEFAULT_METRICS);
		LOG.debug("OracleDBMetricsExporter CUSTOM_METRICS:" + CUSTOM_METRICS); //todo only default metrics are processed currently
		File tomlfile = new File(DEFAULT_METRICS);
		TomlMapper mapper = new TomlMapper();
		JsonNode jsonNode = mapper.readerFor(MetricEntry.class).readTree(new FileInputStream(tomlfile));
		Iterator<JsonNode> logs = jsonNode.get("log").iterator();
		try (Connection conn = getPoolDataSource().getConnection()) {
			while (logs.hasNext()) {
				executeAlertLogQuery(conn);
				//         executeVASHQuery(conn);
//				int queryRetryInterval = queryRetryIntervalString == null ||
//						queryRetryIntervalString.trim().equals("") ?
//						DEFAULT_RETRY_INTERVAL : Integer.parseInt(queryRetryIntervalString.trim());
//				Thread.sleep(1000 * queryRetryInterval);
			}
		}
	}

	private void executeAlertLogQuery(Connection conn) throws SQLException {
		//todo  get from last NORMALIZED_TIMESTAMP inclusive
		/**
		 * ORIGINATING_TIMESTAMP            TIMESTAMP(9) WITH TIME ZONE
		 * NORMALIZED_TIMESTAMP             TIMESTAMP(9) WITH TIME ZONE
		 */
		if(querySQL == null || querySQL.trim().equals("")) {
			querySQL = alertLogDefaultQuery;
		}
//        System.out.println("AlertLogExporterResource querySQL:" + querySQL + " alertLogQueryLastLocalDateTime:" + alertLogQueryLastLocalDateTime);
		PreparedStatement statement = conn.prepareStatement(isFirstCall ? querySQL : querySQL + " WHERE ORIGINATING_TIMESTAMP > ?");
		if (!isFirstCall) statement.setTimestamp(1, alertLogQueryLastLocalDateTime);
		ResultSet rs = statement.executeQuery(); //do not fail for ORA-00942: table or view does not exist etc.
		while (rs.next()) { //todo make dynamic for other SQL queries...
			java.sql.Timestamp localDateTime = rs.getObject("ORIGINATING_TIMESTAMP", java.sql.Timestamp.class);
			if (alertLogQueryLastLocalDateTime == null || localDateTime.after(alertLogQueryLastLocalDateTime)) {
				alertLogQueryLastLocalDateTime = localDateTime;
			}
			String keys[] = {"MODULE_ID", "EXECUTION_CONTEXT_ID", "MESSAGE_TEXT"};
			logKeyValue(rs, keys, localDateTime);
		}
		isFirstCall = false;
	}

	private void executeVASHQuery(Connection conn) throws SQLException {
		/**
		 *      SAMPLE_ID                         NUMBER
		 *      SAMPLE_TIME                       TIMESTAMP(3)
		 *      SAMPLE_TIME_UTC                   TIMESTAMP(3)
		 */
		// ECID will likely be null unless the scaling/stress lab (lab 4) has been run in order to generate enough load for a sample
		//   (or of course if any other activity that logs an ECID has been conducted on this pdb).
		//   todo this being the case this will not produce any logs unless the scaling lab is run and so
		//    we might want a where SQL_OPNAME=INSERT or  PROGRAM/MODULE like order-helidon as a default instead
		// todo use prepared statement and SAMPLE_TIME TIMESTAMP(3) instead of SAMPLE_ID NUMBER ...
		String vashQuery = vashDefaultQuery + vashQueryLastSampleId;
//        System.out.println("AlertLogExporterResource querySQL:" + vashQuery);
		PreparedStatement statement = conn.prepareStatement(vashQuery);
		ResultSet rs = statement.executeQuery();
		while (rs.next()) {
			int sampleId =  rs.getInt("SAMPLE_ID");
			if (sampleId > vashQueryLastSampleId) vashQueryLastSampleId = sampleId;
//            System.out.println("AlertLogExporterResource vashQueryLastSampleId:" + vashQueryLastSampleId);
			String keys[] = {"SAMPLE_ID", "SAMPLE_TIME", "SQL_ID", "SQL_OPNAME", "PROGRAM", "MODULE", "ACTION", "CLIENT_ID", "MACHINE", "ECID"};
			logKeyValue(rs, keys, null); //todo should be sample_time
		}
	}

	private void logKeyValue(ResultSet rs, String[] keys, Timestamp localDateTime) throws SQLException {
		String logString = "";
		for (int i=0; i<keys.length; i++)
			logString+= keys[i] + "=" + rs.getString(keys[i]) + " ";
		if (localDateTime != null) logString = "ORIGINATING_TIMESTAMP=" + localDateTime + " " + logString;
		System.out.println(logString);
	}


}
