// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package testdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
)

var driverID atomic.Uint64

type QueryResult struct {
	Columns   []string
	Rows      [][]driver.Value
	Err       error
	NextErrAt int
	NextErr   error
}

type Scenario struct {
	ConnectErrors []error
	PingErr       error
	ExecFunc      func(ctx context.Context, query string, args []driver.NamedValue) error
	QueryFunc     func(ctx context.Context, query string, args []driver.NamedValue) QueryResult
}

type State struct {
	mu           sync.Mutex
	connectCalls int
	pingCalls    int
	execCalls    []string
	queryCalls   []string
}

func (s *State) ConnectCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connectCalls
}

func (s *State) PingCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingCalls
}

func (s *State) ExecCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.execCalls...)
}

func (s *State) QueryCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.queryCalls...)
}

func New(scenario Scenario) (*sql.DB, *State) {
	state := &State{}
	name := fmt.Sprintf("testdb_%d", driverID.Add(1))
	sql.Register(name, testDriver{
		scenario: scenario,
		state:    state,
	})

	db, err := sql.Open(name, "")
	if err != nil {
		panic(err)
	}
	return db, state
}

type testDriver struct {
	scenario Scenario
	state    *State
}

func (d testDriver) Open(string) (driver.Conn, error) {
	d.state.mu.Lock()
	d.state.connectCalls++
	call := d.state.connectCalls
	var err error
	if idx := call - 1; idx < len(d.scenario.ConnectErrors) {
		err = d.scenario.ConnectErrors[idx]
	}
	d.state.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return &testConn{
		scenario: d.scenario,
		state:    d.state,
	}, nil
}

type testConn struct {
	scenario Scenario
	state    *State
}

func (c *testConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not implemented")
}

func (c *testConn) Close() error {
	return nil
}

func (c *testConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions not implemented")
}

func (c *testConn) Ping(context.Context) error {
	c.state.mu.Lock()
	c.state.pingCalls++
	c.state.mu.Unlock()
	return c.scenario.PingErr
}

func (c *testConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	c.state.execCalls = append(c.state.execCalls, normalizeWhitespace(query))
	c.state.mu.Unlock()

	if c.scenario.ExecFunc != nil {
		if err := c.scenario.ExecFunc(ctx, query, args); err != nil {
			return nil, err
		}
	}

	return driver.RowsAffected(0), nil
}

func (c *testConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	c.state.queryCalls = append(c.state.queryCalls, normalizeWhitespace(query))
	c.state.mu.Unlock()

	result := defaultQueryResult(query)
	if c.scenario.QueryFunc != nil {
		result = c.scenario.QueryFunc(ctx, query, args)
	}
	if result.Err != nil {
		return nil, result.Err
	}
	return &testRows{result: result}, nil
}

func normalizeWhitespace(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

func defaultQueryResult(query string) QueryResult {
	if strings.Contains(query, "sys_context('USERENV', 'ISDBA')") {
		return QueryResult{
			Columns: []string{"sysdba"},
			Rows:    [][]driver.Value{{"FALSE"}},
		}
	}
	return QueryResult{}
}

type testRows struct {
	result QueryResult
	index  int
}

func (r *testRows) Columns() []string {
	return r.result.Columns
}

func (r *testRows) Close() error {
	return nil
}

func (r *testRows) Next(dest []driver.Value) error {
	if r.result.NextErr != nil && r.index == r.result.NextErrAt {
		return r.result.NextErr
	}
	if r.index >= len(r.result.Rows) {
		return io.EOF
	}

	row := r.result.Rows[r.index]
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
			continue
		}
		dest[i] = nil
	}

	r.index++
	return nil
}
