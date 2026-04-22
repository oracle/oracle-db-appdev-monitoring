// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package hashivault

import (
	"io"
	"log/slog"
	"testing"
)

func TestCreateVaultClientFallsBackToDefaultConfigWithoutProxySocket(t *testing.T) {
	t.Setenv("VAULT_ADDR", "http://127.0.0.1:8200")
	t.Setenv("VAULT_TOKEN", "test-token")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	client, err := CreateVaultClient(logger, "")
	if err != nil {
		t.Fatalf("expected default Vault client without proxy socket, got %v", err)
	}
	if client.client == nil {
		t.Fatal("expected non-nil client when proxy socket is not configured")
	}
}
