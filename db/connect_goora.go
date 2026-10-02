// Copyright (c) 2025, 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/sijms/go-ora/v2/network"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

func connect(logger *slog.Logger, dbname string, dbconfig config.DatabaseConfig, clientInfo config.ClientInfoConfig) (*sql.DB, error) {
	logger.Debug("Launching connection to "+MaskDSN(dbconfig.URL), "database", dbname)

	password, err := dbconfig.GetPassword()
	if err != nil {
		return nil, err
	}
	username, err := dbconfig.GetUsername()
	if err != nil {
		return nil, err
	}
	dbconfig.ExternalAuth = password == ""

	logger.Debug(fmt.Sprintf("external authentication set to %t", dbconfig.ExternalAuth), "database", dbname)

	msg := "Using Username/Password Authentication."
	if dbconfig.ExternalAuth {
		msg = "Database Password not specified; will attempt to use external authentication (ignoring user input)."
		dbconfig.Username = ""
	}
	logger.Info(msg, "database", dbname)

	// Build connection string for go-ora
	var dsn string
	if dbconfig.ExternalAuth {
		// go-ora doesn't directly support external authentication
		// So we rely on OS authentication (set Oracle wallet/env)
		dsn = fmt.Sprintf("oracle://@%s", dbconfig.URL)
	} else if username != "" {
		// url.UserPassword properly percent-encodes special characters in credentials,
		// preventing malformed DSNs when passwords contain @, ?, #, /, % etc.
		userInfo := url.UserPassword(username, password)
		dsn = fmt.Sprintf("oracle://%s@%s", userInfo.String(), dbconfig.URL)
	} else {
		dsn = fmt.Sprintf("oracle://%s", dbconfig.URL)
	}

	// open connection (lazy until first use)
	db := sql.OpenDB(withClientInfo(go_ora.NewConnector(dsn), clientInfo))

	return db, nil
}

func withClientInfo(connector driver.Connector, clientInfo config.ClientInfoConfig) driver.Connector {
	if !clientInfo.IsEnabled() {
		return connector
	}
	return initializingConnector{Connector: connector, label: clientInfo.GetLabel()}
}

// initializingConnector marks every new session before database/sql can use it.
type initializingConnector struct {
	driver.Connector
	label string
}

func (c initializingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	exec, ok := conn.(driver.ExecerContext)
	if !ok {
		conn.Close()
		return nil, errors.New("driver does not support session initialization")
	}
	if _, err := exec.ExecContext(ctx, clientInfoSQL(c.label), nil); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initialize exporter session: %w", err)
	}
	return conn, nil
}

func effectiveSQLPoolLimits(dbconfig config.DatabaseConfig) (int, int) {
	maxOpenConns := dbconfig.GetMaxOpenConns()
	if dbconfig.GetPoolMaxConnections() > 0 {
		maxOpenConns = dbconfig.GetPoolMaxConnections()
	}

	maxIdleConns := dbconfig.GetMaxIdleConns()
	if dbconfig.GetPoolMinConnections() > 0 {
		maxIdleConns = dbconfig.GetPoolMinConnections()
	}
	return maxOpenConns, maxIdleConns
}

func warmupConnectionPoolSize(dbconfig config.DatabaseConfig) int {
	maxOpenConns, _ := effectiveSQLPoolLimits(dbconfig)
	return maxOpenConns
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
