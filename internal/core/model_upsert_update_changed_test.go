package core

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/util"
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
// buildUpsertUpdateCols unit tests
// ============================================================================

func TestBuildUpsertUpdateCols_NoFieldsSpecified(t *testing.T) {
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"id": 1, "name": "Alice", "email": "a@b.com"}
	pkCols := []string{"id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	// result should contain "name" and "email" but not "id"
	want := map[string]bool{"name": true, "email": true}
	if len(result) != len(want) {
		t.Errorf("expected length %d, got %d", len(want), len(result))
	}
	for _, col := range result {
		if col == "id" {
			t.Errorf("PK should not be in update cols: expected different, both %v", col)
		}
	}
}

func TestBuildUpsertUpdateCols_SpecificFieldsWithPK(t *testing.T) {
	// If caller includes the PK in fields, it must be excluded.
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"id": 1, "name": "Alice", "email": "a@b.com"}
	pkCols := []string{"id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, []string{"name", "id"})

	if len(result) != 1 || result[0] != "name" {
		t.Errorf("got %v, want %v", result, []string{"name"})
	}
}

func TestBuildUpsertUpdateCols_SingleSpecificField(t *testing.T) {
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"id": 1, "name": "Alice", "email": "a@b.com"}
	pkCols := []string{"id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, []string{"email"})

	if len(result) != 1 || result[0] != "email" {
		t.Errorf("got %v, want %v", result, []string{"email"})
	}
}

func TestBuildUpsertUpdateCols_CompositePK(t *testing.T) {
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"order_id": 1, "product_id": 2, "qty": 5}
	pkCols := []string{"order_id", "product_id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	if len(result) != 1 || result[0] != "qty" {
		t.Errorf("got %v, want %v", result, []string{"qty"})
	}
}

// ============================================================================
// Upsert SQL generation tests (no real DB, only SQL verification)
// ============================================================================

func TestModelUpsert_SQL_PostgreSQL_AllFields(t *testing.T) {
	db := upsertMockDB("postgres")
	user := upsertUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}

	dataMap, err := util.StructToMap(&user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkCols := []string{"id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, `INSERT INTO "users"`) {
		t.Errorf("%q does not contain %q", q.sql, `INSERT INTO "users"`)
	}
	if !strings.Contains(q.sql, `ON CONFLICT ("id")`) {
		t.Errorf("%q does not contain %q", q.sql, `ON CONFLICT ("id")`)
	}
	if !strings.Contains(q.sql, "DO UPDATE SET") {
		t.Errorf("%q does not contain %q", q.sql, "DO UPDATE SET")
	}
	if !strings.Contains(q.sql, `"name" = EXCLUDED."name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name" = EXCLUDED."name"`)
	}
	if !strings.Contains(q.sql, `"email" = EXCLUDED."email"`) {
		t.Errorf("%q does not contain %q", q.sql, `"email" = EXCLUDED."email"`)
	}
	if !strings.Contains(q.sql, `"status" = EXCLUDED."status"`) {
		t.Errorf("%q does not contain %q", q.sql, `"status" = EXCLUDED."status"`)
	}
	if strings.Contains(q.sql, `"id" = EXCLUDED."id"`) {
		t.Errorf("%q should not contain %q", q.sql, `"id" = EXCLUDED."id"`)
	}
}

