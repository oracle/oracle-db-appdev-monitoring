// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build oracledb && !goora

package db

import (
	"database/sql/driver"
	"errors"
	"flag"
	"strconv"
	"strings"
	"sync"

	"github.com/oracle/go-oracledb/v26/oracle"
	oracleerrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

const driverName = "experimental go-oracledb"

func newConnector(dbconfig config.DatabaseConfig, credentials config.DatabaseCredentials, clientInfo config.ClientInfoConfig) (driver.Connector, error) {
	connector, err := newOracleDBConnector(dbconfig, credentials.Username, credentials.Password)
	if err != nil {
		return nil, err
	}
	return withClientInfo(connector, clientInfo), nil
}

var oracleDBFlagsMu sync.Mutex

// The exporter parses its CLI with a dedicated FlagSet. The driver otherwise
// lazily parses os.Args on the global FlagSet and exits on --config.file.
func prepareOracleDBFlags() {
	oracleDBFlagsMu.Lock()
	defer oracleDBFlagsMu.Unlock()
	if !flag.Parsed() {
		_ = flag.CommandLine.Parse(nil)
	}
}

func newOracleDBConnector(dbconfig config.DatabaseConfig, username, password string) (driver.Connector, error) {
	if dbconfig.ExternalAuth || username == "" || password == "" {
		return nil, errors.New("go-oracledb requires username/password authentication; external authentication is not supported")
	}
	if dbconfig.DiscoveryWallet != nil {
		return nil, errors.New("go-oracledb automatic mTLS wallets are not supported; use godror or go-ora for wallet-based OCI discovery")
	}
	if dbconfig.TNSAdmin != "" {
		return nil, errors.New("go-oracledb does not support tnsAdmin; use an Easy Connect string or full connect descriptor in database.url")
	}
	prepareOracleDBFlags()
	cfg := oracle.NewOracleDriverConfig()
	cfg.ConnectDescriptor = dbconfig.URL
	// Keep credentials out of the DSN so special characters are preserved.
	cfg.Credentials.User = username
	cfg.Credentials.Password = password
	cfg.Credentials.LogonMode = dbconfig.Role
	connector, err := oracle.NewOracleConnector(cfg)
	if err != nil {
		// Driver parsing errors can include the DSN. Do not expose connection secrets.
		return nil, errors.New("invalid go-oracledb connection configuration; check database.url and role")
	}
	return connector, nil
}

func oracleErrorCode(err error) int {
	var sqlErr oracleerrors.SQLError
	if !errors.As(err, &sqlErr) {
		return 0
	}
	code := sqlErr.ErrorCode()
	if !strings.HasPrefix(code, "ORA-") {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(code, "ORA-"))
	return n
}
