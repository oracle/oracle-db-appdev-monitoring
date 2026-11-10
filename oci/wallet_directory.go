// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofrs/flock"
)

// Keep each materialization until database handles are closed at process shutdown.
// Caching also prevents metadata polls from repeatedly generating ADB wallets.
var preparedWallets = struct {
	sync.Mutex
	root        string
	lock        *flock.Flock
	directories map[walletSource]string
}{directories: make(map[walletSource]string)}

// InitializeWalletDirectory exclusively owns a dedicated managed-wallet root.
// After a crash the OS releases the lock; the next owner removes stale wallets.
// Never remove the root or lock file: another process may be locking that inode.
func InitializeWalletDirectory(directory string) error {
	preparedWallets.Lock()
	defer preparedWallets.Unlock()
	if strings.TrimSpace(directory) == "" {
		return errors.New("managed wallet directory is required")
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	if filepath.Dir(root) == root {
		return errors.New("managed wallet directory must not be a filesystem root")
	}
	if preparedWallets.lock != nil {
		if preparedWallets.root != root {
			return errors.New("managed wallet directory is already initialized at a different path")
		}
		return nil
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("create managed wallet directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("managed wallet path %q must be a directory, not a symlink", root)
	}
	lock := flock.New(filepath.Join(root, ".wallet.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return fmt.Errorf("lock managed wallet directory: %w", err)
	}
	if !locked {
		return fmt.Errorf("managed wallet directory %q is in use by another exporter", root)
	}
	if err := removeWalletDirectories(root); err != nil {
		return errors.Join(err, lock.Close())
	}
	preparedWallets.root, preparedWallets.lock = root, lock
	return nil
}

// CleanupWallets removes managed wallets and releases directory ownership. Call
// after discovery workers stop and all SQL/native connection pools close.
func CleanupWallets() error {
	preparedWallets.Lock()
	defer preparedWallets.Unlock()
	if preparedWallets.lock == nil {
		return nil
	}
	if err := removeWalletDirectories(preparedWallets.root); err != nil {
		return err
	}
	if err := preparedWallets.lock.Close(); err != nil {
		return err
	}
	preparedWallets.root, preparedWallets.lock = "", nil
	clear(preparedWallets.directories)
	return nil
}

func removeWalletDirectories(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wallet-") {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				errs = append(errs, fmt.Errorf("remove managed wallet directory: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}
