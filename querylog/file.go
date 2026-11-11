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
	"strings"
	"sync"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

// FileOutput opens the path on every delivery, so rename-and-create rotation
// needs no signal. The shared destination mutex serializes complete JSON lines.
type FileOutput struct {
	Path   string
	mu     *sync.Mutex
	Stdout io.Writer
}

var destinationLocks sync.Map

func NewFileOutput(path string) *FileOutput {
	key := path
	if !config.IsStdout(path) {
		if abs, err := filepath.Abs(path); err == nil {
			key = abs
		}
	}
	lock, _ := destinationLocks.LoadOrStore(key, &sync.Mutex{})
	return &FileOutput{Path: path, mu: lock.(*sync.Mutex), Stdout: os.Stdout}
}
func Destination(path, database string, perDatabase bool) (string, error) {
	if !perDatabase {
		return path, nil
	}
	if config.IsStdout(path) {
		return "", errors.New("stdout cannot use per-database files")
	}
	if database == "." || database == ".." || filepath.Base(database) != database || strings.ContainsAny(database, "/\\\x00") {
		return "", errors.New("unsafe database filename")
	}
	ext := filepath.Ext(path)
	return filepath.Join(filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), ext)+"-"+database+ext), nil
}
func (s *FileOutput) Deliver(ctx context.Context, rows []Row) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	var data bytes.Buffer
	for _, row := range rows {
		if err := json.NewEncoder(&data).Encode(row); err != nil {
			return err
		}
	}
	if config.IsStdout(s.Path) {
		return writeFull(s.Stdout, data.Bytes())
	}
	file, err := os.OpenFile(s.Path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	err = checkTail(file)
	if err == nil {
		err = writeFull(file, data.Bytes())
	}
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}
func (s *FileOutput) Close() error { return nil }
func writeFull(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

// checkTail refuses to append to an unterminated line. Existing output is never
// repaired or truncated automatically; operators retain control of recovery.
func checkTail(file *os.File) error {
	stat, err := file.Stat()
	if err != nil || stat.Size() == 0 {
		return err
	}
	var last [1]byte
	if _, err := file.ReadAt(last[:], stat.Size()-1); err != nil {
		return err
	}
	if last[0] != '\n' {
		return errors.New("output file has an unterminated final line; add a newline or repair the file before retrying")
	}
	return nil
}