func TestModelUpsert_SQL_MySQL_AllFields(t *testing.T) {
	db := upsertMockDB("mysql")
	user := upsertUser{ID: 2, Name: "Bob", Email: "bob@example.com", Status: "pending"}

	dataMap, err := util.StructToMap(&user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkCols := []string{"id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, "INSERT INTO `users`") {
		t.Errorf("%q does not contain %q", q.sql, "INSERT INTO `users`")
	}
	if !strings.Contains(q.sql, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("%q does not contain %q", q.sql, "ON DUPLICATE KEY UPDATE")
	}
	if !strings.Contains(q.sql, "`name` = VALUES(`name`)") {
		t.Errorf("%q does not contain %q", q.sql, "`name` = VALUES(`name`)")
	}
	if !strings.Contains(q.sql, "`email` = VALUES(`email`)") {
		t.Errorf("%q does not contain %q", q.sql, "`email` = VALUES(`email`)")
	}
	if !strings.Contains(q.sql, "`status` = VALUES(`status`)") {
		t.Errorf("%q does not contain %q", q.sql, "`status` = VALUES(`status`)")
	}
}

func TestModelUpsert_SQL_SQLite_AllFields(t *testing.T) {
	db := upsertMockDB("sqlite")
	user := upsertUser{ID: 3, Name: "Carol", Email: "carol@example.com", Status: "active"}

	dataMap, err := util.StructToMap(&user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkCols := []string{"id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, `INSERT INTO "users"`) {
		t.Errorf("%q does not contain %q", q.sql, `INSERT INTO "users"`)
	}
	if !strings.Contains(q.sql, `ON CONFLICT ("id")`) {
		t.Errorf("%q does not contain %q", q.sql, `ON CONFLICT ("id")`)
	}
	if !strings.Contains(q.sql, "DO UPDATE SET") {
		t.Errorf("%q does not contain %q", q.sql, "DO UPDATE SET")
	}
	if !strings.Contains(q.sql, `"name" = excluded."name"`) {
		t.Errorf("%q does not contain %q", q.sql, `"name" = excluded."name"`)
	}
}

func TestModelUpsert_SQL_SelectiveFields(t *testing.T) {
	tests := []struct {
		name        string
		dialectName string
		updateCol   string
		expectSQL   string
		notExpect   []string
	}{
		{
			name:        "postgres selective",
			dialectName: "postgres",
			updateCol:   `"name" = EXCLUDED."name"`,
			expectSQL:   "DO UPDATE SET",
			notExpect:   []string{`"email" = EXCLUDED."email"`, `"status" = EXCLUDED."status"`},
		},
		{
			name:        "mysql selective",
			dialectName: "mysql",
			updateCol:   "`name` = VALUES(`name`)",
			expectSQL:   "ON DUPLICATE KEY UPDATE",
			notExpect:   []string{"`email` = VALUES(`email`)", "`status` = VALUES(`status`)"},
		},
		{
			name:        "sqlite selective",
			dialectName: "sqlite",
			updateCol:   `"name" = excluded."name"`,
			expectSQL:   "DO UPDATE SET",
			notExpect:   []string{`"email" = excluded."email"`, `"status" = excluded."status"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := upsertMockDB(tt.dialectName)
			user := upsertUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}

			dataMap, err := util.StructToMap(&user)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			pkCols := []string{"id"}
			mq := &ModelQuery{exclude: make(map[string]bool)}
			// Only "name" on conflict.
			updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, []string{"name"})

			qb := &QueryBuilder{db: db}
			q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

			if q == nil {
				t.Fatal("expected non-nil")
			}
			if !strings.Contains(q.sql, tt.updateCol) {
				t.Errorf("%q does not contain %q", q.sql, tt.updateCol)
			}
			if !strings.Contains(q.sql, tt.expectSQL) {
				t.Errorf("%q does not contain %q", q.sql, tt.expectSQL)
			}
			for _, ne := range tt.notExpect {
				if strings.Contains(q.sql, ne) {
					t.Errorf("%q should not contain %q", q.sql, ne)
				}
			}
		})
	}
}

func TestModelUpsert_SQL_ExplicitPKTag(t *testing.T) {
	db := upsertMockDB("postgres")
	post := upsertPost{PostID: 10, Content: "Hello", Views: 5}

	dataMap, err := util.StructToMap(&post)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkCols := []string{"post_id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("posts", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	if q == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(q.sql, `ON CONFLICT ("post_id")`) {
		t.Errorf("%q does not contain %q", q.sql, `ON CONFLICT ("post_id")`)
	}
	if !strings.Contains(q.sql, `"content" = EXCLUDED."content"`) {
		t.Errorf("%q does not contain %q", q.sql, `"content" = EXCLUDED."content"`)
	}
	if !strings.Contains(q.sql, `"views" = EXCLUDED."views"`) {
		t.Errorf("%q does not contain %q", q.sql, `"views" = EXCLUDED."views"`)
	}
	if strings.Contains(q.sql, `"post_id" = EXCLUDED."post_id"`) {
		t.Errorf("%q should not contain %q", q.sql, `"post_id" = EXCLUDED."post_id"`)
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
// columnFromField unit tests
// ============================================================================

func TestColumnFromField_WithTag(t *testing.T) {
	type sample struct {
		Name string `db:"name"`
	}
	t2 := reflect.TypeOf(sample{})
	col, skip := columnFromField(t2.Field(0))
	if skip {
		t.Error("expected false")
	}
	if col != "name" {
		t.Errorf("got %v, want %v", col, "name")
	}
}

func TestColumnFromField_WithSkipTag(t *testing.T) {
	type sample struct {
		Name string `db:"-"`
	}
	t2 := reflect.TypeOf(sample{})
	_, skip := columnFromField(t2.Field(0))
	if !skip {
		t.Error("expected true")
	}
}

func TestColumnFromField_NoTag(t *testing.T) {
	type sample struct {
		MyField string
	}
	t2 := reflect.TypeOf(sample{})
	col, skip := columnFromField(t2.Field(0))
	if skip {
		t.Error("expected false")
	}
	if col != "MyField" {
		t.Errorf("got %v, want %v", col, "MyField")
	}
}

func TestColumnFromField_PKCompositeTag(t *testing.T) {
	// db:"col_name,pk" → column = "col_name"
	type sample struct {
		TenantID int `db:"tenant_id,pk"`
	}
	t2 := reflect.TypeOf(sample{})
	col, skip := columnFromField(t2.Field(0))
	if skip {
		t.Error("expected false")
	}
	if col != "tenant_id" {
		t.Errorf("got %v, want %v", col, "tenant_id")
	}
}

// ============================================================================
// buildPKSet unit tests
// ============================================================================

func TestBuildPKSet_SinglePK(t *testing.T) {
	pkInfo := &util.PrimaryKeyInfo{
		Columns: []string{"id"},
	}
	set := buildPKSet(pkInfo)
	if !set["id"] {
		t.Error("expected true")
	}
	if set["name"] {
		t.Error("expected false")
	}
}

func TestBuildPKSet_CompositePK(t *testing.T) {
	pkInfo := &util.PrimaryKeyInfo{
		Columns: []string{"order_id", "product_id"},
	}
	set := buildPKSet(pkInfo)
	if !set["order_id"] {
		t.Error("expected true")
	}
	if !set["product_id"] {
		t.Error("expected true")
	}
	if set["qty"] {
		t.Error("expected false")
	}
}

func TestBuildPKSet_Nil(t *testing.T) {
	set := buildPKSet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty, got %d", len(set))
	}
}
