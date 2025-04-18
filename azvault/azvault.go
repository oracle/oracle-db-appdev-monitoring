// Copyright (c) 2023, 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package azvault

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/prometheus/common/promslog"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
)

func GetVaultSecret(vaultId string, secretName string) string {
	promLogConfig := &promslog.Config{}
	logger := promslog.New(promLogConfig)

	vaultURI := fmt.Sprintf("https://%s.vault.azure.net/", vaultId)

	// create a credential
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		logger.Error("Failed to obtain an Azure Credential", "err", err)
		os.Exit(1)
	}

	// establish a connection to the key vault client
	client, err := azsecrets.NewClient(vaultURI, cred, nil)
	if err != nil {
		logger.Error("Failed to create Azure Secrets Client", "err", err)
		os.Exit(1)
	}

	// get the secret - empty string version means "latest"
	version := ""
	secret := ""
	resp, err := client.GetSecret(context.TODO(), secretName, version, nil)
	if err != nil {
		logger.Error("Failed to get secret from vault", "err", err)
		os.Exit(1)
	} else {
		secret = *resp.Value
	}

	return strings.TrimRight(secret, "\r\n") // make sure a \r and/or \n didn't make it into the secret
}
