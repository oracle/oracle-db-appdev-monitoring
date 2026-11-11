// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"
)

type rowConnector struct {
	columns []string
	values  []driver.Value
	args    *[]driver.NamedValue
}

func (c rowConnector) Connect(context.Context) (driver.Conn, error) { return rowConn{c}, nil }
func (c rowConnector) Driver() driver.Driver                        { return rowDriver{} }

type rowDriver struct{}

func (rowDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type rowConn struct{ rowConnector }

func (c rowConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unsupported") }
func (c rowConn) Close() error                        { return nil }
func (c rowConn) Begin() (driver.Tx, error)           { return nil, errors.New("unsupported") }
func (c rowConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if c.args != nil {
		*c.args = args
	}
	return &rowRows{columns: c.columns, values: c.values}, nil
}

type rowRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *rowRows) Columns() []string { return r.columns }
func (r *rowRows) Close() error      { return nil }
func (r *rowRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)
	return nil
}
func rowDB(t *testing.T, columns []string, values []driver.Value, args *[]driver.NamedValue) *sql.DB {
	t.Helper()
	database := sql.OpenDB(rowConnector{columns: columns, values: values, args: args})
	t.Cleanup(func() { _ = database.Close() })
	return database
}
func newRowReader(t *testing.T, columns []string, values []driver.Value, args *[]driver.NamedValue) *sqlReader {
	t.Helper()
	database := rowDB(t, columns, values, args)
	conn, err := database.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reader := &sqlReader{conn: conn, release: conn.Close, source: &SQLSource{Query: "SELECT"}}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

func TestSQLReaderPreservesRetrievedColumns(t *testing.T) {
	ts := time.Date(2026, 11, 11, 12, 0, 0, 123456789, time.FixedZone("offset", 3600))
	columns := []string{"TS", "message_text", "NULL_VALUE", "ID", "AMOUNT", "RATIO", "BINARY", "ACTIVE"}
	values := []driver.Value{ts, "hello\n", nil, int64(42), "999999999999999999999999.123456789", float64(1.2), []byte{0, 255}, true}
	reader := newRowReader(t, columns, values, nil)
	rows, err := reader.Read(context.Background(), ts.Add(-time.Minute), ts, 10, 4096)
	if err != nil {
		t.Fatal(err)
	}
	want := Row{}
	for i, column := range columns {
		want[column] = values[i]
	}
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("columns or values changed: %+v", rows)
	}
	timestamp, err := rows[0].Timestamp("ts")
	if err != nil || !timestamp.Equal(ts) {
		t.Fatalf("cursor column: %v, %v", timestamp, err)
	}
}

func TestSQLReaderRejectsDuplicateColumns(t *testing.T) {
	reader := newRowReader(t, []string{"ID", "ID"}, []driver.Value{int64(1), int64(2)}, nil)
	if _, err := reader.Read(context.Background(), time.Now(), time.Now(), 10, 1024); err == nil {
		t.Fatal("duplicate columns silently overwrote a value")
	}
}

func TestRowRequiresNativeTimestampCursor(t *testing.T) {
	for _, row := range []Row{{}, {"TS": nil}, {"TS": "2026-11-11T12:00:00Z"}, {"TS": time.Time{}}} {
		if _, err := row.Timestamp("ts"); err == nil {
			t.Fatalf("invalid timestamp accepted: %+v", row)
		}
	}
}

func TestSQLReaderBindsBoundariesAndBoundsBytes(t *testing.T) {
	now := time.Now().UTC()
	var args []driver.NamedValue
	reader := newRowReader(t, []string{"TS", "MESSAGE"}, []driver.Value{now, "hello"}, &args)
	rows, err := reader.Read(context.Background(), now.Add(-time.Minute), now, 10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0].Value != now.Add(-time.Minute).Format(BoundaryFormat) || args[1].Value != now.Format(BoundaryFormat) {
		t.Fatalf("unsafe/wrong bind: %#v", args)
	}
	if len(rows) != 1 || len(rows[0]) != 2 {
		t.Fatalf("output contains an envelope: %+v", rows)
	}
	if _, err := reader.Read(context.Background(), now.Add(-time.Minute), now, 10, 1); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func TestSQLReaderPreservesColumnsDifferingOnlyByCase(t *testing.T) {
	reader := newRowReader(t, []string{"ID", "id"}, []driver.Value{int64(1), int64(2)}, nil)
	rows, err := reader.Read(context.Background(), time.Now(), time.Now(), 10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 2 || rows[0]["ID"] != int64(1) || rows[0]["id"] != int64(2) {
		t.Fatalf("distinct columns lost: %+v", rows)
	}
}

func TestTimestampColumnMatchesExactNameBeforeCaseInsensitiveFallback(t *testing.T) {
	first := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Second)
	row := Row{"TS": first, "ts": second}
	for column, want := range map[string]time.Time{"TS": first, "ts": second} {
		got, err := row.Timestamp(column)
		if err != nil || !got.Equal(want) {
			t.Fatalf("%s: got %v, %v; want %v", column, got, err, want)
		}
	}
	if _, err := row.Timestamp("Ts"); err == nil {
		t.Fatal("ambiguous timestamp selection accepted")
	}
	got, err := (Row{"TS": first, "ID": 1, "id": 2}).Timestamp("ts")
	if err != nil || !got.Equal(first) {
		t.Fatalf("unique case-insensitive match failed: %v, %v", got, err)
	}
}
