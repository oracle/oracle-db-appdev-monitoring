package oracle.observability.logs;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import oracle.observability.ObservabilityExporter;
import org.slf4j.LoggerFactory;
import org.springframework.web.bind.annotation.RestController;

import javax.annotation.PostConstruct;
import java.io.File;
import java.io.FileInputStream;
import java.util.ArrayList;
import java.util.Iterator;

import java.sql.*;
import java.util.List;

@RestController
public class LogsExporter extends ObservabilityExporter implements Runnable {

	private static final org.slf4j.Logger LOGGER = LoggerFactory.getLogger(LogsExporter.class);
	public static final String TIMESTAMPFIELD = "timestampfield";
	public static final String LOG = "log";
	public String LOG_INTERVAL     = System.getenv("LOG_INTERVAL"); // "30s"
	private int logInterval = 30;
	List<String> lastLogged = new ArrayList<>();
	private java.sql.Timestamp alertLogQueryLastLocalDateTime;

	private int consecutiveExceptionCount = 0; //used to backoff todo should be a finer/log entry level rather than global


	@PostConstruct
	public void init() throws Exception {
		new Thread(this).start();
	}

	@Override
	public void run() {
		while (true) {
			try {
				Thread.sleep(consecutiveExceptionCount * 1000);
				Thread.sleep(logInterval * 1000);
				LOGGER.debug("LogsExporter default metrics from:" + DEFAULT_METRICS);
				if(LOG_INTERVAL!=null && !LOG_INTERVAL.trim().equals("")) logInterval = Integer.getInteger(LOG_INTERVAL);
				LOGGER.debug("LogsExporter logInterval:" + logInterval);
				File tomlfile = new File(DEFAULT_METRICS);
				TomlMapper mapper = new TomlMapper();
				JsonNode jsonNode = mapper.readerFor(LogsExporterConfigEntry.class).readTree(new FileInputStream(tomlfile));
				JsonNode log = jsonNode.get(LOG);
				if(log == null || log.isEmpty()) {
					LOGGER.info("No logs records configured");
					return;
				}
				Iterator<JsonNode> logs = log.iterator();
				List<String> currentLogged = new ArrayList<>();
				try (Connection connection = getPoolDataSource().getConnection()) {
					while (logs.hasNext()) { //for each "log" entry in toml/config...
						JsonNode next = logs.next();
						String request = next.get(REQUEST).asText(); // the sql query
						LOGGER.debug("LogsExporter request:" + request);
						JsonNode timestampfieldNode = next.get(TIMESTAMPFIELD);
						if (timestampfieldNode==null) {
							LOGGER.warn("LogsExporter entry does not contain `timestampfield' value request:" + request);
							continue;
						}
						String timestampfield = timestampfieldNode.asText(); // eg ORIGINATING_TIMESTAMP
						LOGGER.debug("LogsExporter timestampfield:" + timestampfield);
						PreparedStatement statement = connection.prepareStatement(
								alertLogQueryLastLocalDateTime == null ? request : request + " WHERE " + timestampfield + " > ?");
						if(alertLogQueryLastLocalDateTime!=null) statement.setTimestamp(1, alertLogQueryLastLocalDateTime);
						ResultSet resultSet = statement.executeQuery();
						while (resultSet.next()) {
							int columnCount = resultSet.getMetaData().getColumnCount();
							String logString = "";
							String columnName;
							Object object;
							for (int i = 0; i < columnCount; i++) { //for each column...
								columnName = resultSet.getMetaData().getColumnName(i + 1);
								object = resultSet.getObject(i + 1);
								if (columnName.equals(timestampfield)) {
									java.sql.Timestamp localDateTime = resultSet.getObject("ORIGINATING_TIMESTAMP", java.sql.Timestamp.class);
									object = localDateTime.getTime();
									if (alertLogQueryLastLocalDateTime == null || localDateTime.after(alertLogQueryLastLocalDateTime)) {
										alertLogQueryLastLocalDateTime = localDateTime;
									}
								}
								logString += columnName + "=" + object + " ";
							}
							if(!lastLogged.contains(logString)) {
								System.out.println(logString); //avoids dupes, log queries should contain timestamps to avoid dupes that should indeed be logged
								currentLogged.add(logString);
							}
						}
					}
					lastLogged = currentLogged;
					consecutiveExceptionCount = 0;
				}
			} catch (Exception e) {
				consecutiveExceptionCount++;
				LOGGER.warn("LogsExporter.processMetric exception:" + e);
			}
		}
	}
}
