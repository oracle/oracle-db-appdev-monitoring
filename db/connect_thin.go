// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

//go:build goora || oracledb

package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
)

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
