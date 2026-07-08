// Copyright (c) 2024, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.package vault

package alertlog

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/collector"
)

const (
	alertLogReadChunkSize = 4096
	maxAlertLogLineBytes  = 1 << 20
	unknownLevel = "UNKNOWN"
	warningLevel = "WARNING"
	traceLevel   = "TRACE"
	infoLevel = "INFO"
	errorLevel = "ERROR"
)

type LogRecord struct {
	Timestamp string `json:"timestamp"`
	Database  string `json:"database"`
	ModuleId  string `json:"moduleId"`
	ECID      string `json:"ecid"`
	Message   string `json:"message"`
	Level     string `json:"level,omitempty"`
}

var defaultLastLogRecord = LogRecord{
	Timestamp: "1900-01-01T01:01:01.001Z",
}



var levelMap = map[int64]string{
	1: unknownLevel,
	2: errorLevel,
	3: errorLevel,
	4: warningLevel,
	5: infoLevel,
	6: traceLevel,
}

const alertLogQuery = `select originating_timestamp, module_id, execution_context_id, message_text, message_type
		from v$diag_alert_ext
		where originating_timestamp > to_utc_timestamp_tz(:1)`


func toLogLevel(messageLevel int64) string {
	if v, ok := levelMap[messageLevel]; ok {
		return v
	}
	return infoLevel
}

// nullStringValue unwraps a nullable database string, returning an empty string for NULL.
func nullStringValue(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func toStringLevel(messageLevel sql.NullInt64, logLevelEnabled bool) string {
	if !logLevelEnabled {
		return ""
	}
	if !messageLevel.Valid {
		return infoLevel
	}
	return levelMap[messageLevel.Int64]
}

// logDestinationForDatabase resolves the output path for a database, either shared or per-database.
func logDestinationForDatabase(logDestination, database string, perDatabaseFiles bool) string {
	if !perDatabaseFiles {
		return logDestination
	}

	dir := filepath.Dir(logDestination)
	ext := filepath.Ext(logDestination)
	base := strings.TrimSuffix(filepath.Base(logDestination), ext)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, database, ext))
}

