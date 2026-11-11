// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/db"
)

// BoundaryFormat matches TO_TIMESTAMP_TZ(:1, BoundaryMask). Explicit timezone and
// fractional seconds avoid implicit NLS formats and driver-specific DATE binds.
const BoundaryFormat = "2006-01-02T15:04:05.000000000-07:00"
const sourceTimeQuery = `select systimestamp from dual`

type SQLSource struct {
	Database *db.Database
	Query    string
}

type sqlReader struct {
	conn    *sql.Conn
	release func() error
	now     time.Time
	source  *SQLSource
}

func (s *SQLSource) Open(ctx context.Context) (Reader, error) {
	if !s.Database.StartupReady() || s.Database.IsValid() != nil {
		return nil, ErrNotReady
	}
	conn, release, err := s.Database.ConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	r := &sqlReader{conn: conn, release: release, source: s}
	if err := conn.QueryRowContext(ctx, sourceTimeQuery).Scan(&r.now); err != nil {
		_ = release()
		return nil, fmt.Errorf("read source time: %w", err)
	}
	return r, nil
}
func (r *sqlReader) Now() time.Time { return r.now }
func (r *sqlReader) Close() error {
	if r.release == nil {
		return nil
	}
	release := r.release
	r.release = nil
	return release()
}
func (r *sqlReader) Read(ctx context.Context, from, to time.Time, maxRows, maxBytes int) ([]Row, error) {
	rows, err := r.conn.QueryContext(ctx, r.source.Query, from.UTC().Format(BoundaryFormat), to.UTC().Format(BoundaryFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			return nil, fmt.Errorf("duplicate query column %q; use unique SQL aliases", name)
		}
		seen[name] = true
	}
	result := make([]Row, 0, min(maxRows, 256))
	values := make([]any, len(names))
	dest := make([]any, len(names))
	for i := range values {
		dest[i] = &values[i]
	}
	size := 0
	for rows.Next() {
		if len(result) >= maxRows {
			return nil, ErrLimit
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make(Row, len(names))
		for i, name := range names {
			row[name] = values[i]
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		size += len(encoded)
		if size > maxBytes {
			return nil, ErrLimit
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, rows.Close()
}
