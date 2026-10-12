// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

const shutdownTimeout = 30 * time.Second

// shutdownExporter bounds even non-context-aware database and native driver
// cleanup. On error the caller must terminate, never re-exec or resume service.
func shutdownExporter(ctx context.Context, cancelWork func(), shutdownHTTP, shutdownOTLP func(context.Context) error, workers *sync.WaitGroup, closeDatabases func() error) error {
	cancelWork()
	done := make(chan error, 1)
	go func() {
		httpErr := shutdownHTTP(ctx)
		var otlpErr error
		if shutdownOTLP != nil {
			otlpErr = shutdownOTLP(ctx)
		}
		workers.Wait()
		// An unsuccessful drain may leave collectors accessing native pools.
		if err := errors.Join(httpErr, otlpErr, ctx.Err()); err != nil {
			done <- err
			return
		}
		done <- closeDatabases()
	}()
	select {
	case err := <-done:
		return errors.Join(err, ctx.Err())
	case <-ctx.Done():
		return ctx.Err()
	}
}
