package core

import (
	"context"
	"errors"
	"testing"
)

type bugfixCtxKey string

// Bug 1: Tx.Model() must propagate transaction context.
func TestTxModel_PropagatesContext(t *testing.T) {
	db := openCovDB(t)

	ctx := context.WithValue(context.Background(), bugfixCtxKey("test"), "txctx")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer tx.Rollback()

	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	mq := tx.Model(&User{Name: "Alice"})
	if mq.ctx == nil {
		t.Fatal("Tx.Model() must propagate transaction context, got nil")
	}
	if mq.ctx.Value(bugfixCtxKey("test")) != "txctx" {
		t.Error("Tx.Model() context does not carry transaction context values")
	}
}

// Bug 2: ModelQuery.WithContext() must be immutable.
func TestModelQuery_WithContext_Immutable(t *testing.T) {
	db := mockDBFull("postgres")

	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	mq1 := db.Model(&User{Name: "Alice"})
	ctx1 := context.WithValue(context.Background(), bugfixCtxKey("k"), "v1")
	ctx2 := context.WithValue(context.Background(), bugfixCtxKey("k"), "v2")

	mq2 := mq1.WithContext(ctx1)
	mq3 := mq1.WithContext(ctx2)

	if mq2 == mq1 {
		t.Error("WithContext must return new ModelQuery, not same pointer")
	}
	if mq3 == mq1 {
		t.Error("WithContext must return new ModelQuery, not same pointer")
	}
	if mq2.ctx == mq3.ctx {
		t.Error("two WithContext calls must produce independent instances")
	}
	if mq2.ctx != ctx1 {
		t.Error("mq2 should have ctx1")
	}
	if mq3.ctx != ctx2 {
		t.Error("mq3 should have ctx2")
	}
}

// Bug 3: Row() must return ErrNotFound on no rows.
func TestRow_ReturnsErrNotFound(t *testing.T) {
	db := openCovDB(t)
	seedCovTable(t, db)

	var name string
	err := db.NewQuery("SELECT name FROM cov_items WHERE id = 999").Row(&name)
	if err == nil {
		t.Fatal("expected error for no rows")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Row() should return ErrNotFound, got: %v", err)
	}
}
