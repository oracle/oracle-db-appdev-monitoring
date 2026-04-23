// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package hashivault

import (
	"io"
	"log/slog"
	"reflect"
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

func TestCopyStringSecretDataIgnoresNonStringValues(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := HashicorpVaultClient{logger: logger}

	result := map[string]string{}
	client.copyStringSecretData(result, map[string]interface{}{
		"username": "scott\n",
		"password": "tiger\r\n",
		"ttl":      3600,
		"enabled":  true,
		"metadata": map[string]interface{}{"env": "dev"},
		"null":     nil,
	})

	want := map[string]string{
		"username": "scott",
		"password": "tiger",
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("unexpected remapped secret data: got %#v want %#v", result, want)
	}
}

func TestCopyStringSecretDataLeavesRequiredKeyValidationToCaller(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := HashicorpVaultClient{logger: logger}

	result := map[string]string{}
	client.copyStringSecretData(result, map[string]interface{}{
		"username": "scott",
		"ttl":      3600,
	})

	requiredKeys := []string{"username", "password"}
	for _, key := range requiredKeys {
		val, ok := result[key]
		if key == "password" {
			if ok || val != "" {
				t.Fatalf("expected missing required key %q after filtering non-string values, got ok=%v val=%q", key, ok, val)
			}
			continue
		}
		if !ok || val == "" {
			t.Fatalf("expected required key %q to remain present, got ok=%v val=%q", key, ok, val)
		}
	}
}
