package relica_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coregx/relica"
)

// These tests pin the contract that DB.WithContext actually reaches query
// execution. Before v0.18.0 Builder() and NewQuery() dropped the stored
// context, so every query built from db.WithContext(ctx) ran under
// context.Background() and cancellation was silently ignored.
//
// database/sql checks ctx.Done() before acquiring a connection, so an already
// canceled context fails deterministically with context.Canceled regardless
// of driver — the stub driver never gets involved.

func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func requireCanceled(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected context.Canceled, got nil — context was not propagated")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if errors.Is(err, relica.ErrNotFound) {
		t.Fatalf("cancellation must not be reported as ErrNotFound: %v", err)
	}
}

func TestDBWithContext_PropagatesToEveryQueryType(t *testing.T) {
	cases := []struct {
		name string
		run  func(db *relica.DB) error
	}{
		{"Select.One", func(db *relica.DB) error {
			var u testUser
			return db.Select("id").From("users").One(&u)
		}},
		{"Select.All", func(db *relica.DB) error {
			var us []testUser
			return db.Select("id").From("users").All(&us)
		}},
		{"Select.Count", func(db *relica.DB) error {
			_, err := db.Select().From("users").Count()
			return err
		}},
		{"Insert", func(db *relica.DB) error {
			_, err := db.Insert("users", map[string]any{"name": "a"}).Execute()
			return err
		}},
		{"InsertStruct", func(db *relica.DB) error {
			_, err := db.InsertStruct("users", &testUser{Name: "a"}).Execute()
			return err
		}},
		{"Update", func(db *relica.DB) error {
			_, err := db.Update("users").Set(map[string]any{"name": "b"}).Where(relica.Eq("id", 1)).Execute()
			return err
		}},
		{"Delete", func(db *relica.DB) error {
			_, err := db.Delete("users").Where(relica.Eq("id", 1)).Execute()
			return err
		}},
		{"Upsert", func(db *relica.DB) error {
			_, err := db.Upsert("users", map[string]any{"id": 1, "name": "a"}).
				OnConflict("id").DoUpdate("name").Execute()
			return err
		}},
		{"BatchInsert", func(db *relica.DB) error {
			_, err := db.BatchInsert("users", []string{"name"}).Values("a").Values("b").Execute()
			return err
		}},
		{"BatchUpdate", func(db *relica.DB) error {
			_, err := db.BatchUpdate("users", "id").Set(1, map[string]any{"name": "a"}).Execute()
			return err
		}},
		{"NewQuery.Execute", func(db *relica.DB) error {
			_, err := db.NewQuery("DELETE FROM users").Execute()
			return err
		}},
		{"NewQuery.Row", func(db *relica.DB) error {
			var n int
			return db.NewQuery("SELECT COUNT(*) FROM users").Row(&n)
		}},
		{"Model.Insert", func(db *relica.DB) error {
			return db.Model(&testUser{Name: "a"}).Insert()
		}},
		{"Model.Find", func(db *relica.DB) error {
			var u testUser
			return db.Model(&u).Find(1)
		}},
		{"Select.Model", func(db *relica.DB) error {
			u := testUser{ID: 1}
			return db.Select().Model(&u)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			defer db.Close()
			requireCanceled(t, tc.run(db.WithContext(canceledCtx())))
		})
	}
}

func TestQueryWithContext_AppliesToInsertAndNewQuery(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, err := db.Insert("users", map[string]any{"name": "a"}).WithContext(canceledCtx()).Execute()
	requireCanceled(t, err)

	_, err = db.NewQuery("DELETE FROM users").WithContext(canceledCtx()).Execute()
	requireCanceled(t, err)
}

func TestQueryWithContext_OverridesDBContext(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// DB carries a canceled context, the query replaces it with a live one:
	// the query must run.
	_, err := db.WithContext(canceledCtx()).
		Insert("users", map[string]any{"name": "a"}).
		WithContext(context.Background()).
		Execute()
	if err != nil {
		t.Fatalf("per-query context must override DB context, got %v", err)
	}

	// Same priority for builder types that already had WithContext.
	var us []testUser
	err = db.WithContext(canceledCtx()).
		Select("id").From("users").
		WithContext(context.Background()).
		All(&us)
	if err != nil {
		t.Fatalf("SelectQuery.WithContext must override DB context, got %v", err)
	}
}

func TestDBWithContext_DoesNotLeakIntoTransactions(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// A transaction is bound to the context passed to Begin, not to the
	// context stored on the DB it was started from.
	tx, err := db.WithContext(canceledCtx()).Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin with a live context must succeed, got %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Insert("users", map[string]any{"name": "a"}).Execute(); err != nil {
		t.Fatalf("tx query must use Begin's context, got %v", err)
	}
}

func TestDBWithContext_DoesNotMutateOriginal(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_ = db.WithContext(canceledCtx()) // derived DB discarded

	// The original DB keeps running with its own (background) context.
	if _, err := db.Insert("users", map[string]any{"name": "a"}).Execute(); err != nil {
		t.Fatalf("WithContext must not mutate the receiver, got %v", err)
	}
}
