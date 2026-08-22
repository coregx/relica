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

// Negative: Row() with data does NOT return ErrNotFound.
func TestRow_WithData_NoError(t *testing.T) {
	db := openCovDB(t)
	seedCovTable(t, db)

	var name string
	err := db.NewQuery("SELECT name FROM cov_items WHERE id = 1").Row(&name)
	if err != nil {
		t.Fatalf("Row() with existing data should not error, got: %v", err)
	}
	if name != "alpha" {
		t.Errorf("expected 'alpha', got %q", name)
	}
}

// Negative: WithContext(nil) must not panic.
func TestModelQuery_WithContext_Nil_NoPanic(t *testing.T) {
	db := mockDBFull("postgres")

	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WithContext(nil) panicked: %v", r)
		}
	}()

	mq := db.Model(&User{Name: "Test"})
	mq2 := mq.WithContext(nil)
	if mq2 == nil {
		t.Error("WithContext(nil) should return non-nil ModelQuery")
	}
}

// Negative: ModelQuery.WithContext does not share exclude map.
func TestModelQuery_WithContext_ExcludeIndependent(t *testing.T) {
	db := mockDBFull("postgres")

	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	mq := db.Model(&User{Name: "Test"})
	mq.exclude["name"] = true

	ctx := context.Background()
	mq2 := mq.WithContext(ctx)

	// Modify original's exclude — should not affect mq2
	mq.exclude["extra"] = true

	if _, ok := mq2.exclude["extra"]; ok {
		t.Error("WithContext copy shares exclude map with original — mutation leaked")
	}
	if _, ok := mq2.exclude["name"]; !ok {
		t.Error("WithContext copy should have inherited 'name' exclusion")
	}
}
