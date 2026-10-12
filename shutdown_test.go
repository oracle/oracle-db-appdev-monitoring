// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package main

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestShutdownExporterDrainsBeforeClosingDatabases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var workers sync.WaitGroup
	releaseWorker := make(chan struct{})
	workers.Go(func() { <-releaseWorker })
	draining := make(chan struct{})
	closed := make(chan struct{})
	var steps []string
	done := make(chan error, 1)
	go func() {
		done <- shutdownExporter(ctx, func() { steps = append(steps, "cancel") },
			func(context.Context) error { steps = append(steps, "http"); return nil },
			func(context.Context) error {
				steps = append(steps, "otlp")
				close(draining)
				return nil
			}, &workers, func() error {
				steps = append(steps, "databases")
				close(closed)
				return nil
			})
	}()
	select {
	case <-draining:
	case <-ctx.Done():
		close(releaseWorker)
		t.Fatal("shutdown did not reach worker drain")
	}
	select {
	case <-closed:
		t.Error("closed databases with a worker still running")
	default:
	}
	close(releaseWorker)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if want := []string{"cancel", "http", "otlp", "databases"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("shutdown order = %v, want %v", steps, want)
	}
}

func TestShutdownExporterDoesNotClosePoolsAfterFailedDrain(t *testing.T) {
	drainErr := errors.New("HTTP drain failed")
	var workers sync.WaitGroup
	err := shutdownExporter(context.Background(), func() {},
		func(context.Context) error { return drainErr }, nil, &workers,
		func() error { t.Error("closed databases after failed drain"); return nil })
	if !errors.Is(err, drainErr) {
		t.Fatalf("error = %v, want %v", err, drainErr)
	}
}

func TestShutdownExporterBoundsBlockingNativeCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var workers sync.WaitGroup
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- shutdownExporter(ctx, func() {}, func(context.Context) error { return nil }, nil, &workers, func() error {
			close(started)
			<-release
			close(finished)
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("native cleanup did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Error("shutdown did not return while native cleanup blocked")
	}
	close(release)
	<-finished
}
