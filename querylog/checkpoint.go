// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

const stateVersion = 2
const maxCheckpointBytes = 1024

// A checkpoint stores only the timestamp cursor and its format version.
type checkpoint struct {
	Version          int       `json:"version"`
	CompletedThrough time.Time `json:"completedThrough"`
}

type stateFile struct {
	path string
	lock *flock.Flock
}

func openState(dir, key string) (*stateFile, checkpoint, error) {
	var state checkpoint
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, state, err
	}
	path := filepath.Join(dir, Digest(key)+".json")
	lock := flock.New(path + ".lock")
	locked, err := lock.TryLock()
	if err != nil {
		return nil, state, err
	}
	if !locked {
		return nil, state, fmt.Errorf("checkpoint %q already owned by another process", path)
	}
	f := &stateFile{path: path, lock: lock}
	// Only this checkpoint's owner can create these temporary files. After
	// acquiring its lock, any remnants with this prefix belong to a prior crash.
	remnants, err := os.ReadDir(dir)
	if err != nil {
		_ = lock.Unlock()
		return nil, state, err
	}
	for _, entry := range remnants {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), filepath.Base(path)+".tmp-") {
			continue
		}
		err := os.Remove(filepath.Join(dir, entry.Name()))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = lock.Unlock()
			return nil, state, fmt.Errorf("clean checkpoint temporary file: %w", err)
		}
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, checkpoint{Version: stateVersion}, nil
	}
	if err == nil {
		var data []byte
		data, err = io.ReadAll(io.LimitReader(file, maxCheckpointBytes+1))
		err = errors.Join(err, file.Close())
		if err == nil && len(data) > maxCheckpointBytes {
			err = errors.New("checkpoint exceeds size limit")
		}
		if err == nil {
			err = json.Unmarshal(data, &state)
		}
	}
	if err == nil && (state.Version != stateVersion || state.CompletedThrough.IsZero()) {
		err = errors.New("unsupported checkpoint version or missing timestamp cursor")
	}
	if err != nil {
		_ = lock.Unlock()
		return nil, checkpoint{}, fmt.Errorf("load checkpoint %q (preserve file; stop collector before deliberate reset): %w", path, err)
	}
	return f, state, nil
}

func (f *stateFile) save(state checkpoint) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(data) > maxCheckpointBytes {
		return errors.New("checkpoint exceeds size limit")
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), filepath.Base(f.path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = writeFull(tmp, data); err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), f.path); err != nil {
		return err
	}
	// Windows does not support syncing directory handles in the same manner.
	if runtime.GOOS != "windows" {
		dir, err := os.Open(filepath.Dir(f.path))
		if err != nil {
			return err
		}
		return errors.Join(dir.Sync(), dir.Close())
	}
	return nil
}
func (f *stateFile) close() error { return f.lock.Unlock() }
