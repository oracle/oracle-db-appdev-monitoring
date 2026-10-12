// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLandingPageHTMLEscapesMetricsPath(t *testing.T) {
	originalVersion := Version
	Version = "test"
	t.Cleanup(func() {
		Version = originalVersion
	})

	body := landingPageHTML("/metrics' onclick='alert(1)")

	if strings.Contains(body, "onclick='alert(1)") {
		t.Fatalf("expected landing page to escape metrics path, got %q", body)
	}
	if !strings.Contains(body, "href='/metrics&#39; onclick=&#39;alert(1)'") {
		t.Fatalf("expected escaped metrics path in href, got %q", body)
	}
}

func TestParseConfigFileExplicitFlag(t *testing.T) {
	got, err := parseConfigFile([]string{"--config.file", "example-config.yaml"}, emptyEnv, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("expected config flag to parse, got %v", err)
	}
	if got != "example-config.yaml" {
		t.Fatalf("expected explicit config file, got %q", got)
	}
}

func TestParseConfigFileFromEnvironment(t *testing.T) {
	got, err := parseConfigFile(nil, func(key string) string {
		if key == "CONFIG_FILE" {
			return "env-config.yaml"
		}
		return ""
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("expected CONFIG_FILE fallback to parse, got %v", err)
	}
	if got != "env-config.yaml" {
		t.Fatalf("expected environment config file, got %q", got)
	}
}

func TestParseConfigFileRequiresConfig(t *testing.T) {
	_, err := parseConfigFile(nil, emptyEnv, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected missing config file to fail")
	}
	if !strings.Contains(err.Error(), "config file is required") {
		t.Fatalf("expected required config error, got %v", err)
	}
}

func TestParseConfigFileRejectsRemovedFlags(t *testing.T) {
	_, err := parseConfigFile([]string{"--web.telemetry-path", "/metrics"}, emptyEnv, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected removed flag to fail")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}
}

func emptyEnv(string) string {
	return ""
}

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
