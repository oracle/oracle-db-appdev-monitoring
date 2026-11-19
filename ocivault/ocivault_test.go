// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocivault

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

type secretGetterFunc func(context.Context, secrets.GetSecretBundleByNameRequest) (secrets.GetSecretBundleByNameResponse, error)

func (f secretGetterFunc) GetSecretBundleByName(ctx context.Context, req secrets.GetSecretBundleByNameRequest) (secrets.GetSecretBundleByNameResponse, error) {
	return f(ctx, req)
}

func TestSecretRetrievalUsesCurrentVersionAndCallerContext(t *testing.T) {
	original := newSecretClient
	t.Cleanup(func() { newSecretClient = original })
	calls := 0
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	newSecretClient = func(common.ConfigurationProvider) (secretBundleGetter, error) {
		return secretGetterFunc(func(got context.Context, req secrets.GetSecretBundleByNameRequest) (secrets.GetSecretBundleByNameResponse, error) {
			calls++
			deadline, ok := got.Deadline()
			wantDeadline, _ := ctx.Deadline()
			if !ok || !deadline.Equal(wantDeadline) {
				t.Fatal("caller deadline not preserved")
			}
			if *req.VaultId != "vault" || *req.SecretName != "credentials" {
				t.Fatalf("incorrect secret request: %#v", req)
			}
			payload := fmt.Sprintf(`{"username":"exporter","password":"version-%d"}`, calls)
			return secrets.GetSecretBundleByNameResponse{SecretBundle: secrets.SecretBundle{SecretBundleContent: secrets.Base64SecretBundleContentDetails{Content: common.String(base64.StdEncoding.EncodeToString([]byte(payload)))}}}, nil
		}), nil
	}
	for i := 1; i <= 2; i++ {
		username, password, err := GetUsernamePasswordSecretContext(ctx, "vault", "credentials", "config_file")
		if err != nil || username != "exporter" || password != fmt.Sprintf("version-%d", i) {
			t.Fatalf("failed to retrieve current credentials: username=%q err=%v", username, err)
		}
	}
}

func TestCanceledSecretRetrievalDoesNotCreateClient(t *testing.T) {
	original := newSecretClient
	t.Cleanup(func() { newSecretClient = original })
	newSecretClient = func(common.ConfigurationProvider) (secretBundleGetter, error) {
		t.Fatal("created client after cancellation")
		return nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetVaultSecretContext(ctx, "vault", "secret", "config_file"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
