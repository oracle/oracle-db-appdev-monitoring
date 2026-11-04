// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build oracledb && !goora

package oci

import (
	"context"
	"io"
	"strings"
	"testing"

	adb "github.com/oracle/oci-go-sdk/v65/database"
)

func TestOracleDBWalletGenerationUsesNativePassword(t *testing.T) {
	t.Setenv("ORACLE_GO_WALLET_PASSWORD", "upper-case-password")
	t.Setenv("oracle.go.wallet_password", "preferred-password")
	client := walletADBFunc(func(_ context.Context, req adb.GenerateAutonomousDatabaseWalletRequest) (adb.GenerateAutonomousDatabaseWalletResponse, error) {
		if req.Password == nil || *req.Password != "preferred-password" {
			t.Fatal("generation password differs from native driver precedence")
		}
		return adb.GenerateAutonomousDatabaseWalletResponse{Content: io.NopCloser(strings.NewReader("zip"))}, nil
	})
	if _, err := walletArchiveFromADB(context.Background(), client, "database"); err != nil {
		t.Fatal(err)
	}
}

func TestOracleDBWalletGenerationRequiresPasswordBeforeAPICall(t *testing.T) {
	t.Setenv("ORACLE_GO_WALLET_PASSWORD", "")
	t.Setenv("oracle.go.wallet_password", "")
	client := walletADBFunc(func(context.Context, adb.GenerateAutonomousDatabaseWalletRequest) (adb.GenerateAutonomousDatabaseWalletResponse, error) {
		t.Fatal("wallet generation called without a usable PEM password")
		return adb.GenerateAutonomousDatabaseWalletResponse{}, nil
	})
	if _, err := walletArchiveFromADB(context.Background(), client, "database"); err == nil || !strings.Contains(err.Error(), "ORACLE_GO_WALLET_PASSWORD") {
		t.Fatalf("expected required password error, got %v", err)
	}
}
