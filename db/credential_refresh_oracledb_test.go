// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build oracledb && !goora

package db

func credentialRefreshAuthError() error { return oracleDBTestError("ORA-01017") }
