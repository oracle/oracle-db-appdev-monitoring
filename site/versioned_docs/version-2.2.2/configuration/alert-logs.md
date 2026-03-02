---
title: Alert Logs
sidebar_position: 5
---

# Alert logs

Collect export alert logs with a log ingestion tool.

The exporter exports alert log records as a JSON file suitable for collection by a log ingestion tool like Promtail or FluentBit.

Alert logging is configured with the following parameters in the exporter config file:

| Parameter       | Description                   | Default          |
|-----------------|-------------------------------|------------------|
| log.destination | Log file path                 | `/log/alert.log` |
| log.interval    | Interval to log records       | `15s`            |
| log.disable     | Disable logging if set to `1` | `0`              |

Example alert log YAML configuration:

```yaml
log:
  # Path of log file
  destination: /opt/exporter/alert.log
  # Interval of log updates
  interval: 15s
  ## Set disable to 1 to disable logging
  # disable: 0
```
