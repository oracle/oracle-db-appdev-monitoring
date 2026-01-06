// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"testing"
	"time"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		name         string
		invalidUntil *time.Time
		expected     bool
	}{
		{
			name:         "Nil invalidUntil",
			invalidUntil: nil,
			expected:     true,
		},
		{
			name:         "Future invalidUntil",
			invalidUntil: func() *time.Time { t := time.Now().Add(time.Minute); return &t }(),
			expected:     false,
		},
		{
			name:         "Past invalidUntil",
			invalidUntil: func() *time.Time { t := time.Now().Add(-time.Minute); return &t }(),
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &Database{invalidUntil: tt.invalidUntil}
			result := db.IsValid()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestInvalidate(t *testing.T) {
	db := &Database{}
	backoff := time.Minute
	db.invalidate(backoff)
	if db.invalidUntil == nil {
		t.Fatal("Expected non-nil invalidUntil")
	}
	if time.Now().After(*db.invalidUntil) {
		t.Error("Expected invalidUntil in the future")
	}
}
