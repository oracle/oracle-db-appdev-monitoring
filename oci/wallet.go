// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/secrets"
)

const (
	maxWalletArchiveBytes   = 16 << 20
	maxWalletExtractedBytes = 32 << 20
	maxWalletEntries        = 64
	walletDownloadTimeout   = 2 * time.Minute
)

type walletSource struct {
	auth                            AuthMode
	databaseID, vaultID, secretName string
}

// Keep each materialization until database handles are closed at process shutdown.
// Caching also prevents metadata polls from repeatedly generating ADB wallets.
var preparedWallets = struct {
	sync.Mutex
	directories map[walletSource]string
}{directories: make(map[walletSource]string)}

// PrepareWallet returns a private, process-owned wallet directory for a database.
// A named Vault secret must contain ZIP bytes (the OCI secret bundle base64 layer
// is decoded once). Without a secret, ADB generates a wallet for the database.
// Repeated requests for the same source reuse its directory until CleanupWallets.
func PrepareWallet(ctx context.Context, auth AuthMode, databaseID, vaultID, walletSecret string) (string, error) {
	if err := ValidateAuthMode(auth); err != nil {
		return "", err
	}
	if strings.TrimSpace(databaseID) == "" {
		return "", errors.New("wallet preparation requires a database OCID")
	}
	if walletSecret != "" && vaultID == "" {
		return "", errors.New("wallet secret requires a vault OCID")
	}
	source := walletSource{normalizeAuthMode(auth), databaseID, vaultID, walletSecret}
	return prepareWallet(ctx, source, func(ctx context.Context) ([]byte, error) {
		provider, err := ConfigurationProviderForAuthMode(auth)
		if err != nil {
			return nil, err
		}
		if walletSecret != "" {
			client, err := secrets.NewSecretsClientWithConfigurationProvider(provider)
			if err != nil {
				return nil, fmt.Errorf("create wallet secret client: %w", err)
			}
			return walletArchiveFromVault(ctx, client, vaultID, walletSecret)
		}
		client, err := adb.NewDatabaseClientWithConfigurationProvider(provider)
		if err != nil {
			return nil, fmt.Errorf("create wallet database client: %w", err)
		}
		return walletArchiveFromADB(ctx, client, databaseID)
	})
}

func prepareWallet(ctx context.Context, source walletSource, download func(context.Context) ([]byte, error)) (string, error) {
	// Serialize preparation and cleanup so each source has a single directory.
	preparedWallets.Lock()
	defer preparedWallets.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if directory, ok := preparedWallets.directories[source]; ok {
		return directory, nil
	}
	ctx, cancel := context.WithTimeout(ctx, walletDownloadTimeout)
	defer cancel()
	archive, err := download(ctx)
	if err != nil {
		return "", err
	}
	directory, err := extractWallet(ctx, archive)
	if err != nil {
		return "", err
	}
	preparedWallets.directories[source] = directory
	return directory, nil
}

// CleanupWallets removes process-owned wallet files. Call after discovery workers
// stop and all SQL/native connection pools close. It is safe to call repeatedly.
func CleanupWallets() error {
	preparedWallets.Lock()
	defer preparedWallets.Unlock()
	var errs []error
	for source, directory := range preparedWallets.directories {
		if err := os.RemoveAll(directory); err != nil {
			errs = append(errs, fmt.Errorf("remove OCI wallet directory: %w", err))
			continue
		}
		delete(preparedWallets.directories, source)
	}
	return errors.Join(errs...)
}

type walletDatabaseClient interface {
	GenerateAutonomousDatabaseWallet(context.Context, adb.GenerateAutonomousDatabaseWalletRequest) (adb.GenerateAutonomousDatabaseWalletResponse, error)
}

type walletSecretsClient interface {
	GetSecretBundleByName(context.Context, secrets.GetSecretBundleByNameRequest) (secrets.GetSecretBundleByNameResponse, error)
}

func walletArchiveFromADB(ctx context.Context, client walletDatabaseClient, databaseID string) ([]byte, error) {
	password, err := walletGenerationPassword()
	if err != nil {
		return nil, err
	}
	response, err := client.GenerateAutonomousDatabaseWallet(ctx, adb.GenerateAutonomousDatabaseWalletRequest{
		AutonomousDatabaseId:                    common.String(databaseID),
		GenerateAutonomousDatabaseWalletDetails: adb.GenerateAutonomousDatabaseWalletDetails{Password: &password},
	})
	if err != nil {
		return nil, fmt.Errorf("generate Autonomous Database wallet: %w", err)
	}
	if response.Content == nil {
		return nil, errors.New("ADB wallet response has no content")
	}
	defer response.Content.Close()
	if response.ContentLength != nil && *response.ContentLength > maxWalletArchiveBytes {
		return nil, errors.New("ADB wallet archive exceeds size limit")
	}
	return readWalletArchive(ctx, response.Content)
}

