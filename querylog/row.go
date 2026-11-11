// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Row contains exactly the columns returned by a query, keyed by column name.
type Row map[string]any

// Timestamp reads the cursor column without changing the row's output shape.
func (r Row) Timestamp(column string) (time.Time, error) {
	value, found := r[column]
	if !found {
		for name, candidate := range r {
			if strings.EqualFold(name, column) {
				if found {
					return time.Time{}, fmt.Errorf("timestamp column %q is ambiguous; use an exact column name", column)
				}
				value, found = candidate, true
			}
		}
	}
	if !found {
		return time.Time{}, fmt.Errorf("timestamp column %q is absent", column)
	}
	if timestamp, ok := value.(time.Time); ok && !timestamp.IsZero() {
		return timestamp.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("timestamp column %q must return a non-null native timestamp", column)
}

func Digest(v any) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

var ErrLimit = errors.New("source read exceeds configured row/byte limit")
var ErrNotReady = errors.New("database is not ready")
var ErrRejected = errors.New("output rejected records")

// Reader owns a database session. Close must release it before output delivery.
type Reader interface {
	Now() time.Time
	Read(context.Context, time.Time, time.Time, int, int) ([]Row, error)
	Close() error
}

type Source interface {
	Open(context.Context) (Reader, error)
}

type Output interface {
	Deliver(context.Context, []Row) error
	Close() error
}
