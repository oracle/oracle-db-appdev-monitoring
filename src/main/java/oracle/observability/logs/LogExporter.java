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
public class LogExporter extends ObservabilityExporter implements Runnable {

	private static final org.slf4j.Logger LOG = LoggerFactory.getLogger(LogExporter.class);
	public String LOG_INTERVAL     = System.getenv("LOG_INTERVAL"); // "30s"
	private int logInterval = 30;
	List<String> lastLogged = new ArrayList<>();
	private java.sql.Timestamp alertLogQueryLastLocalDateTime;


	@PostConstruct
	public void init() throws Exception {
		new Thread(this).start();
	}

	@Override
	public void run() {
		while (true) {
			try {
				LOG.debug("LogExporter default metrics from:" + DEFAULT_METRICS);
				if(LOG_INTERVAL!=null && !LOG_INTERVAL.trim().equals("")) logInterval = Integer.getInteger(LOG_INTERVAL);
				LOG.debug("LogExporter logInterval:" + logInterval);
				File tomlfile = new File(DEFAULT_METRICS);
				TomlMapper mapper = new TomlMapper();
				JsonNode jsonNode = mapper.readerFor(LogExporterConfigEntry.class).readTree(new FileInputStream(tomlfile));
				Iterator<JsonNode> logs = jsonNode.get("log").iterator();
				List<String> currentLogged = new ArrayList<>();
				try (Connection connection = getPoolDataSource().getConnection()) {
					while (logs.hasNext()) { //for each "log" entry in toml/config...
						JsonNode next = logs.next();
						String request = next.get("request").asText(); // the sql query
						LOG.debug("DBLogsExporter. request:" + request);
						String timestampfield = next.get("timestampfield").asText(); // eg ORIGINATING_TIMESTAMP
						LOG.debug("DBLogsExporter. timestampfield:" + timestampfield);
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
				}
				Thread.sleep(logInterval * 1000);
			} catch (Exception e) {
				throw new RuntimeException(e);
			}
		}
	}
}
