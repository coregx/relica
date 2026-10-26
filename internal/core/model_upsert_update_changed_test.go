package core

import (
	"strings"
	"testing"
	"time"

	"github.com/coregx/relica/internal/dialects"
)

// ============================================================================
// Test models
// ============================================================================

// upsertUser is a test model for upsert SQL generation tests.
type upsertUser struct {
	ID     int    `db:"id"`
	Name   string `db:"name"`
	Email  string `db:"email"`
	Status string `db:"status"`
}

func (upsertUser) TableName() string { return "users" }

// upsertPost uses explicit pk tag.
type upsertPost struct {
	PostID  int    `db:"post_id,pk"`
	Content string `db:"content"`
	Views   int    `db:"views"`
}

func (upsertPost) TableName() string { return "posts" }

// diffUser for UpdateChanged tests.
type diffUser struct {
	ID     int    `db:"id"`
	Name   string `db:"name"`
	Email  string `db:"email"`
	Status string `db:"status"`
}

// diffUserWithTime includes a time field for deep equality testing.
type diffUserWithTime struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

// noIDModelUpsert has no primary key for error path tests.
type noIDModelUpsert struct {
	Name string `db:"name"`
}

func (noIDModelUpsert) TableName() string { return "things" }

// ============================================================================
// Helpers
// ============================================================================

// upsertMockDB creates a minimal DB for SQL generation testing (dialect only).
func upsertMockDB(dialectName string) *DB {
	return &DB{
		dialect:    dialects.GetDialect(dialectName),
		driverName: dialectName,
	}
}

// newTestMQ creates a ModelQuery for use in unit tests without a real DB.
func newTestMQ(db *DB, model interface{}, table string) *ModelQuery {
	return &ModelQuery{
		db:      db,
		model:   model,
		table:   table,
		exclude: make(map[string]bool),
	}
}

// ============================================================================
// Model upsert SQL (buildUpsert; no real DB). Validation: upsert_action_test.go
// ============================================================================

func modelUpsertSQL(t *testing.T, dialect string, model any, fields []string) string {
	t.Helper()
	db := upsertMockDB(dialect)
	q, err := db.Model(model).buildUpsert(nil, fields)
	if err != nil {
		t.Fatalf("buildUpsert: %v", err)
	}
	return q.sql
}

func TestModelUpsert_SQL_AllFields(t *testing.T) {
	user := upsertUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	want := map[string]string{
		"postgres": `INSERT INTO "users" ("email", "id", "name", "status") VALUES ($1, $2, $3, $4) ON CONFLICT ("id") DO UPDATE SET "email" = EXCLUDED."email", "name" = EXCLUDED."name", "status" = EXCLUDED."status"`,
		"mysql":    "INSERT INTO `users` (`email`, `id`, `name`, `status`) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE `email` = VALUES(`email`), `name` = VALUES(`name`), `status` = VALUES(`status`)",
		"sqlite":   "INSERT INTO `users` (`email`, `id`, `name`, `status`) VALUES (?, ?, ?, ?) ON CONFLICT (`id`) DO UPDATE SET `email` = excluded.`email`, `name` = excluded.`name`, `status` = excluded.`status`",
	}
	for d, sql := range want {
		t.Run(d, func(t *testing.T) {
			if got := modelUpsertSQL(t, d, &user, nil); got != sql {
				t.Errorf("got  %s\nwant %s", got, sql)
			}
		})
	}
}

func TestModelUpsert_SQL_SelectiveFields(t *testing.T) {
	user := upsertUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	want := map[string]string{
		"postgres": `INSERT INTO "users" ("email", "id", "name", "status") VALUES ($1, $2, $3, $4) ON CONFLICT ("id") DO UPDATE SET "name" = EXCLUDED."name"`,
		"mysql":    "INSERT INTO `users` (`email`, `id`, `name`, `status`) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)",
		"sqlite":   "INSERT INTO `users` (`email`, `id`, `name`, `status`) VALUES (?, ?, ?, ?) ON CONFLICT (`id`) DO UPDATE SET `name` = excluded.`name`",
	}
	for d, sql := range want {
		t.Run(d, func(t *testing.T) {
			if got := modelUpsertSQL(t, d, &user, []string{"name"}); got != sql {
				t.Errorf("got  %s\nwant %s", got, sql)
			}
		})
	}
}

