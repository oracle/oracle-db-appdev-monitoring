// Copyright (c) 2023, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package azvault

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

func GetVaultSecret(vaultId string, secretName string) (string, error) {
	vaultURI := fmt.Sprintf("https://%s.vault.azure.net/", vaultId)

	// create a credential
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", fmt.Errorf("obtain Azure credential: %w", err)
	}

	// establish a connection to the key vault client
	client, err := azsecrets.NewClient(vaultURI, cred, nil)
	if err != nil {
		return "", fmt.Errorf("create Azure Secrets client for vault %q: %w", vaultId, err)
	}

	// get the secret - empty string version means "latest"
	version := ""
	resp, err := client.GetSecret(context.TODO(), secretName, version, nil)
	if err != nil {
		return "", fmt.Errorf("fetch Azure Vault secret %q from vault %q: %w", secretName, vaultId, err)
	}
	if resp.Value == nil {
		return "", fmt.Errorf("Azure Vault secret %q from vault %q has no value", secretName, vaultId)
	}

	return strings.TrimRight(*resp.Value, "\r\n"), nil // make sure a \r and/or \n didn't make it into the secret
}
