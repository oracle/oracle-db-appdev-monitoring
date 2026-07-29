// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package restart

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

type Request string

func RequestRestart(requests chan<- Request, reason Request) {
	select {
	case requests <- reason:
	default:
	}
}

func RunRestartCoordinator(ctx context.Context, logger *slog.Logger, requests <-chan Request, restart func() error) {
	for {
		select {
		case <-ctx.Done():
			return
		case reason := <-requests:
			logger.Info("Restarting the process", "reason", reason)
			if err := restart(); err != nil {
				logger.Error("Could not restart the process", "error", err)
			}
		}
	}
}

func Process() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(executable, os.Args, os.Environ())
}

func WatchConfigFile(ctx context.Context, logger *slog.Logger, configFile string, validate func() error, changed func()) error {
	configFile, err := filepath.Abs(configFile)
	if err != nil {
		return err
	}
	configFile = filepath.Clean(configFile)

	fingerprint, err := configFingerprint(configFile)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(filepath.Dir(configFile)); err != nil {
		_ = watcher.Close()
		return err
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Error("config file watcher error", "error", err)
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !isConfigChangeEvent(configFile, event) {
					continue
				}

				updatedFingerprint, err := configFingerprint(configFile)
				if err != nil {
					logger.Error("unable to read changed configuration file; keeping current process", "error", err)
					continue
				}
				if updatedFingerprint == fingerprint {
					continue
				}
				if err := validate(); err != nil {
					logger.Error("changed configuration file is invalid; keeping current process", "error", err)
					continue
				}

				fingerprint = updatedFingerprint
				changed()
			}
		}
	}()

	return nil
}

func configFingerprint(configFile string) ([sha256.Size]byte, error) {
	content, err := os.ReadFile(configFile)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(content), nil
}

func isConfigChangeEvent(configFile string, event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return false
	}

	eventName, err := filepath.Abs(event.Name)
	if err != nil {
		return false
	}
	eventName = filepath.Clean(eventName)
	if eventName == configFile {
		return true
	}

	// Kubernetes ConfigMap updates replace the ..data symlink in the mounted directory.
	return filepath.Dir(eventName) == filepath.Dir(configFile) && strings.HasPrefix(filepath.Base(eventName), "..data")
}
