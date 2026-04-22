// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package main

import (
	"strings"
	"testing"
)

func TestLandingPageHTMLEscapesMetricsPath(t *testing.T) {
	originalVersion := Version
	Version = "test"
	t.Cleanup(func() {
		Version = originalVersion
	})

	body := landingPageHTML("/metrics' onclick='alert(1)")

	if strings.Contains(body, "onclick='alert(1)") {
		t.Fatalf("expected landing page to escape metrics path, got %q", body)
	}
	if !strings.Contains(body, "href='/metrics&#39; onclick=&#39;alert(1)'") {
		t.Fatalf("expected escaped metrics path in href, got %q", body)
	}
}
