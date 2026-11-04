// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build oracledb && !goora

package oci

import (
	"errors"
	"os"
)

func walletGenerationPassword() (string, error) {
	// Use the same native-driver password for generation and PEM decryption.
	// Its supported password configuration is process-wide; do not mutate it.
	password := os.Getenv("oracle.go.wallet_password")
	if password == "" {
		password = os.Getenv("ORACLE_GO_WALLET_PASSWORD")
	}
	if password == "" {
		return "", errors.New("generating a wallet for go-oracledb requires ORACLE_GO_WALLET_PASSWORD (or oracle.go.wallet_password) to decrypt ewallet.pem")
	}
	return password, nil
}
