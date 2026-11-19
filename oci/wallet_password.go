// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build !oracledb || goora

package oci

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func walletGenerationPassword() (string, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return "", fmt.Errorf("generate wallet encryption password: %w", err)
	}
	// These drivers use the auto-login SSO wallet; the PKCS12 password is not retained.
	return "aA1!" + base64.RawURLEncoding.EncodeToString(entropy), nil
}
