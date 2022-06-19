FROM openjdk:11-jre-slim

ENTRYPOINT ["java", "-jar", "/usr/share/observability-exporter.jar"]

ADD target/observability-exporter-0.1.0.jar /usr/share/observability-exporter.jar