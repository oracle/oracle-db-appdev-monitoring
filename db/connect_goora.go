// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora && !oracledb

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	go_ora "github.com/sijms/go-ora/v3"
	"github.com/sijms/go-ora/v3/network"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func connect(logger *slog.Logger, dbname string, dbconfig config.DatabaseConfig, clientInfo config.ClientInfoConfig) (*sql.DB, error) {
	return connectContext(context.Background(), logger, dbname, dbconfig, clientInfo)
}

func connectContext(ctx context.Context, logger *slog.Logger, dbname string, dbconfig config.DatabaseConfig, clientInfo config.ClientInfoConfig) (*sql.DB, error) {
	logger.Debug("Launching connection to "+MaskDSN(dbconfig.URL), "database", dbname)

	credentials, err := dbconfig.ResolveCredentialsContext(ctx)
	if err != nil {
		return nil, err
	}
	username, password := credentials.Username, credentials.Password
	dbconfig.ExternalAuth = password == ""

	logger.Debug(fmt.Sprintf("external authentication set to %t", dbconfig.ExternalAuth), "database", dbname)

	msg := "Using Username/Password Authentication."
	if dbconfig.ExternalAuth {
		msg = "Database Password not specified; will attempt to use external authentication (ignoring user input)."
		dbconfig.Username = ""
	}
	logger.Info(msg, "database", dbname)

	dsn := gooraDSN(dbconfig, username, password)

	// open connection (lazy until first use)
	db := sql.OpenDB(withClientInfo(go_ora.NewConnector(dsn), clientInfo))

	return db, nil
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

func isInvalidCredentialsError(err error) bool {
	if err == nil {
		return false
	}
	var oraErr *network.OracleError
	ok := errors.As(err, &oraErr)
	if !ok {
		return false
	}
	return oraErr.ErrCode == ora01017code || oraErr.ErrCode == ora28000code
}

func isTemporaryConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var oraErr *network.OracleError
	ok := errors.As(err, &oraErr)
	if !ok {
		return false
	}
	return isTemporaryConnectionErrorCode(oraErr.ErrCode)
}
