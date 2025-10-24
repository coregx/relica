package cache

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"sync/atomic"
)

// Mock driver for tests and benchmarks.
type mockDriver struct{}

type mockConn struct {
	closed bool
}

type mockStmt struct {
	closed bool
	query  string
}

func (d *mockDriver) Open(_ string) (driver.Conn, error) {
	return &mockConn{}, nil
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{query: query}, nil
}

func (c *mockConn) Close() error {
	c.closed = true
	return nil
}

func (c *mockConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (s *mockStmt) Close() error {
	s.closed = true
	return nil
}

func (s *mockStmt) NumInput() int {
	return 0
}

func (s *mockStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return nil, driver.ErrSkip
}

func (s *mockStmt) Query(_ []driver.Value) (driver.Rows, error) {
	return nil, driver.ErrSkip
}

var (
	driverCounter  atomic.Uint64
	driverRegistry sync.Map
)

// registerMockDriver registers a unique mock driver and returns a DB connection.
func registerMockDriver() (*sql.DB, error) {
	id := driverCounter.Add(1)
	driverName := fmt.Sprintf("mock-driver-%d", id)

	// Check if already registered.
	if _, loaded := driverRegistry.LoadOrStore(driverName, true); !loaded {
		sql.Register(driverName, &mockDriver{})
	}

	return sql.Open(driverName, "")
}
