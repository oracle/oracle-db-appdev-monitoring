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
	rawSecret := getSecretFromBase64(resp)
	return strings.TrimRight(rawSecret, "\r\n"), nil // make sure a \r and/or \n didn't make it into the secret
}

func getSecretFromBase64(resp secrets.GetSecretBundleByNameResponse) string {
	base64Details, ok := resp.SecretBundleContent.(secrets.Base64SecretBundleContentDetails)
	secret := ""
	if ok {
		secretBytes, _ := b64.StdEncoding.DecodeString(*base64Details.Content)
		secret = string(secretBytes)
	}

	return secret
}
