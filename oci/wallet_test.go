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
	"path/filepath"
	"strings"
	"testing"

	adb "github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/secrets"
)

func walletZIP(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestExtractWalletPermissionsAndSQLNet(t *testing.T) {
	dir, err := extractWallet(context.Background(), walletZIP(t, map[string]string{
		"cwallet.sso": "wallet bytes", "tnsnames.ora": "aliases", "sqlnet.ora": "SSL_SERVER_DN_MATCH=no\nWALLET_LOCATION=\"?/network/admin\"",
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("wallet directory permissions: %v, %v", info, err)
	}
	for _, name := range []string{"cwallet.sso", "sqlnet.ora", "tnsnames.ora"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("wallet file permissions: %v, %v", info, err)
		}
	}
	sqlnet, err := os.ReadFile(filepath.Join(dir, "sqlnet.ora"))
	if err != nil || !strings.Contains(string(sqlnet), "DIRECTORY=\""+dir+"\"") || !strings.Contains(string(sqlnet), "SSL_SERVER_DN_MATCH=yes") || strings.Contains(string(sqlnet), "SSL_SERVER_DN_MATCH=no") {
		t.Fatalf("sqlnet configuration not rewritten: %q, %v", sqlnet, err)
	}
}

func TestExtractWalletRejectsUnsafeEntries(t *testing.T) {
	for _, path := range []string{"../escape", "/absolute", "nested/../../escape", "windows\\escape", "C:/escape", "normalized/../escape"} {
		t.Run(path, func(t *testing.T) {
			directory, err := extractWallet(context.Background(), walletZIP(t, map[string]string{"cwallet.sso": "wallet", path: "unsafe"}))
			if err == nil || directory != "" {
				t.Fatalf("unsafe wallet extracted into %q: %v", directory, err)
			}
		})
	}
}

func TestExtractWalletRejectsSymlinkAndDuplicate(t *testing.T) {
	for _, symlink := range []bool{true, false} {
		var output bytes.Buffer
		writer := zip.NewWriter(&output)
		header := &zip.FileHeader{Name: "cwallet.sso"}
		if symlink {
			header.SetMode(os.ModeSymlink | 0777)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(entry, "content")
		if !symlink {
			entry, err = writer.Create("cwallet.sso")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(entry, "duplicate")
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		directory, err := extractWallet(context.Background(), output.Bytes())
		if err == nil || directory != "" {
			t.Fatalf("unsafe wallet extracted into %q: %v", directory, err)
		}
	}
}

func TestExtractWalletRejectsMissingWalletAndLimits(t *testing.T) {
	for name, archive := range map[string][]byte{
		"invalid ZIP":     []byte("not ZIP"),
		"missing cwallet": walletZIP(t, map[string]string{"ewallet.p12": "wallet"}),
		"empty cwallet":   walletZIP(t, map[string]string{"cwallet.sso": ""}),
		"oversized ZIP":   make([]byte, maxWalletArchiveBytes+1),
		"oversized entry": walletZIP(t, map[string]string{"cwallet.sso": strings.Repeat("x", maxWalletArchiveBytes+1)}),
	} {
		t.Run(name, func(t *testing.T) {
			directory, err := extractWallet(context.Background(), archive)
			if err == nil || directory != "" {
				t.Fatalf("invalid wallet extracted into %q: %v", directory, err)
			}
		})
	}
}

func TestPrepareWalletCachesPerResourceAndCleansUp(t *testing.T) {
	if err := CleanupWallets(); err != nil {
		t.Fatal(err)
	}
	defer CleanupWallets()
	calls := 0
	archive := walletZIP(t, map[string]string{"cwallet.sso": "wallet"})
	download := func(context.Context) ([]byte, error) { calls++; return archive, nil }
	first, err := prepareWallet(context.Background(), walletSource{databaseID: "first"}, download)
	if err != nil {
		t.Fatal(err)
	}
	again, err := prepareWallet(context.Background(), walletSource{databaseID: "first"}, download)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareWallet(context.Background(), walletSource{databaseID: "second"}, download)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || first != again || first == second {
		t.Fatalf("unexpected materializations: calls=%d first=%q again=%q second=%q", calls, first, again, second)
	}
	if err := CleanupWallets(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{first, second} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("wallet directory remains after cleanup: %q %v", dir, err)
		}
	}
	if err := CleanupWallets(); err != nil {
		t.Fatal(err)
	}
}

func TestWalletCancellationAndFailedDownloads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := prepareWallet(ctx, walletSource{databaseID: "cancelled"}, func(context.Context) ([]byte, error) { t.Fatal("download called after cancellation"); return nil, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	want := errors.New("download failed")
	_, err = prepareWallet(context.Background(), walletSource{databaseID: "failed"}, func(context.Context) ([]byte, error) { return nil, want })
	if !errors.Is(err, want) {
		t.Fatalf("download error not preserved: %v", err)
	}
	archive := walletZIP(t, map[string]string{"cwallet.sso": "wallet"})
	_, err = extractWallet(ctx, archive)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("extraction ignored cancellation: %v", err)
	}
}

type walletADBFunc func(context.Context, adb.GenerateAutonomousDatabaseWalletRequest) (adb.GenerateAutonomousDatabaseWalletResponse, error)

func (f walletADBFunc) GenerateAutonomousDatabaseWallet(ctx context.Context, req adb.GenerateAutonomousDatabaseWalletRequest) (adb.GenerateAutonomousDatabaseWalletResponse, error) {
	return f(ctx, req)
}

type walletVaultFunc func(context.Context, secrets.GetSecretBundleByNameRequest) (secrets.GetSecretBundleByNameResponse, error)

func (f walletVaultFunc) GetSecretBundleByName(ctx context.Context, req secrets.GetSecretBundleByNameRequest) (secrets.GetSecretBundleByNameResponse, error) {
	return f(ctx, req)
}

type walletReadCloser struct {
	io.Reader
	closed bool
}

func (r *walletReadCloser) Close() error { r.closed = true; return nil }

func TestWalletArchiveFromADBUsesContextAndClosesBody(t *testing.T) {
	t.Setenv("oracle.go.wallet_password", "")
	t.Setenv("ORACLE_GO_WALLET_PASSWORD", "Test-Wallet-Password-With-Enough-Length-123!")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &walletReadCloser{Reader: strings.NewReader("archive")}
	client := walletADBFunc(func(got context.Context, req adb.GenerateAutonomousDatabaseWalletRequest) (adb.GenerateAutonomousDatabaseWalletResponse, error) {
		if got != ctx || req.AutonomousDatabaseId == nil || *req.AutonomousDatabaseId != "database" || req.Password == nil || len(*req.Password) < 32 {
			t.Fatalf("unexpected wallet request")
		}
		return adb.GenerateAutonomousDatabaseWalletResponse{Content: body}, nil
	})
	data, err := walletArchiveFromADB(ctx, client, "database")
	if err != nil || string(data) != "archive" || !body.closed {
		t.Fatalf("unexpected wallet response: bytes=%d closed=%v err=%v", len(data), body.closed, err)
	}
}

func TestWalletArchiveFromVaultDecodesBinaryZIPOnce(t *testing.T) {
	archive := walletZIP(t, map[string]string{"cwallet.sso": "wallet\x00\xff\r\n"})
	encoded := base64.StdEncoding.EncodeToString(archive)
	client := walletVaultFunc(func(ctx context.Context, req secrets.GetSecretBundleByNameRequest) (secrets.GetSecretBundleByNameResponse, error) {
		if *req.VaultId != "vault" || *req.SecretName != "secret" {
			t.Fatal("unexpected secret request")
		}
		return secrets.GetSecretBundleByNameResponse{SecretBundle: secrets.SecretBundle{SecretBundleContent: secrets.Base64SecretBundleContentDetails{Content: &encoded}}}, nil
	})
	data, err := walletArchiveFromVault(context.Background(), client, "vault", "secret")
	if err != nil || !bytes.Equal(data, archive) {
		t.Fatalf("binary wallet archive changed: %v", err)
	}
	encoded = "not base64"
	if _, err := walletArchiveFromVault(context.Background(), client, "vault", "secret"); err == nil {
		t.Fatal("invalid base64 accepted")
	}
}

func TestExtractWalletFailureRemovesPrivateDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	_, err := extractWallet(context.Background(), walletZIP(t, map[string]string{"cwallet.sso": "wallet", "../unsafe": "escape"}))
	if err == nil {
		t.Fatal("unsafe archive accepted")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed extraction left files behind: %v, %v", entries, err)
	}
}

func TestExtractWalletRejectsTooManyEntries(t *testing.T) {
	entries := map[string]string{"cwallet.sso": "wallet"}
	for i := 0; i < maxWalletEntries; i++ {
		entries[fmt.Sprintf("file-%d", i)] = "content"
	}
	if directory, err := extractWallet(context.Background(), walletZIP(t, entries)); err == nil || directory != "" {
		t.Fatalf("oversized entry count accepted: %q, %v", directory, err)
	}
}

func TestExtractPEMOnlyWalletPreservesSuppliedFile(t *testing.T) {
	const supplied = "supplied encrypted or unencrypted PEM bytes"
	directory, err := extractWallet(context.Background(), walletZIP(t, map[string]string{"ewallet.pem": supplied}))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	content, err := os.ReadFile(filepath.Join(directory, "ewallet.pem"))
	if err != nil || string(content) != supplied {
		t.Fatal("supplied PEM was modified")
	}
	if _, err := os.Stat(filepath.Join(directory, "cwallet.sso")); !os.IsNotExist(err) {
		t.Fatal("wallet material was synthesized")
	}
}
