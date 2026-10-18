package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

// failTxDriver begins transactions whose Rollback always fails and whose Commit
// fails when the DSN is "commit-fails", to check that Transactional reports
// both instead of dropping them.
type failTxDriver struct{}

type failTxConn struct{ commitFails bool }

type failTx struct{ commitFails bool }

var (
	errFailRollback = errors.New("rollback failed: connection lost")
	errFailCommit   = errors.New("commit failed: serialization failure")
	errWork         = errors.New("work failed")
)

func init() { sql.Register("relica_fail_tx", failTxDriver{}) }

func (failTxDriver) Open(dsn string) (driver.Conn, error) {
	return failTxConn{commitFails: dsn == "commit-fails"}, nil
}

func (failTxConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (failTxConn) Close() error                        { return nil }
func (c failTxConn) Begin() (driver.Tx, error)         { return failTx(c), nil }

func (t failTx) Commit() error {
	if t.commitFails {
		return errFailCommit
	}
	return nil
}
func (failTx) Rollback() error { return errFailRollback }

func openFailRollbackDB(t *testing.T) *DB { return openFailTxDB(t, "") }

func openFailTxDB(t *testing.T, dsn string) *DB {
	t.Helper()
	sqlDB, err := sql.Open("relica_fail_tx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return WrapDB(sqlDB, "sqlite")
}

// A failed rollback means the transaction's fate is unknown to the caller; it
// must be reported together with the error that caused the rollback, and both
// must stay matchable with errors.Is.
func TestTransactional_ReportsRollbackError(t *testing.T) {
	db := openFailRollbackDB(t)
	run := map[string]func(f func(*Tx) error) error{
		"Transactional": func(f func(*Tx) error) error { return db.Transactional(context.Background(), f) },
		"TransactionalTx": func(f func(*Tx) error) error {
			return db.TransactionalTx(context.Background(), &TxOptions{}, f)
		},
	}
	for name, transactional := range run {
		t.Run(name, func(t *testing.T) {
			err := transactional(func(*Tx) error { return errWork })
			if !errors.Is(err, errWork) {
				t.Errorf("err = %v, want it to match the work error", err)
			}
			if !errors.Is(err, errFailRollback) {
				t.Errorf("err = %v, want it to match the rollback error", err)
			}
		})
	}
}

// If the function already ended the transaction, the deferred rollback gets
// sql.ErrTxDone; that is not a failure and must not be added to the error.
func TestTransactional_IgnoresErrTxDoneOnRollback(t *testing.T) {
	db := openCovDB(t)
	err := db.Transactional(context.Background(), func(tx *Tx) error {
		_ = tx.Rollback()
		return errWork
	})
	if !errors.Is(err, errWork) {
		t.Errorf("err = %v, want the work error", err)
	}
	if errors.Is(err, sql.ErrTxDone) {
		t.Errorf("err = %v, must not report sql.ErrTxDone from the deferred rollback", err)
	}
}

// Success path unchanged: the work error is nil, Commit runs, nil is returned.
func TestTransactional_CommitsOnSuccess(t *testing.T) {
	db := openFailRollbackDB(t)
	if err := db.Transactional(context.Background(), func(*Tx) error { return nil }); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}
