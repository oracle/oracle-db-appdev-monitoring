// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocivault

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/secrets"
)

func TestGetUsernamePasswordSecret(t *testing.T) {
	credentials, err := parseUsernamePasswordSecret(`{"username":"scott","password":"tiger"}`)
	if err != nil {
		t.Fatalf("expected JSON credentials, got %v", err)
	}
	if credentials.Username != "scott" || credentials.Password != "tiger" {
		t.Fatalf("unexpected credentials: %#v", credentials)
	}
}

func TestParseUsernamePasswordSecretRejectsInvalidJSON(t *testing.T) {
	_, err := parseUsernamePasswordSecret("not JSON")
	if err == nil || !strings.Contains(err.Error(), "decode OCI Vault username/password secret") {
		t.Fatalf("expected OCI Vault JSON decode error, got %v", err)
	}
}

func TestParseUsernamePasswordSecretRejectsMissingCredentials(t *testing.T) {
	for _, secret := range []string{
		`{}`,
		`{"username":"scott"}`,
		`{"password":"tiger"}`,
		`null`,
	} {
		_, err := parseUsernamePasswordSecret(secret)
		if err == nil || !strings.Contains(err.Error(), "must include non-empty username and password") {
			t.Fatalf("expected missing credentials error for %s, got %v", secret, err)
		}
	}
}

func TestGetSecretFromBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("tiger\r\n"))

	got, err := getSecretFromBase64(secrets.GetSecretBundleByNameResponse{
		SecretBundle: secrets.SecretBundle{
			SecretBundleContent: secrets.Base64SecretBundleContentDetails{
				Content: common.String(encoded),
			},
		},
	})
	if err != nil {
		t.Fatalf("expected base64 secret to decode, got %v", err)
	}
	if got != "tiger\r\n" {
		t.Fatalf("expected decoded secret, got %q", got)
	}
}

func TestGetSecretFromBase64RejectsInvalidContent(t *testing.T) {
	_, err := getSecretFromBase64(secrets.GetSecretBundleByNameResponse{
		SecretBundle: secrets.SecretBundle{
			SecretBundleContent: secrets.Base64SecretBundleContentDetails{
				Content: common.String("not-base64"),
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid base64 content to return an error")
	}
	if !strings.Contains(err.Error(), "decode OCI Vault secret content") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestGetSecretFromBase64RejectsUnsupportedContentType(t *testing.T) {
	_, err := getSecretFromBase64(secrets.GetSecretBundleByNameResponse{
		SecretBundle: secrets.SecretBundle{
			SecretBundleContent: struct{}{},
		},
	})
	if err == nil {
		t.Fatal("expected unsupported content type to return an error")
	}
	if !strings.Contains(err.Error(), "unsupported OCI Vault secret bundle content type") {
		t.Fatalf("expected unsupported content type error, got %v", err)
	}
}
