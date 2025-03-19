// Copyright (c) 2023, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package azvault

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/common/promslog"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
)

func GetVaultSecret(vaultId string, secretName string) string {
	promLogConfig := &promslog.Config{}
	logger := promslog.New(promLogConfig)

	logger.Info("AZ_VAULT_ID env var is present so using Azure Key Vault", "vaultID", vaultId)

	vaultURI := fmt.Sprintf("https://%s.vault.azure.net/", vaultId)

	// create a credential
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		logger.Error("Failed to obtain an Azure Credential", "err", err)
	}

	// establish a connection to the key vault client
	client, err := azsecrets.NewClient(vaultURI, cred, nil)

	// get the secret - empty string version means "latest"
	version := ""
	resp, err := client.GetSecret(context.TODO(), secretName, version, nil)

	rawSecret := *resp.Value
	return strings.TrimRight(rawSecret, "\r\n") // make sure a \r and/or \n didn't make it into the secret
}
