// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora && !oracledb

package db

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	go_ora "github.com/sijms/go-ora/v3"
	"github.com/sijms/go-ora/v3/network"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

const driverName = "go-ora"

func newConnector(dbconfig config.DatabaseConfig, credentials config.DatabaseCredentials, clientInfo config.ClientInfoConfig) (driver.Connector, error) {
	return withClientInfo(go_ora.NewConnector(gooraDSN(dbconfig, credentials.Username, credentials.Password)), clientInfo), nil
}

var gooraTCPSProtocol = regexp.MustCompile(`(?i)\(\s*protocol\s*=\s*tcps\s*\)`)

// gooraDSN passes long OCI descriptors via connStr instead of interpolating
// them into a URL authority. Query encoding preserves certificate DNs containing
// commas and credentials containing URL delimiters.
func gooraDSN(dbconfig config.DatabaseConfig, username, password string) string {
	if strings.HasPrefix(strings.TrimSpace(dbconfig.URL), "(") {
		u := url.URL{Scheme: "oracle", Host: ":0", User: url.UserPassword(username, password), Path: "/"}
		q := url.Values{"connStr": []string{dbconfig.URL}}
		if gooraTCPSProtocol.MatchString(dbconfig.URL) {
			q.Set("SSL", "true")
			q.Set("SSL VERIFY", "true")
		}
		if dbconfig.TNSAdmin != "" {
			q.Set("WALLET", dbconfig.TNSAdmin)
		}
		if dbconfig.Role != "" {
			q.Set("DBA PRIVILEGE", dbconfig.Role)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	var dsn string
	if password == "" {
		dsn = fmt.Sprintf("oracle://@%s", dbconfig.URL)
	} else if username != "" {
		dsn = fmt.Sprintf("oracle://%s@%s", url.UserPassword(username, password).String(), dbconfig.URL)
	} else {
		dsn = fmt.Sprintf("oracle://%s", dbconfig.URL)
	}
	// Existing Easy Connect URLs retain their query options.
	if dbconfig.TNSAdmin != "" {
		if u, err := url.Parse(dsn); err == nil {
			q := u.Query()
			q.Set("WALLET", dbconfig.TNSAdmin)
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	return dsn
}

func oracleErrorCode(err error) int {
	var oraErr *network.OracleError
	if !errors.As(err, &oraErr) {
		return 0
	}
	return oraErr.ErrCode
}
