// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora && !oracledb

package db

import "github.com/sijms/go-ora/v3/network"

func credentialRefreshAuthError() error { return network.NewOracleError(ora01017code) }
