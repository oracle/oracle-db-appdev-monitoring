package oracle.observability.logs;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.dataformat.toml.TomlMapper;
import oracle.observability.ObservabilityExporter;
import oracle.observability.metrics.MetricEntry;
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
public class DBLogsExporter extends ObservabilityExporter implements Runnable {

	//todo currently logs are read from the beginning during startup, instead add config/functionality similar to promtail positions...
//    private LocalDateTime logQueryLastLocalDateTime;
	private static final org.slf4j.Logger LOG = LoggerFactory.getLogger(DBLogsExporter.class);
	List<String> lastLogged = new ArrayList<>();


	@PostConstruct
	public void init() throws Exception {
		new Thread(this).start();
	}


	@Override
	public void run() {
		while (true) {
			System.out.println("DBLogsExporter.run");
			try {
				LOG.debug("Successfully loaded default metrics from:" + DEFAULT_METRICS);
				LOG.debug("OracleDBLogExporter CUSTOM_METRICS:" + CUSTOM_METRICS); //todo only default metrics are processed currently
				File tomlfile = new File(DEFAULT_METRICS);
				TomlMapper mapper = new TomlMapper();
				JsonNode jsonNode = mapper.readerFor(MetricEntry.class).readTree(new FileInputStream(tomlfile));
				Iterator<JsonNode> logs = jsonNode.get("log").iterator();
				List<String> currentLogged = new ArrayList<>();
				try (Connection connection = getPoolDataSource().getConnection()) {
					while (logs.hasNext()) { //for each "log" entry in toml/config...
						JsonNode next = logs.next();
						String request = next.get("request").asText(); // the sql query
						System.out.println("DBLogsExporter. request:" + request);
						ResultSet resultSet = connection.prepareStatement(request).executeQuery();
						while (resultSet.next()) {
							int columnCount = resultSet.getMetaData().getColumnCount();
							String logString = "";
							for (int i = 0; i < columnCount; i++) { //for each column...
								logString += resultSet.getMetaData().getColumnName(i + 1) + "=" + resultSet.getObject(i + 1) + " ";
							}
							if(!lastLogged.contains(logString)) {
								System.out.println(logString); //avoids dupes, log queries should contain timestamps to avoid dupes that should indeed be logged
								currentLogged.add(logString);
							}
						}
//				int queryRetryInterval = queryRetryIntervalString == null ||
//						queryRetryIntervalString.trim().equals("") ?
//						DEFAULT_RETRY_INTERVAL : Integer.parseInt(queryRetryIntervalString.trim());
//				Thread.sleep(1000 * queryRetryInterval);
					}
					lastLogged = currentLogged;
				}
				Thread.sleep(30 * 1000);
			} catch (Exception e) {
				throw new RuntimeException(e);
			}
		}
	}
}
