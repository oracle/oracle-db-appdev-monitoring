// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"crypto/rsa"
	"errors"
	"testing"

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
	originalInstance := instancePrincipalConfigurationProvider
	originalResource := resourcePrincipalConfigurationProvider
	originalWorkload := workloadIdentityConfigurationProvider
	t.Cleanup(func() {
		defaultConfigProvider = originalDefault
		instancePrincipalConfigurationProvider = originalInstance
		resourcePrincipalConfigurationProvider = originalResource
		workloadIdentityConfigurationProvider = originalWorkload
	})

	defaultConfigProvider = providerFactoryForTest("config_file")
	instancePrincipalConfigurationProvider = providerFactoryForTest("instance_principal")
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
	original := instancePrincipalConfigurationProvider
	t.Cleanup(func() { instancePrincipalConfigurationProvider = original })
	instancePrincipalConfigurationProvider = func() (common.ConfigurationProvider, error) {
		return nil, errors.New("provider unavailable")
	}

	_, err := ConfigurationProviderForAuthMode("instance_principal")
	if err == nil {
		t.Fatal("expected provider factory error")
	}
	if err.Error() != "provider unavailable" {
		t.Fatalf("expected provider factory error to be returned, got %v", err)
	}
}

func providerFactoryForTest(name string) func() (common.ConfigurationProvider, error) {
	return func() (common.ConfigurationProvider, error) {
		return fakeConfigurationProvider{name: name}, nil
	}
}
