// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora && oracledb

package db

// Select exactly one experimental database driver.
var _ = select_only_one_of_goora_and_oracledb_build_tags
