package relica_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coregx/relica"
)

// insertViaExecutor is the shape of repository code the Executor interface
// exists for: it runs the same query whether handed a *DB or a *Tx.
func insertViaExecutor(ctx context.Context, ex relica.Executor) error {
	_, err := ex.ExecContext(ctx, "INSERT INTO users (id) VALUES (?)", 1)
	return err
}

func TestExecutor_DBSatisfies(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var ex relica.Executor = db
	if _, ok := ex.(*relica.DB); !ok {
		t.Fatalf("expected *relica.DB behind Executor, got %T", ex)
	}
	if err := insertViaExecutor(context.Background(), ex); err != nil {
		t.Fatalf("insert via *DB executor: %v", err)
	}
}

func TestExecutor_TxSatisfies(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var seen relica.Executor
	err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
		seen = tx
		return insertViaExecutor(context.Background(), tx)
	})
	if err != nil {
		t.Fatalf("insert via *Tx executor: %v", err)
	}
	if _, ok := seen.(*relica.Tx); !ok {
		t.Fatalf("expected *relica.Tx behind Executor, got %T", seen)
	}
}

// txKey mirrors the private context key a dbcontext package would use.
type txKey struct{}

// withExecutor is the go-rest-api / ozzo-dbx dbcontext pattern expressed with
// relica.Executor: return the transaction stored in ctx, else the connection.
func withExecutor(ctx context.Context, db *relica.DB) relica.Executor {
	if tx, ok := ctx.Value(txKey{}).(*relica.Tx); ok {
		return tx
	}
	return db.WithContext(ctx)
}

func TestExecutor_DBContextPattern(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// Outside a transaction With returns the connection.
	if _, ok := withExecutor(ctx, db).(*relica.DB); !ok {
		t.Fatalf("outside tx: expected *relica.DB")
	}

	// Inside Transactional the stored tx is returned, and repository code
	// that only sees an Executor runs against it.
	err := db.Transactional(ctx, func(tx *relica.Tx) error {
		txCtx := context.WithValue(ctx, txKey{}, tx)
		got := withExecutor(txCtx, db)
		if got != relica.Executor(tx) {
			t.Fatalf("inside tx: expected the stored *relica.Tx, got %T", got)
		}
		return insertViaExecutor(txCtx, got)
	})
	if err != nil {
		t.Fatalf("transactional via executor: %v", err)
	}
}

func TestExecutor_RollbackPropagates(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	want := errors.New("abort")

	err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
		var ex relica.Executor = tx
		if err := insertViaExecutor(context.Background(), ex); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v from Transactional, got %v", want, err)
	}
}
