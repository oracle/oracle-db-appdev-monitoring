// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"
)

func initializeTestWalletDirectory(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := InitializeWalletDirectory(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := CleanupWallets(); err != nil {
			t.Error(err)
		}
	})
	return root
}

func TestWalletDirectoryOwnershipAndCleanup(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "wallet-stale")
	if err := os.Mkdir(stale, 0700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "unmanaged-wallet")
	if err := os.WriteFile(other, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	owner := flock.New(filepath.Join(root, ".wallet.lock"))
	if locked, err := owner.TryLock(); err != nil || !locked {
		t.Fatalf("acquire existing owner lock: %v", err)
	}
	defer owner.Close()
	if err := InitializeWalletDirectory(root); err == nil {
		t.Fatal("second exporter acquired an active wallet directory")
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("second exporter modified active wallets: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := InitializeWalletDirectory(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CleanupWallets() })
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale wallet was not removed: %v", err)
	}
	active := filepath.Join(root, "wallet-active")
	if err := os.Mkdir(active, 0700); err != nil {
		t.Fatal(err)
	}
	if err := InitializeWalletDirectory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("reinitialization removed a current wallet: %v", err)
	}
	if err := CleanupWallets(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(active); !os.IsNotExist(err) {
		t.Fatalf("active wallet remains after cleanup: %v", err)
	}
	if content, err := os.ReadFile(other); err != nil || string(content) != "preserve" {
		t.Fatalf("unmanaged file changed: %v", err)
	}
	if locked, err := owner.TryLock(); err != nil || !locked {
		t.Fatalf("cleanup did not release directory ownership: %v", err)
	}
}

func TestWalletDirectoryCrashRecovery(t *testing.T) {
	const rootEnv = "EXPORTER_TEST_CRASH_WALLET_DIRECTORY"
	if root := os.Getenv(rootEnv); root != "" {
		if err := InitializeWalletDirectory(root); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "wallet-abandoned"), 0700); err != nil {
			t.Fatal(err)
		}
		// Exit without cleanup, as with a process crash. The OS releases the lock.
		os.Exit(0)
	}
	root := t.TempDir()
	t.Cleanup(func() { _ = CleanupWallets() })
	for range 3 {
		child := exec.Command(os.Args[0], "-test.run=^TestWalletDirectoryCrashRecovery$")
		child.Env = append(os.Environ(), rootEnv+"="+root)
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("crashing exporter: %v\n%s", err, output)
		}
		if err := InitializeWalletDirectory(root); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 || entries[0].Name() != ".wallet.lock" {
			t.Fatalf("restart left abandoned wallets: %v, %v", entries, err)
		}
		if err := CleanupWallets(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWalletDirectoryRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"", "   ", file} {
		if err := InitializeWalletDirectory(directory); err == nil {
			t.Fatalf("accepted invalid wallet directory %q", directory)
		}
	}
	if err := os.Symlink(root, filepath.Join(root, "link")); err == nil {
		if err := InitializeWalletDirectory(filepath.Join(root, "link")); err == nil {
			t.Fatal("accepted symlink as managed wallet root")
		}
	}
}