func TestModelUpsert_SQL_ExplicitPKTag(t *testing.T) {
	post := upsertPost{PostID: 10, Content: "Hello", Views: 5}
	want := `INSERT INTO "posts" ("content", "post_id", "views") VALUES ($1, $2, $3) ON CONFLICT ("post_id") DO UPDATE SET "content" = EXCLUDED."content", "views" = EXCLUDED."views"`
	if got := modelUpsertSQL(t, "postgres", &post, nil); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

type upsertOrderItem struct {
	OrderID   int `db:"order_id,pk"`
	ProductID int `db:"product_id,pk"`
	Qty       int `db:"qty"`
}

func (upsertOrderItem) TableName() string { return "order_items" }

func TestModelUpsert_SQL_CompositePK(t *testing.T) {
	item := upsertOrderItem{OrderID: 1, ProductID: 2, Qty: 5}
	want := `INSERT INTO "order_items" ("order_id", "product_id", "qty") VALUES ($1, $2, $3) ON CONFLICT ("order_id", "product_id") DO UPDATE SET "qty" = EXCLUDED."qty"`
	if got := modelUpsertSQL(t, "postgres", &item, nil); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestModelUpsert_SQL_ZeroPKLeftOutOfInsert(t *testing.T) {
	user := upsertUser{Name: "Alice", Email: "alice@example.com", Status: "active"}
	want := `INSERT INTO "users" ("email", "name", "status") VALUES ($1, $2, $3) ON CONFLICT ("id") DO UPDATE SET "email" = EXCLUDED."email", "name" = EXCLUDED."name", "status" = EXCLUDED."status"`
	if got := modelUpsertSQL(t, "postgres", &user, nil); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// ============================================================================
// Upsert error path tests
// ============================================================================

func TestModelUpsert_Error_EmptyTable(t *testing.T) {
	db := upsertMockDB("postgres")
	user := upsertUser{ID: 1}

	mq := newTestMQ(db, &user, "")
	err := mq.Upsert()

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "table name not specified") {
		t.Errorf("%q does not contain %q", err.Error(), "table name not specified")
	}
}

func TestModelUpsert_Error_NoPrimaryKey(t *testing.T) {
	db := upsertMockDB("postgres")
	thing := noIDModelUpsert{Name: "thing"}

	mq := newTestMQ(db, &thing, "things")
	err := mq.Upsert()

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "primary key not found") {
		t.Errorf("%q does not contain %q", err.Error(), "primary key not found")
	}
}

// ============================================================================
// diffFields (UpdateChanged internals) unit tests
// ============================================================================

func TestDiffFields_SomeFieldsChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original
	current.Name = "Alice Updated"
	current.Status = "inactive"

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(changed))
	}
	if changed["name"] != "Alice Updated" {
		t.Errorf("got %v, want %v", changed["name"], "Alice Updated")
	}
	if changed["status"] != "inactive" {
		t.Errorf("got %v, want %v", changed["status"], "inactive")
	}
	if _, ok := changed["id"]; ok {
		t.Errorf("%q should not contain %q", changed, "id")
	}
	if _, ok := changed["email"]; ok {
		t.Errorf("%q should not contain %q", changed, "email")
	}
}

func TestDiffFields_NoFieldsChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original // exact copy

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("expected empty, got %d", len(changed))
	}
}

func TestDiffFields_AllNonPKFieldsChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := diffUser{ID: 1, Name: "Bob", Email: "bob@example.com", Status: "inactive"}

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 3 { // name, email, status
		t.Errorf("expected length %d, got %d", 3, len(changed))
	}
	if changed["name"] != "Bob" {
		t.Errorf("got %v, want %v", changed["name"], "Bob")
	}
	if changed["email"] != "bob@example.com" {
		t.Errorf("got %v, want %v", changed["email"], "bob@example.com")
	}
	if changed["status"] != "inactive" {
		t.Errorf("got %v, want %v", changed["status"], "inactive")
	}
	if _, ok := changed["id"]; ok {
		t.Errorf("%q should not contain %q", changed, "id")
	}
}

