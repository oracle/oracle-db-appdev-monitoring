// Copyright (c) 2023, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocivault

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/secrets"
	"github.com/oracle/oracle-db-appdev-monitoring/oci"
)

type usernamePasswordSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func GetVaultSecret(vaultId string, secretName string, authMode oci.AuthMode) (string, error) {
	configProvider, err := oci.ConfigurationProviderForAuthMode(authMode)
	if err != nil {
		return "", err
	}
	client, err := secrets.NewSecretsClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("create OCI Vault client: %w", err)
	}

	req := secrets.GetSecretBundleByNameRequest{
		SecretName: common.String(secretName),
		VaultId:    common.String(vaultId)}
	resp, err := client.GetSecretBundleByName(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("fetch OCI Vault secret %q from vault %q: %w", secretName, vaultId, err)
	}
	rawSecret, err := getSecretFromBase64(resp)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(rawSecret, "\r\n"), nil // make sure a \r and/or \n didn't make it into the secret
}

func GetUsernamePasswordSecret(vaultID string, secretName string, authMode oci.AuthMode) (string, string, error) {
	secret, err := GetVaultSecret(vaultID, secretName, authMode)
	if err != nil {
		return "", "", err
	}
	credentials, err := parseUsernamePasswordSecret(secret)
	if err != nil {
		return "", "", err
	}
	return credentials.Username, credentials.Password, nil
}

func parseUsernamePasswordSecret(secret string) (usernamePasswordSecret, error) {
	credentials := usernamePasswordSecret{}
	if err := json.Unmarshal([]byte(secret), &credentials); err != nil {
		return usernamePasswordSecret{}, fmt.Errorf("decode OCI Vault username/password secret: %w", err)
	}
	if credentials.Username == "" || credentials.Password == "" {
		return usernamePasswordSecret{}, fmt.Errorf("OCI Vault username/password secret must include non-empty username and password")
	}
	return credentials, nil
}

func getSecretFromBase64(resp secrets.GetSecretBundleByNameResponse) (string, error) {
	base64Details, ok := resp.SecretBundleContent.(secrets.Base64SecretBundleContentDetails)
	if !ok {
		return "", fmt.Errorf("unsupported OCI Vault secret bundle content type %T", resp.SecretBundleContent)
	}
	if base64Details.Content == nil {
		return "", fmt.Errorf("OCI Vault secret bundle content is empty")
	}
	secretBytes, err := b64.StdEncoding.DecodeString(*base64Details.Content)
	if err != nil {
		return "", fmt.Errorf("decode OCI Vault secret content: %w", err)
	}

	return string(secretBytes), nil
}