func walletArchiveFromVault(ctx context.Context, client walletSecretsClient, vaultID, secretName string) ([]byte, error) {
	response, err := client.GetSecretBundleByName(ctx, secrets.GetSecretBundleByNameRequest{VaultId: &vaultID, SecretName: &secretName})
	if err != nil {
		return nil, fmt.Errorf("fetch wallet secret: %w", err)
	}
	details, ok := response.SecretBundleContent.(secrets.Base64SecretBundleContentDetails)
	if !ok || details.Content == nil {
		return nil, errors.New("wallet secret must have base64 bundle content")
	}
	if len(*details.Content) > base64.StdEncoding.EncodedLen(maxWalletArchiveBytes) {
		return nil, errors.New("wallet secret exceeds size limit")
	}
	return readWalletArchive(ctx, base64.NewDecoder(base64.StdEncoding, strings.NewReader(*details.Content)))
}

func readWalletArchive(ctx context.Context, reader io.Reader) ([]byte, error) {
	archive, err := io.ReadAll(io.LimitReader(walletContextReader{ctx, reader}, maxWalletArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read wallet archive: %w", err)
	}
	if len(archive) > maxWalletArchiveBytes {
		return nil, errors.New("wallet archive exceeds size limit")
	}
	return archive, nil
}

type walletContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r walletContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func extractWallet(ctx context.Context, archive []byte) (directory string, err error) {
	if len(archive) > maxWalletArchiveBytes {
		return "", errors.New("wallet archive exceeds size limit")
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", errors.New("wallet content is not a valid ZIP archive")
	}
	if len(reader.File) > maxWalletEntries {
		return "", errors.New("wallet archive has too many entries")
	}
	directory, err = os.MkdirTemp("", "metrics-exporter-wallet-")
	if err != nil {
		return "", err
	}
	// Temp directory mode is 0700 regardless of the caller's umask.
	defer func() {
		if err != nil {
			_ = os.RemoveAll(directory)
			directory = ""
		}
	}()
	var total int64
	seen := make(map[string]bool)
	for _, file := range reader.File {
		if err = ctx.Err(); err != nil {
			return directory, err
		}
		clean := path.Clean(file.Name)
		if clean == "." || path.IsAbs(file.Name) || strings.Contains(file.Name, "\\") || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") || clean != strings.TrimSuffix(file.Name, "/") {
			return directory, errors.New("wallet archive contains an unsafe entry path")
		}
		if file.Mode()&os.ModeSymlink != 0 || (!file.FileInfo().IsDir() && !file.Mode().IsRegular()) {
			return directory, errors.New("wallet archive contains a non-regular entry")
		}
		if seen[clean] {
			return directory, errors.New("wallet archive contains a duplicate entry")
		}
		seen[clean] = true
		destination := filepath.Join(directory, filepath.FromSlash(clean))
		if file.FileInfo().IsDir() {
			if err = os.MkdirAll(destination, 0700); err != nil {
				return directory, err
			}
			continue
		}
		if file.UncompressedSize64 > maxWalletArchiveBytes || file.UncompressedSize64 > uint64(maxWalletExtractedBytes-total) {
			return directory, errors.New("wallet extracted content exceeds size limit")
		}
		if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return directory, err
		}
		var content io.ReadCloser
		content, err = file.Open()
		if err != nil {
			return directory, errors.New("cannot open wallet ZIP entry")
		}
		var output *os.File
		output, err = os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			content.Close()
			return directory, err
		}
		var count int64
		count, err = io.Copy(output, io.LimitReader(walletContextReader{ctx, content}, int64(file.UncompressedSize64)+1))
		err = errors.Join(err, content.Close(), output.Close())
		if err != nil {
			return directory, fmt.Errorf("extract wallet entry: %w", err)
		}
		if count != int64(file.UncompressedSize64) {
			return directory, errors.New("wallet ZIP entry has unexpected size")
		}
		total += count
	}
	hasWallet := false
	for _, name := range []string{"cwallet.sso", "ewallet.pem"} {
		if info, statErr := os.Stat(filepath.Join(directory, name)); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
			hasWallet = true
		}
	}
	if !hasWallet {
		return directory, errors.New("wallet archive must contain a non-empty cwallet.sso or ewallet.pem at its root")
	}
	// Write our own configuration rather than trusting an archive's filesystem
	// paths or verification settings. TNS_ADMIN points to this directory.
	sqlnet := fmt.Sprintf("WALLET_LOCATION = (SOURCE = (METHOD = file) (METHOD_DATA = (DIRECTORY=\"%s\")))\nSSL_SERVER_DN_MATCH=yes\n", directory)
	if err = os.WriteFile(filepath.Join(directory, "sqlnet.ora"), []byte(sqlnet), 0600); err != nil {
		return directory, err
	}
	return directory, nil
}
