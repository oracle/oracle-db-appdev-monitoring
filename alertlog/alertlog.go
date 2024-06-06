// Copyright (c) 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.package vault

package alertlog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type LogRecord struct {
	Timestamp string `json:"timestamp"`
	ModuleId  string `json:"moduleId"`
	ECID      string `json:"ecid"`
	Message   string `json:"message"`
}

func UpdateLog(logDestination string, logger log.Logger, db *sql.DB) {

	// check if the log file exists, and if not, create it
	if _, err := os.Stat(logDestination); errors.Is(err, os.ErrNotExist) {
		level.Info(logger).Log("msg", "Log destination file does not exist, will try to create it: "+logDestination)
		f, e := os.Create(logDestination)
		if e != nil {
			level.Error(logger).Log("msg", "Failed to create the log file: "+logDestination)
			return
		}
		f.Close()
	}

	// read the last line of the file to get the latest timestamp
	file, err := os.Open(logDestination)

	if err != nil {
		level.Error(logger).Log("msg", "Could not open the alert log destination file: "+logDestination)
		return
	}

	// create an empty line
	line := ""

	// the file could be very large, so we will read backwards from the end of the file
	// until the first line break is found, or until we reach the start of the file
	var pointer int64 = 0
	stat, _ := file.Stat()
	filesize := stat.Size()

	for {
		if filesize == 0 {
			break
		}

		pointer -= 1
		file.Seek(pointer, io.SeekEnd)

		char := make([]byte, 1)
		file.Read(char)

		if pointer != -1 && (char[0] == 10 || char[0] == 13) {
			// we found a new line
			break
		}

		line = fmt.Sprintf("%s%s", string(char), line)

		if pointer == -filesize {
			// we got all the way to the start of the file
			break
		}
	}

	// if we got an empty line, then set a reasonable default
	if len(line) <= 1 {
		//                    2024-06-06T14:01:22.513Z
		line = `{"timestamp":"1900-01-01T01:01:01.001Z","moduleId":"","ecid":"","message":""}`
	}

	file.Close()

	// read the timestamp from the line
	var lastLogRecord LogRecord
	err = json.Unmarshal([]byte(line), &lastLogRecord)
	if err != nil {
		level.Error(logger).Log("msg", "Could not parse last line of log file")
		return
	}

	// query for any new alert log entries
	stmt := fmt.Sprintf(`select originating_timestamp, module_id, execution_context_id, message_text
		from v$diag_alert_ext
		where originating_timestamp > to_utc_timestamp_tz('%s')`, lastLogRecord.Timestamp)

	rows, err := db.Query(stmt)
	if err != nil {
		level.Error(logger).Log("msg", "Error querying the alert logs")
		return
	}
	defer rows.Close()

	// write them to the file
	outfile, err := os.OpenFile(logDestination, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		level.Error(logger).Log("msg", "Could not open log file for writing: "+logDestination)
		return
	}

	defer outfile.Close()

	for rows.Next() {
		var newRecord LogRecord
		if err := rows.Scan(&newRecord.Timestamp, &newRecord.ModuleId, &newRecord.ECID, &newRecord.Message); err != nil {
			level.Error(logger).Log("msg", "Error reading a row from the alert logs")
			return
		}

		// strip the newline from end of message
		newRecord.Message = strings.TrimSuffix(newRecord.Message, "\n")

		jsonLogRecord, err := json.Marshal(newRecord)
		if err != nil {
			level.Error(logger).Log("msg", "Error marshalling alert log record")
			return
		}

		if _, err = outfile.WriteString(string(jsonLogRecord) + "\n"); err != nil {
			level.Error(logger).Log("msg", "Could not write to log file: "+logDestination)
			return
		}
	}

	if err = rows.Err(); err != nil {
		level.Error(logger).Log("msg", "Error querying the alert logs")
	}
}
