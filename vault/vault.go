// Copyright (c) 2023, 2024, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package vault

import (
	"context"
	b64 "encoding/base64"

	// "fmt"
	"strings"

	"github.com/go-kit/log/level"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/example/helpers"
	"github.com/oracle/oci-go-sdk/v65/secrets"
	"github.com/prometheus/common/promlog"
)

func GetVaultSecret(vaultId string, secretName string) string {
	promLogConfig := &promlog.Config{}
	logger := promlog.New(promLogConfig)

	// configProvider := common.ConfigurationProviderEnvironmentVariables("vault", "")
	// configProvider := common.DefaultConfigProvider()
	client, err := secrets.NewSecretsClientWithConfigurationProvider(common.DefaultConfigProvider())
	helpers.FatalIfError(err)

	// client, err := secrets.NewSecretsClientWithConfigurationProvider(configProvider)
	// helpers.FatalIfError(err)

	tenancyID, err := common.DefaultConfigProvider().TenancyOCID()
	helpers.FatalIfError(err)
	region, err := common.DefaultConfigProvider().Region()
	helpers.FatalIfError(err)
	// userID, err := common.DefaultConfigProvider().UserOCID()
	// helpers.FatalIfError(err)
	level.Info(logger).Log("msg", "OCI_VAULT_ID env var is present so using OCI Vault", "region-name", region)
	level.Info(logger).Log("msg", "OCI_VAULT_ID env var is present so using OCI Vault", "tenancyOCID", tenancyID)
	// level.Info(logger).Log("msg", "User ID", "user-id", userID)

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
