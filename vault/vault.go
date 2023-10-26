// Copyright (c) 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.package vault

package vault

import (
	"context"
	b64 "encoding/base64"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/example/helpers"
	"github.com/oracle/oci-go-sdk/v65/secrets"
)

func GetVaultSecret(vaultId string, secretName string) string {
	configProvider := common.ConfigurationProviderEnvironmentVariables("vault", "")

	client, err := secrets.NewSecretsClientWithConfigurationProvider(configProvider)
	helpers.FatalIfError(err)

	req := secrets.GetSecretBundleByNameRequest{
		SecretName: common.String(secretName),
		VaultId:    common.String(vaultId)}

	resp, err := client.GetSecretBundleByName(context.Background(), req)
	helpers.FatalIfError(err)

	rawSecret := getSecretFromBase64(resp)
	return strings.TrimRight(rawSecret, "\r\n") // make sure a \r and/or \n didn't make it into the secret
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
