// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package db

import "strings"

// Quote configured labels as Oracle string literals for godror's OnInitStmts.
func clientInfoSQL(label string) string {
	return "BEGIN DBMS_APPLICATION_INFO.SET_CLIENT_INFO('" + strings.ReplaceAll(label, "'", "''") + "'); END;"
}
