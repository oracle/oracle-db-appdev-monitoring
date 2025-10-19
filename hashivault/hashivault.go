// Copyright (c) 2025, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package hashivault

import (
	"context"
	"strings"
	"errors"
	"net"
	"net/http"
	"time"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/example/helpers"

	"log/slog"
	vault "github.com/hashicorp/vault/api"
)

var UnsupportedMountType = errors.New("Unsupported HashiCorp Vault mount type")
var RequiredKeyMissing = errors.New("Required key missing from HashiCorp Vault secret")

type HashicorpVaultClient struct {
	client *vault.Client
	logger *slog.Logger
}

// newUnixSocketVaultClient creates a custom HTTP client using a Unix socket
func newUnixSocketVaultClient(socketPath string) (*vault.Client, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 10 * time.Second,
	}

	// Configure the Vault client
	config := &vault.Config{
		Address:      "http://unix",
		HttpClient:   httpClient,
		Timeout:      10 * time.Second,
		MinRetryWait: time.Millisecond * 1000,
		MaxRetryWait: time.Millisecond * 1500,
		MaxRetries:   2,
	}

	return vault.NewClient(config)
}

// createVaultClient connects to a vault client, using connection method specified with the parameters. Returns error if fails.
func createVaultClient(logger *slog.Logger, socketPath string) (HashicorpVaultClient,error) {
	var vaultClient HashicorpVaultClient
	var err error

	if socketPath != "" {
		// Create Vault client that uses Unix Socket
		vaultClient.client, err = newUnixSocketVaultClient(socketPath)
	}
	if err != nil {
		logger.Error("Failed to connect to HashiCorp Vault", "err", err)
	}
	vaultClient.logger = logger
	return vaultClient,err
}

// CreateVaultClient connects to a vault client, using connection method specified with the parameters. Fatal if fails.
func CreateVaultClient(logger *slog.Logger, socketPath string) HashicorpVaultClient {
	c,err := createVaultClient(logger, socketPath)
	helpers.FatalIfError(err)
	return c
}

// getVaultSecret fetches secret from vault using specified mount type. Returns error on failure.
func (c HashicorpVaultClient) getVaultSecret(mountType string, mount string, path string, requiredKeys []string) (map[string]string,error) {
	result := map[string]string{}
	var err error
	var secretData map[string]interface{}
	if mountType == "kvv2" || mountType == "kvv1" {
		// Handle simple key-value secrets
		var secret *vault.KVSecret
		c.logger.Info("Making call to HashiCorp Vault", "mountType", mountType, "mountName", mount, "secretPath", path, "expectedKeys", requiredKeys)
		if mountType == "kvv2" {
			secret, err = c.client.KVv2(mount).Get(context.TODO(), path)
		} else {
			secret, err = c.client.KVv1(mount).Get(context.TODO(), path)
		}
		if err != nil {
			c.logger.Error("Failed to fetch secret from HashiCorp Vault", "err", err)
			return result, err
		}
		secretData = secret.Data
	} else if mountType == "database" || mountType == "logical" {
		// Handle other types of secrets, for example database roles, just using the Logical() backend
		var secret *vault.Secret
		var secretPath string
		if mountType == "database" {
			secretPath = fmt.Sprintf("%s/creds/%s", mount, path)
		} else {
			secretPath = fmt.Sprintf("%s/%s", mount, path)
		}
		c.logger.Info("Making logical call to HashiCorp Vault", "mountType", mountType, "mountName", mount, "secretPath", path, "expectedKeys", requiredKeys)
		secret, err = c.client.Logical().Read(secretPath)
		if err != nil {
			c.logger.Error("Failed to fetch secret from HashiCorp Vault", "err", err)
			return result, err
		}
		secretData = secret.Data
	} else {
		c.logger.Error(UnsupportedMountType.Error())
		return result, UnsupportedMountType
	}
	// Expect simple one-level JSON, remap interface{} straight to string
	for key,val := range secretData {
		result[key] = strings.TrimRight(val.(string), "\r\n") // make sure a \r and/or \n didn't make it into the secret
	}
	// Check that we have all required keys present
	for _, key := range requiredKeys {
		val, keyExists := result[key]
		if !keyExists || val == "" {
			c.logger.Error(RequiredKeyMissing.Error(), "key", key)
			return result, RequiredKeyMissing
		}
	}
	return result, nil
}

// GetVaultSecret fetches secret from vault using specified mount type. Fatal on failure.
func (c HashicorpVaultClient) GetVaultSecret(mountType string, mount string, path string, requiredKeys []string) map[string]string {
	// Public callable function that does not return an error, just exits instead. Like other vault code in this project.
	res,err := c.getVaultSecret(mountType, mount, path, requiredKeys)
	helpers.FatalIfError(err)
	return res
}
