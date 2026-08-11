// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

// Package oci contains shared OCI integration helpers.
package oci

import (
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

// AuthMode identifies the OCI SDK authentication provider to use.
type AuthMode string

const (
	AuthModeConfigFile        AuthMode = "config_file"
	AuthModeInstancePrincipal AuthMode = "instance_principal"
	AuthModeResourcePrincipal AuthMode = "resource_principal"
	AuthModeWorkloadIdentity  AuthMode = "workload_identity"
)

var (
	defaultConfigProvider = func() (common.ConfigurationProvider, error) {
		return common.DefaultConfigProvider(), nil
	}
	instancePrincipalConfigurationProvider = func() (common.ConfigurationProvider, error) {
		return auth.InstancePrincipalConfigurationProvider()
	}
	resourcePrincipalConfigurationProvider = func() (common.ConfigurationProvider, error) {
		return auth.ResourcePrincipalConfigurationProvider()
	}
	workloadIdentityConfigurationProvider = func() (common.ConfigurationProvider, error) {
		return auth.OkeWorkloadIdentityConfigurationProvider()
	}
)

// AcceptedAuthModes returns the supported OCI authentication mode values.
func AcceptedAuthModes() []string {
	return []string{
		string(AuthModeConfigFile),
		string(AuthModeInstancePrincipal),
		string(AuthModeResourcePrincipal),
		string(AuthModeWorkloadIdentity),
	}
}

// ValidateAuthMode verifies that authMode is supported. An empty mode defaults
// to config_file.
func ValidateAuthMode(authMode AuthMode) error {
	switch normalizeAuthMode(authMode) {
	case AuthModeConfigFile, AuthModeInstancePrincipal, AuthModeResourcePrincipal, AuthModeWorkloadIdentity:
		return nil
	default:
		return fmt.Errorf("unsupported OCI auth mode %q; accepted values are: %s", authMode, strings.Join(AcceptedAuthModes(), ", "))
	}
}

// ConfigurationProviderForAuthMode returns the OCI SDK configuration provider
// selected by authMode. Unsupported runtime values use the config-file
// provider; callers that accept configuration input should call
// ValidateAuthMode first.
func ConfigurationProviderForAuthMode(authMode AuthMode) (common.ConfigurationProvider, error) {
	switch normalizeAuthMode(authMode) {
	case AuthModeConfigFile:
		return defaultConfigProvider()
	case AuthModeInstancePrincipal:
		return instancePrincipalConfigurationProvider()
	case AuthModeResourcePrincipal:
		return resourcePrincipalConfigurationProvider()
	case AuthModeWorkloadIdentity:
		return workloadIdentityConfigurationProvider()
	default:
		return defaultConfigProvider()
	}
}

func normalizeAuthMode(authMode AuthMode) AuthMode {
	mode := strings.ToLower(strings.TrimSpace(string(authMode)))
	if mode == "" {
		return AuthModeConfigFile
	}
	return AuthMode(mode)
}
