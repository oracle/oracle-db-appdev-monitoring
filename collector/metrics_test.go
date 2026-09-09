// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"io"
	"log/slog"
	"testing"
)

func TestParseFloat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name     string
		metric   string
		row      map[string]string
		expected float64
		ok       bool
	}{
		{
			name:     "parses valid float",
			metric:   "value",
			row:      map[string]string{"value": "42.5"},
			expected: 42.5,
			ok:       true,
		},
		{
			name:     "trims whitespace",
			metric:   "value",
			row:      map[string]string{"value": "  7.25  "},
			expected: 7.25,
			ok:       true,
		},
		{
			name:     "treats nil string as zero",
			metric:   "value",
			row:      map[string]string{"value": "<nil>"},
			expected: 0,
			ok:       true,
		},
		{
			name:     "returns zero and false when key is missing",
			metric:   "value",
			row:      map[string]string{},
			expected: 0,
			ok:       false,
		},
		{
			name:     "returns error sentinel for invalid float",
			metric:   "value",
			row:      map[string]string{"value": "not-a-number"},
			expected: -1,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseFloat(logger, tt.metric, "help", tt.row)
			if got != tt.expected || ok != tt.ok {
				t.Fatalf("expected (%v, %v), got (%v, %v)", tt.expected, tt.ok, got, ok)
			}
		})
	}
}