// readLastMatchingLogRecord scans backward through the log file to find the latest record for a database.
// Shared log files may contain records from multiple databases, so they must be filtered by database name.
func readLastMatchingLogRecord(logDestination, database string, perDatabaseFiles bool) (LogRecord, error) {
	file, err := os.Open(logDestination)
	if err != nil {
		return LogRecord{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return LogRecord{}, err
	}
	filesize := stat.Size()
	if filesize == 0 {
		return defaultLastLogRecord, nil
	}

	currentReversed := make([]byte, 0, alertLogReadChunkSize)

	flushLine := func() (LogRecord, bool, error) {
		if len(currentReversed) == 0 {
			return LogRecord{}, false, nil
		}

		lineBytes := make([]byte, len(currentReversed))
		for i, b := range currentReversed {
			lineBytes[len(currentReversed)-1-i] = b
		}
		currentReversed = currentReversed[:0]

		lineBytes = bytes.TrimSpace(lineBytes)
		if len(lineBytes) == 0 {
			return LogRecord{}, false, nil
		}

		var record LogRecord
		if err := json.Unmarshal(lineBytes, &record); err != nil {
			return LogRecord{}, false, err
		}

		if !perDatabaseFiles && record.Database != database {
			return LogRecord{}, false, nil
		}

		return record, true, nil
	}

	buffer := make([]byte, alertLogReadChunkSize)
	for offset := filesize; offset > 0; {
		start := max(offset-int64(len(buffer)), 0)
		chunkSize := int(offset - start)
		if _, err := file.ReadAt(buffer[:chunkSize], start); err != nil && !errors.Is(err, io.EOF) {
			return LogRecord{}, err
		}

		for i := chunkSize - 1; i >= 0; i-- {
			if buffer[i] == '\n' || buffer[i] == '\r' {
				record, matched, err := flushLine()
				if err != nil {
					return LogRecord{}, err
				}
				if matched {
					return record, nil
				}
				continue
			}

			currentReversed = append(currentReversed, buffer[i])
			if len(currentReversed) > maxAlertLogLineBytes {
				return LogRecord{}, fmt.Errorf("last log line exceeds %d byte limit", maxAlertLogLineBytes)
			}
		}

		offset = start
	}

	record, matched, err := flushLine()
	if err != nil {
		return LogRecord{}, err
	}
	if matched {
		return record, nil
	}

	return defaultLastLogRecord, nil
}

func buildAlertLogQuery(lastTimestamp string) (string, []interface{}) {
	return alertLogQuery, []interface{}{lastTimestamp}
}

// UpdateLog appends newly queried alert log records for a database to the configured log destination.
func UpdateLog(logDestination string, perDatabaseFiles bool, logLevelEnabled bool, logger *slog.Logger, d *collector.Database) {
	if !d.StartupReady() {
		return
	}
	// Do not try to query the alert log if the database configuration is invalid.
	if d.IsValid() != nil {
		return
	}
	now := time.Now()
	if shouldRetry, retryAfter := databaseRetries.shouldRetry(d.Name, now); !shouldRetry {
		logger.Debug("Skipping alert log update while database is in backoff", "database", d.Name, "retry_after", retryAfter)
		return
	}
	logDestination = logDestinationForDatabase(logDestination, d.Name, perDatabaseFiles)

	// check if the log file exists, and if not, create it
	if _, err := os.Stat(logDestination); errors.Is(err, os.ErrNotExist) {
		logger.Info("Log destination file does not exist, will try to create it: "+logDestination, "database", d.Name)
		f, e := os.Create(logDestination)
		if e != nil {
			logger.Error("Failed to create the log file: "+logDestination, "database", d.Name)
			return
		}
		f.Close()
	}

	// read the latest timestamp for this database from the log file
	lastLogRecord, err := readLastMatchingLogRecord(logDestination, d.Name, perDatabaseFiles)
	if err != nil {
		logger.Error("Could not parse last line of log file")
		return
	}

	// query for any new alert log entries
	stmt, args := buildAlertLogQuery(lastLogRecord.Timestamp)
	rows, unlock, err := d.Query(stmt, args...)
	if err != nil {
		retryAfter := databaseRetries.recordFailure(d.Name, now)
		logger.Error("Error querying the alert logs", "error", err, "database", d.Name, "retry_after", retryAfter)
		return
	}
	defer unlock()
	defer rows.Close()

	// write them to the file
	outfile, err := os.OpenFile(logDestination, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		logger.Error("Could not open log file for writing: "+logDestination, "database", d.Name)
		return
	}
	defer outfile.Close()

	for rows.Next() {
		var (
			timestamp string
			moduleID  sql.NullString
			ecid      sql.NullString
			message   sql.NullString
			messageLevel sql.NullInt64
		)
		if err := rows.Scan(&timestamp, &moduleID, &ecid, &message, &messageLevel); err != nil {
			retryAfter := databaseRetries.recordFailure(d.Name, time.Now())
			logger.Error("Error reading a row from the alert logs", "error", err, "database", d.Name, "retry_after", retryAfter)
			return
		}
		newRecord := LogRecord{
			Timestamp: timestamp,
			Database:  d.Name,
			ModuleId:  nullStringValue(moduleID),
			ECID:      nullStringValue(ecid),
			Message:   nullStringValue(message),
			Level:     toStringLevel(messageLevel, logLevelEnabled),
		}

		// strip the newline from end of message
		newRecord.Message = strings.TrimSuffix(newRecord.Message, "\n")

		jsonLogRecord, err := json.Marshal(newRecord)
		if err != nil {
			retryAfter := databaseRetries.recordFailure(d.Name, time.Now())
			logger.Error("Error marshalling alert log record", "error", err, "database", d.Name, "retry_after", retryAfter)
			return
		}

		if _, err = outfile.WriteString(string(jsonLogRecord) + "\n"); err != nil {
			retryAfter := databaseRetries.recordFailure(d.Name, time.Now())
			logger.Error("Could not write to log file: "+logDestination, "error", err, "database", d.Name, "retry_after", retryAfter)
			return
		}
	}

	if err = rows.Err(); err != nil {
		retryAfter := databaseRetries.recordFailure(d.Name, time.Now())
		logger.Error("Error querying the alert logs", "error", err, "database", d.Name, "retry_after", retryAfter)
		return
	}

	databaseRetries.recordSuccess(d.Name)
}
