// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"context"
	"crypto/rsa"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type fakeConfigurationProvider struct {
	name string
}

func (f fakeConfigurationProvider) PrivateRSAKey() (*rsa.PrivateKey, error) { return nil, nil }
func (f fakeConfigurationProvider) KeyID() (string, error)                  { return f.name, nil }
func (f fakeConfigurationProvider) TenancyOCID() (string, error)            { return "tenancy", nil }
func (f fakeConfigurationProvider) UserOCID() (string, error)               { return "user", nil }
func (f fakeConfigurationProvider) KeyFingerprint() (string, error)         { return "fingerprint", nil }
func (f fakeConfigurationProvider) Region() (string, error)                 { return "us-ashburn-1", nil }
func (f fakeConfigurationProvider) AuthType() (common.AuthConfig, error) {
	return common.AuthConfig{}, nil
}

func TestConfigurationProviderForAuthModeUsesSelectedFactory(t *testing.T) {
	originalDefault := defaultConfigProvider
	originalInstance := instancePrincipal
	originalResource := resourcePrincipalConfigurationProvider
	originalWorkload := workloadIdentityConfigurationProvider
	t.Cleanup(func() {
		defaultConfigProvider = originalDefault
		instancePrincipal = originalInstance
		resourcePrincipalConfigurationProvider = originalResource
		workloadIdentityConfigurationProvider = originalWorkload
	})

	defaultConfigProvider = providerFactoryForTest("config_file")
	instancePrincipal = &instancePrincipalProvider{create: providerFactoryForTest("instance_principal")}
	resourcePrincipalConfigurationProvider = providerFactoryForTest("resource_principal")
	workloadIdentityConfigurationProvider = providerFactoryForTest("workload_identity")

	tests := []struct {
		name     string
		authMode AuthMode
		want     string
	}{
		{name: "empty defaults to config file", authMode: "", want: "config_file"},
		{name: "config file", authMode: "config_file", want: "config_file"},
		{name: "instance principal", authMode: "instance_principal", want: "instance_principal"},
		{name: "resource principal", authMode: "resource_principal", want: "resource_principal"},
		{name: "workload identity", authMode: "workload_identity", want: "workload_identity"},
		{name: "trims and lowercases", authMode: " Instance_Principal ", want: "instance_principal"},
		{name: "unsupported runtime value falls back to config file", authMode: "api_key", want: "config_file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := ConfigurationProviderForAuthMode(tt.authMode)
			if err != nil {
				t.Fatalf("expected provider for auth mode %q, got %v", tt.authMode, err)
			}
			fake, ok := provider.(fakeConfigurationProvider)
			if !ok {
				t.Fatalf("expected fake provider, got %T", provider)
			}
			if fake.name != tt.want {
				t.Fatalf("expected provider %q, got %q", tt.want, fake.name)
			}
		})
	}
}

func TestConfigurationProviderForAuthModeReturnsFactoryError(t *testing.T) {
	original := instancePrincipal
	t.Cleanup(func() { instancePrincipal = original })
	instancePrincipal = &instancePrincipalProvider{create: func() (common.ConfigurationProvider, error) {
		return nil, errors.New("provider unavailable")
	}}

	_, err := ConfigurationProviderForAuthMode("instance_principal")
	if err == nil {
		t.Fatal("expected provider factory error")
	}
	if err.Error() != "provider unavailable" {
		t.Fatalf("expected provider factory error to be returned, got %v", err)
	}
}

func TestInstancePrincipalCallerCancellationSharesBootstrap(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{}, 1)
	defer close(release)
	var calls atomic.Int32
	provider := gatedInstancePrincipal{
		fakeConfigurationProvider: fakeConfigurationProvider{name: "instance_principal"},
		started:                   started,
		release:                   release,
	}
	loader := &instancePrincipalProvider{create: func() (common.ConfigurationProvider, error) {
		calls.Add(1)
		return provider, nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	go func() {
		_, err := loader.get(ctx)
		returned <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial token validation")
	}
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation during token validation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider validation blocked the canceled caller")
	}

	deadline, cancelDeadline := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelDeadline()
	if _, err := loader.get(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected caller deadline during ongoing initialization, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one shared initialization, got %d", got)
	}

	// Release the shared attempt and verify subsequent callers reuse its result.
	release <- struct{}{}
	ready, cancelReady := context.WithTimeout(context.Background(), time.Second)
	defer cancelReady()
	for range 2 {
		if got, err := loader.get(ready); err != nil || got != provider {
			t.Fatalf("expected the initialized provider, got %T, %v", got, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected successful provider reuse, got %d factory calls", got)
	}
}

type gatedInstancePrincipal struct {
	fakeConfigurationProvider
	started chan<- struct{}
	release <-chan struct{}
}

func (p gatedInstancePrincipal) KeyID() (string, error) {
	p.started <- struct{}{}
	<-p.release
	return p.fakeConfigurationProvider.KeyID()
}

func TestInstancePrincipalRetriesAfterFailure(t *testing.T) {
	failure := errors.New("provider unavailable")
	var calls atomic.Int32
	loader := &instancePrincipalProvider{create: func() (common.ConfigurationProvider, error) {
		if calls.Add(1) == 1 {
			return nil, failure
		}
		return fakeConfigurationProvider{name: "instance_principal"}, nil
	}}
	if _, err := loader.get(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("expected factory failure, got %v", err)
	}
	if provider, err := loader.get(context.Background()); provider == nil || err != nil {
		t.Fatalf("expected a successful retry, got %T, %v", provider, err)
	}
}

func providerFactoryForTest(name string) func() (common.ConfigurationProvider, error) {
	return func() (common.ConfigurationProvider, error) {
		return fakeConfigurationProvider{name: name}, nil
	}
}
