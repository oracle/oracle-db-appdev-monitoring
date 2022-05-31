package oracle.observability;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class ObservabilityExporterApplication {

	static {
		System.setProperty("oracle.jdbc.fanEnabled", "false");
	}

	public static void main(String[] args) {
		SpringApplication.run(ObservabilityExporterApplication.class, args);
	}

}