func TestDiffFields_TimeFieldChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	original := diffUserWithTime{ID: 1, Name: "Alice", CreatedAt: t1}
	current := diffUserWithTime{ID: 1, Name: "Alice", CreatedAt: t2}

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(changed))
	}
	if changed["created_at"] != t2 {
		t.Errorf("got %v, want %v", changed["created_at"], t2)
	}
}

func TestDiffFields_TimeFieldUnchanged(t *testing.T) {
	db := upsertMockDB("postgres")
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	original := diffUserWithTime{ID: 1, Name: "Alice", CreatedAt: ts}
	current := original

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("expected empty, got %d", len(changed))
	}
}

func TestDiffFields_TypeMismatch_Error(t *testing.T) {
	db := upsertMockDB("postgres")

	type OtherModel struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	current := diffUser{ID: 1, Name: "Alice", Email: "a@b.com", Status: "active"}
	other := OtherModel{ID: 1, Name: "Alice"}

	mq := newTestMQ(db, &current, "users")

	_, err := mq.diffFields(&other)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not match model type") {
		t.Errorf("%q does not contain %q", err.Error(), "does not match model type")
	}
}

func TestDiffFields_OriginalNotStruct_Error(t *testing.T) {
	db := upsertMockDB("postgres")
	current := diffUser{ID: 1}

	mq := newTestMQ(db, &current, "users")

	notStruct := 42
	_, err := mq.diffFields(&notStruct)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "original is not a struct") {
		t.Errorf("%q does not contain %q", err.Error(), "original is not a struct")
	}
}

func TestDiffFields_OriginalPassedByValue(t *testing.T) {
	// original passed as value (not pointer) — should work.
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original
	current.Name = "Bob"

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(changed))
	}
	if changed["name"] != "Bob" {
		t.Errorf("got %v, want %v", changed["name"], "Bob")
	}
}

// ============================================================================
// UpdateChanged error path tests
// ============================================================================

func TestUpdateChanged_EmptyTable_Error(t *testing.T) {
	db := upsertMockDB("postgres")
	current := diffUser{ID: 1, Name: "Alice", Email: "a@b.com", Status: "active"}
	original := current
	current.Name = "Bob"

	mq := newTestMQ(db, &current, "")

	err := mq.UpdateChanged(&original)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "table name not specified") {
		t.Errorf("%q does not contain %q", err.Error(), "table name not specified")
	}
}

func TestUpdateChanged_TypeMismatch_Error(t *testing.T) {
	db := upsertMockDB("postgres")

	type AnotherType struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	current := diffUser{ID: 1, Name: "Alice", Email: "a@b.com", Status: "active"}
	other := AnotherType{ID: 1, Name: "Bob"}

	mq := newTestMQ(db, &current, "users")

	err := mq.UpdateChanged(&other)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not match model type") {
		t.Errorf("%q does not contain %q", err.Error(), "does not match model type")
	}
}

func TestUpdateChanged_NoPrimaryKey_Error(t *testing.T) {
	db := upsertMockDB("postgres")

	type NoPK struct {
		Name  string `db:"name"`
		Email string `db:"email"`
	}
	current := NoPK{Name: "Alice", Email: "a@b.com"}
	original := current
	current.Name = "Bob"

	mq := newTestMQ(db, &current, "nopk")

	err := mq.UpdateChanged(&original)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "primary key not found") {
		t.Errorf("%q does not contain %q", err.Error(), "primary key not found")
	}
}

// TestUpdateChanged_NothingChanged_NilNoQuery verifies that when nothing
// changed, UpdateChanged returns nil without executing any query.
// We verify via diffFields returning empty (no real DB needed).
func TestUpdateChanged_NothingChanged_NilNoQuery(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original // identical

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("no fields should be reported as changed — no query will execute: expected empty, got %d", len(changed))
	}
}

// ============================================================================
// ============================================================================

// ============================================================================
// ============================================================================
