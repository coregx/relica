package relica_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/coregx/relica"
)

func TestWrapTx_RunsQueriesOnExternalTransaction(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	sqlTx, err := db.SqlDB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	tx := db.WrapTx(ctx, sqlTx)

	// The wrapped transaction is a full relica.Tx and therefore an Executor.
	var ex relica.Executor = tx
	if err := insertViaExecutor(ctx, ex); err != nil {
		t.Fatalf("insert via wrapped tx: %v", err)
	}

	// Committing through the ORIGINAL *sql.Tx ends the same transaction:
	// Relica's wrapper must observe it as done.
	if err := sqlTx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	err = insertViaExecutor(ctx, ex)
	if !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("expected sql.ErrTxDone after external commit, got %v", err)
	}
}

func TestWrapTx_CommitThroughWrapper(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	sqlTx, err := db.SqlDB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	tx := db.WrapTx(ctx, sqlTx)
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit via wrapper: %v", err)
	}
	// Original handle is finished too — same transaction, not a copy.
	if err := sqlTx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("expected original *sql.Tx to be done, got %v", err)
	}
}

func TestWrapTx_AttachesContext(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	sqlTx, err := db.SqlDB().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = sqlTx.Rollback() }()

	// Queries built from the wrapper run under the ctx passed to WrapTx.
	tx := db.WrapTx(canceledCtx(), sqlTx)
	_, err = tx.Insert("users", map[string]any{"name": "a"}).Execute()
	requireCanceled(t, err)
}

func TestWrapTx_NilContextFallsBackToDBContext(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	sqlTx, err := db.SqlDB().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = sqlTx.Rollback() }()

	var noCtx context.Context // nil: the documented fallback to the DB context
	tx := db.WithContext(canceledCtx()).WrapTx(noCtx, sqlTx)
	_, err = tx.Insert("users", map[string]any{"name": "a"}).Execute()
	requireCanceled(t, err)
}

func TestWrapTx_NilTxPanics(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil *sql.Tx")
		}
	}()
	_ = db.WrapTx(context.Background(), nil)
}
