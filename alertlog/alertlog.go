// Copyright (c) 2024, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.package vault

package alertlog

import (
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

type LogRecord struct {
	Timestamp string `json:"timestamp"`
	Database  string `json:"database"`
	ModuleId  string `json:"moduleId"`
	ECID      string `json:"ecid"`
	Message   string `json:"message"`
}

var defaultLastLogRecord = LogRecord{
	Timestamp: "1900-01-01T01:01:01.001Z",
}

// nullStringValue unwraps a nullable database string, returning an empty string for NULL.
func nullStringValue(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
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

	var (
		pointer  int64
		current  string
		stat, _  = file.Stat()
		filesize = stat.Size()
	)

	flushLine := func() (LogRecord, bool, error) {
		line := strings.TrimSpace(current)
		current = ""
		if line == "" {
			return LogRecord{}, false, nil
		}

		var record LogRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return LogRecord{}, false, err
		}

		if !perDatabaseFiles && record.Database != database {
			return LogRecord{}, false, nil
		}

		return record, true, nil
	}

	for {
		if filesize == 0 {
			break
		}

		pointer--
		if _, err := file.Seek(pointer, io.SeekEnd); err != nil {
			return LogRecord{}, err
		}

		char := make([]byte, 1)
		if _, err := file.Read(char); err != nil {
			return LogRecord{}, err
		}

		if char[0] == '\n' || char[0] == '\r' {
			record, matched, err := flushLine()
			if err != nil {
				return LogRecord{}, err
			}
			if matched {
				return record, nil
			}
		} else {
			current = string(char) + current
		}

		if pointer == -filesize {
			record, matched, err := flushLine()
			if err != nil {
				return LogRecord{}, err
			}
			if matched {
				return record, nil
			}
			break
		}
	}

	return defaultLastLogRecord, nil
}

// UpdateLog appends newly queried alert log records for a database to the configured log destination.
func UpdateLog(logDestination string, perDatabaseFiles bool, logger *slog.Logger, d *collector.Database) {
	// Do not try to query the alert log if the database configuration is invalid.
	if !d.IsValid() {
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
	stmt := fmt.Sprintf(`select originating_timestamp, module_id, execution_context_id, message_text
		from v$diag_alert_ext
		where originating_timestamp > to_utc_timestamp_tz('%s')`, lastLogRecord.Timestamp)

	rows, err := d.Session.Query(stmt)
	if err != nil {
		retryAfter := databaseRetries.recordFailure(d.Name, now)
		logger.Error("Error querying the alert logs", "error", err, "database", d.Name, "retry_after", retryAfter)
		return
	}
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
		)
		if err := rows.Scan(&timestamp, &moduleID, &ecid, &message); err != nil {
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
