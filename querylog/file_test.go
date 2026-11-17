// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestStdoutShortWrite(t *testing.T) {
	out := NewFileOutput("stdout")
	out.Stdout = shortWriter{}
	if err := out.Deliver(context.Background(), []Row{{"ts": time.Now()}}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}
func TestFileUnterminatedTailIsPreserved(t *testing.T) {
	for _, suffix := range []string{`{"complete":true}`, `{"partial":`, strings.Repeat("x", 2<<20)} {
		path := filepath.Join(t.TempDir(), "events.log")
		original := []byte("{\"old\":true}\n" + suffix)
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		out := NewFileOutput(path)
		if err := out.Deliver(context.Background(), []Row{{"message": "new"}}); err == nil || !strings.Contains(err.Error(), "unterminated final line") {
			t.Fatalf("expected unterminated line error, got %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, original) {
			t.Fatal("existing output was modified")
		}
		// Operator repair allows this same output to recover on its next delivery.
		if err := os.WriteFile(path, []byte("{\"old\":true}\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := out.Deliver(context.Background(), []Row{{"message": "new"}}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFileAppendAndRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	out := NewFileOutput(path)
	// A large, newline-terminated record has no repair-size ceiling.
	if err := out.Deliver(context.Background(), []Row{{"message": strings.Repeat("x", 2<<20)}}); err != nil {
		t.Fatal(err)
	}
	if err := out.Deliver(context.Background(), []Row{{"message": "new"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) != 2 || !json.Valid(lines[0]) || !json.Valid(lines[1]) {
		t.Fatal("invalid appended JSON lines")
	}
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	if err := out.Deliver(context.Background(), []Row{{"message": "rotated"}}); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "rotated") {
		t.Fatal("rotation not followed")
	}
}
func TestUnsafeDestination(t *testing.T) {
	for _, name := range []string{"../escape", "..", `db\bad`} {
		if _, err := Destination("/tmp/alert.log", name, true); err == nil {
			t.Fatalf("unsafe name %q", name)
		}
	}
}

func TestJSONOutputContainsOnlyRetrievedColumns(t *testing.T) {
	var buffer bytes.Buffer
	output := NewFileOutput("stdout")
	output.Stdout = &buffer
	row := Row{"custom_name": "hello\n", "NULL_VALUE": nil, "COUNT": float64(42), "level": "ERROR"}
	if err := output.Deliver(context.Background(), []Row{row}); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, map[string]any(row)) {
		t.Fatalf("row was wrapped or changed: %s", buffer.Bytes())
	}
}
