// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/azvault"
	"github.com/oracle/oracle-db-appdev-monitoring/hashivault"
	"github.com/oracle/oracle-db-appdev-monitoring/ocivault"
)

var (
	getOCIVaultSecret            = ocivault.GetVaultSecretContext
	getOCIUsernamePasswordSecret = ocivault.GetUsernamePasswordSecretContext
	getAZVaultSecret             = azvault.GetVaultSecret
	getHashiCorpVaultSecret      = func(logger *slog.Logger, cfg *HashiCorpVault, requiredKeys []string) (map[string]string, error) {
		client, err := hashivault.CreateVaultClient(logger, cfg.Socket)
		if err != nil {
			return nil, err
		}
		return client.GetVaultSecret(cfg.MountType, cfg.MountName, cfg.SecretPath, requiredKeys)
	}
)

type DatabaseCredentials struct {
	Username string
	Password string
}

func (h HashiCorpVault) GetUsernameAttr() string {
	if h.UsernameAttr == "" || h.MountType == hashivault.MountTypeDatabase {
		return "username"
	}
	return h.UsernameAttr
}

func (h HashiCorpVault) GetPasswordAttr() string {
	if h.PasswordAttr == "" || h.MountType == hashivault.MountTypeDatabase {
		return "password"
	}
	return h.PasswordAttr
}

func (d DatabaseConfig) fetchHashiCorpVaultSecret() error {
	if len(d.Vault.HashiCorp.fetchedSecert) > 0 {
		return nil
	}
	requiredKeys := []string{d.Vault.HashiCorp.GetUsernameAttr(), d.Vault.HashiCorp.GetPasswordAttr()}
	secret, err := getHashiCorpVaultSecret(slog.Default(), d.Vault.HashiCorp, requiredKeys)
	if err != nil {
		return err
	}
	d.Vault.HashiCorp.fetchedSecert = secret
	return nil
}

func (d DatabaseConfig) ResolveCredentials() (DatabaseCredentials, error) {
	return d.ResolveCredentialsContext(context.Background())
}

// ResolveCredentialsContext bounds OCI secret retrieval and observes caller cancellation.
func (d DatabaseConfig) ResolveCredentialsContext(ctx context.Context) (DatabaseCredentials, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return DatabaseCredentials{}, err
	}
	if d.isOCIVault() && d.Vault.OCI.UsernamePasswordSecret != "" {
		username, password, err := getOCIUsernamePasswordSecret(ctx, d.Vault.OCI.ID, d.Vault.OCI.UsernamePasswordSecret, d.Vault.OCI.Auth)
		if err != nil {
			return DatabaseCredentials{}, err
		}
		return DatabaseCredentials{Username: username, Password: password}, nil
	}
	password, err := d.resolvePassword(ctx)
	if err != nil {
		return DatabaseCredentials{}, err
	}
	username, err := d.resolveUsername(ctx)
	if err != nil {
		return DatabaseCredentials{}, err
	}
	return DatabaseCredentials{Username: username, Password: password}, nil
}

func (d DatabaseConfig) resolvePassword(ctx context.Context) (string, error) {
	switch {
	case d.PasswordFile != "":
		bytes, err := os.ReadFile(d.PasswordFile)
		if err != nil {
			return "", fmt.Errorf("failed to read password file %q: %w", d.PasswordFile, err)
		}
		return string(bytes), nil
	case d.isOCIVault() && d.Vault.OCI.PasswordSecret != "":
		return getOCIVaultSecret(ctx, d.Vault.OCI.ID, d.Vault.OCI.PasswordSecret, d.Vault.OCI.Auth)
	case d.isAzureVault() && d.Vault.Azure.PasswordSecret != "":
		return getAZVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.PasswordSecret)
	case d.isHashiCorpVault() && d.Vault.HashiCorp.MountType != "" && d.Vault.HashiCorp.MountName != "" && d.Vault.HashiCorp.SecretPath != "":
		if err := d.fetchHashiCorpVaultSecret(); err != nil {
			return "", err
		}
		return d.Vault.HashiCorp.fetchedSecert[d.Vault.HashiCorp.GetPasswordAttr()], nil
	default:
		return d.Password, nil
	}
}

func (d DatabaseConfig) resolveUsername(ctx context.Context) (string, error) {
	switch {
	case d.isOCIVault() && d.Vault.OCI.UsernameSecret != "":
		return getOCIVaultSecret(ctx, d.Vault.OCI.ID, d.Vault.OCI.UsernameSecret, d.Vault.OCI.Auth)
	case d.isAzureVault() && d.Vault.Azure.UsernameSecret != "":
		return getAZVaultSecret(d.Vault.Azure.ID, d.Vault.Azure.UsernameSecret)
	case d.isHashiCorpVault() && d.Vault.HashiCorp.MountType != "" && d.Vault.HashiCorp.MountName != "" && d.Vault.HashiCorp.SecretPath != "":
		if err := d.fetchHashiCorpVaultSecret(); err != nil {
			return "", err
		}
		username := d.Vault.HashiCorp.fetchedSecert[d.Vault.HashiCorp.GetUsernameAttr()]
		if d.Vault.HashiCorp.AsProxy != "" {
			username = fmt.Sprintf("%s[%s]", username, d.Vault.HashiCorp.AsProxy)
		}
		return username, nil
	default:
		return d.Username, nil
	}
}

func (d DatabaseConfig) isOCIVault() bool {
	return d.Vault != nil && d.Vault.OCI != nil
}

func (d DatabaseConfig) isAzureVault() bool {
	return d.Vault != nil && d.Vault.Azure != nil
}

func (d DatabaseConfig) isHashiCorpVault() bool {
	return d.Vault != nil && d.Vault.HashiCorp != nil
}

// GetUsername resolves the username. Connections should use ResolveCredentials to
// fetch a combined OCI username/password secret only once.
func (d DatabaseConfig) GetUsername() (string, error) {
	if d.isOCIVault() && d.Vault.OCI.UsernamePasswordSecret != "" {
		c, err := d.ResolveCredentials()
		return c.Username, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return d.resolveUsername(ctx)
}

func (d DatabaseConfig) GetPassword() (string, error) {
	if d.isOCIVault() && d.Vault.OCI.UsernamePasswordSecret != "" {
		c, err := d.ResolveCredentials()
		return c.Password, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return d.resolvePassword(ctx)
}
