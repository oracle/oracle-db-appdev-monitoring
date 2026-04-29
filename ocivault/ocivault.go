// Copyright (c) 2023, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocivault

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/secrets"
)

func GetVaultSecret(vaultId string, secretName string) (string, error) {
	client, err := secrets.NewSecretsClientWithConfigurationProvider(common.DefaultConfigProvider())
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
