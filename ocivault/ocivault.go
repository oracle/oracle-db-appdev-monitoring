// Copyright (c) 2023, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocivault

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/secrets"
)

type AuthMode string

const (
	AuthModeConfigFile        AuthMode = "config_file"
	AuthModeInstancePrincipal AuthMode = "instance_principal"
	AuthModeResourcePrincipal AuthMode = "resource_principal"
	AuthModeWorkloadIdentity  AuthMode = "workload_identity"
)

var (
	defaultConfigProvider = func() (common.ConfigurationProvider, error) {
		return common.DefaultConfigProvider(), nil
	}
	instancePrincipalConfigurationProvider = func() (common.ConfigurationProvider, error) {
		return auth.InstancePrincipalConfigurationProvider()
	}
	resourcePrincipalConfigurationProvider = func() (common.ConfigurationProvider, error) {
		return auth.ResourcePrincipalConfigurationProvider()
	}
	workloadIdentityConfigurationProvider = func() (common.ConfigurationProvider, error) {
		return auth.OkeWorkloadIdentityConfigurationProvider()
	}
)

func AcceptedAuthModes() []string {
	return []string{
		string(AuthModeConfigFile),
		string(AuthModeInstancePrincipal),
		string(AuthModeResourcePrincipal),
		string(AuthModeWorkloadIdentity),
	}
}

func ValidateAuthMode(authMode AuthMode) error {
	switch normalizeAuthMode(authMode) {
	case AuthModeConfigFile, AuthModeInstancePrincipal, AuthModeResourcePrincipal, AuthModeWorkloadIdentity:
		return nil
	default:
		return fmt.Errorf("unsupported OCI Vault auth mode %q; accepted values are: %s", authMode, strings.Join(AcceptedAuthModes(), ", "))
	}
}

func GetVaultSecretWithAuth(vaultId string, secretName string, authMode AuthMode) (string, error) {
	configProvider, err := configurationProviderForAuthMode(authMode)
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

func configurationProviderForAuthMode(authMode AuthMode) (common.ConfigurationProvider, error) {
	mode := normalizeAuthMode(authMode)
	switch mode {
	case AuthModeConfigFile:
		return defaultConfigProvider()
	case AuthModeInstancePrincipal:
		return instancePrincipalConfigurationProvider()
	case AuthModeResourcePrincipal:
		return resourcePrincipalConfigurationProvider()
	case AuthModeWorkloadIdentity:
		return workloadIdentityConfigurationProvider()
	default:
		return defaultConfigProvider()
	}
}

func normalizeAuthMode(authMode AuthMode) AuthMode {
	mode := strings.ToLower(strings.TrimSpace(string(authMode)))
	if mode == "" {
		return AuthModeConfigFile
	}
	return AuthMode(mode)
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
