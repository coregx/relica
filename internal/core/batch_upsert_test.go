package core

import (
	"errors"
	"strings"
	"testing"
)

// helper: build and get SQL + params from BatchInsertQuery
func batchSQL(biq *BatchInsertQuery) (string, []interface{}) {
	q := biq.Build()
	return q.sql, q.params
}

// ============================================================================
// BatchInsertQuery.OnConflict + DoUpdate — SQL generation per dialect
// ============================================================================

func TestBatchInsert_OnConflict_DoUpdate_Postgres(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := batchSQL(qb.BatchInsert("products", []string{"sku", "name", "price"}).
		Values("SKU-001", "Widget", 9.99).
		Values("SKU-002", "Gadget", 19.99).
		OnConflict("sku").
		DoUpdate("name", "price"))

	checks := []string{
		`INSERT INTO "products"`,
		`ON CONFLICT ("sku") DO UPDATE SET`,
		`"name" = EXCLUDED."name"`,
		`"price" = EXCLUDED."price"`,
	}
	for _, c := range checks {
		if !strings.Contains(sql, c) {
			t.Errorf("SQL missing %q:\n  %s", c, sql)
		}
	}
	if len(params) != 6 {
		t.Errorf("expected 6 params, got %d", len(params))
	}
}

func TestBatchInsert_OnConflict_DoUpdate_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sql, params := batchSQL(qb.BatchInsert("products", []string{"sku", "name", "price"}).
		Values("SKU-001", "Widget", 9.99).
		Values("SKU-002", "Gadget", 19.99).
		OnConflict("sku").
		DoUpdate("name", "price"))

	checks := []string{
		"INSERT INTO `products`",
		"ON DUPLICATE KEY UPDATE",
		"`name` = VALUES(`name`)",
		"`price` = VALUES(`price`)",
	}
	for _, c := range checks {
		if !strings.Contains(sql, c) {
			t.Errorf("SQL missing %q:\n  %s", c, sql)
		}
	}
	if len(params) != 6 {
		t.Errorf("expected 6 params, got %d", len(params))
	}
}

func TestBatchInsert_OnConflict_DoUpdate_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sql, params := batchSQL(qb.BatchInsert("nodes", []string{"project_id", "qualified_name", "name", "label"}).
		Values("p1", "pkg.Foo", "Foo", "struct").
		Values("p1", "pkg.Bar", "Bar", "func").
		OnConflict("project_id", "qualified_name").
		DoUpdate("name", "label"))

	checks := []string{
		`INSERT INTO "nodes"`,
		`ON CONFLICT ("project_id", "qualified_name") DO UPDATE SET`,
		`"name" = excluded."name"`,
		`"label" = excluded."label"`,
	}
	for _, c := range checks {
		if !strings.Contains(sql, c) {
			t.Errorf("SQL missing %q:\n  %s", c, sql)
		}
	}
	if len(params) != 8 {
		t.Errorf("expected 8 params, got %d", len(params))
	}
}

// ============================================================================
// DoNothing
// ============================================================================

func TestBatchInsert_OnConflict_DoNothing_Postgres(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := batchSQL(qb.BatchInsert("events", []string{"id", "type"}).
		Values(1, "click").
		Values(2, "view").
		OnConflict("id").
		DoNothing())

	if !strings.Contains(sql, `ON CONFLICT ("id") DO NOTHING`) {
		t.Errorf("missing DO NOTHING: %s", sql)
	}
	if strings.Contains(sql, "DO UPDATE") {
		t.Errorf("should NOT contain DO UPDATE: %s", sql)
	}
}

func TestBatchInsert_OnConflict_DoNothing_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sql, _ := batchSQL(qb.BatchInsert("events", []string{"id", "type"}).
		Values(1, "click").
		OnConflict("id").
		DoNothing())

	if !strings.Contains(sql, `ON CONFLICT ("id") DO NOTHING`) {
		t.Errorf("missing DO NOTHING: %s", sql)
	}
}

// ============================================================================
// Without OnConflict — existing behavior unchanged
// ============================================================================

func TestBatchInsert_WithoutOnConflict_Unchanged(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := batchSQL(qb.BatchInsert("users", []string{"name", "email"}).
		Values("Alice", "alice@example.com").
		Values("Bob", "bob@example.com"))

	if strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("should NOT contain ON CONFLICT: %s", sql)
	}
	if len(params) != 4 {
		t.Errorf("expected 4 params, got %d", len(params))
	}
}

// ============================================================================
// Auto-detect update columns
// ============================================================================

func TestBatchInsert_OnConflict_AutoDetectUpdateCols(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := batchSQL(qb.BatchInsert("products", []string{"sku", "name", "price"}).
		Values("SKU-001", "Widget", 9.99).
		OnConflict("sku"))

	if !strings.Contains(sql, `ON CONFLICT ("sku") DO UPDATE SET`) {
		t.Errorf("missing ON CONFLICT: %s", sql)
	}
	if !strings.Contains(sql, `"name" = EXCLUDED."name"`) {
		t.Errorf("missing auto-detected name: %s", sql)
	}
	if !strings.Contains(sql, `"price" = EXCLUDED."price"`) {
		t.Errorf("missing auto-detected price: %s", sql)
	}
	if strings.Contains(sql, `"sku" = EXCLUDED."sku"`) {
		t.Errorf("should NOT update conflict column: %s", sql)
	}
}

// ============================================================================
// DoNothing overrides DoUpdate
// ============================================================================

func TestBatchInsert_DoNothing_Overrides_DoUpdate(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := batchSQL(qb.BatchInsert("t", []string{"a", "b"}).
		Values(1, 2).
		OnConflict("a").
		DoUpdate("b").
		DoNothing())

	if !strings.Contains(sql, "DO NOTHING") {
		t.Errorf("DoNothing should override: %s", sql)
	}
	if strings.Contains(sql, "DO UPDATE") {
		t.Errorf("should NOT contain DO UPDATE: %s", sql)
	}
}

// ============================================================================
// Many rows
// ============================================================================

func TestBatchInsert_OnConflict_100Rows(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	batch := qb.BatchInsert("items", []string{"id", "name"}).
		OnConflict("id").
		DoUpdate("name")

	for i := 0; i < 100; i++ {
		batch.Values(i, "item")
	}

	sql, params := batchSQL(batch)

	if len(params) != 200 {
		t.Errorf("expected 200 params, got %d", len(params))
	}
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("missing ON CONFLICT")
	}
}

// ============================================================================
// ModelQuery.UpsertOn
// ============================================================================

func TestUpsertOn_EmptyConflictColumns_Error(t *testing.T) {
	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	user := &User{Name: "Alice"}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres"), table: "users"}

	err := mq.UpsertOn(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "at least one conflict column") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpsertOn_EmptyTable_Error(t *testing.T) {
	type User struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	user := &User{Name: "Alice"}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres"), table: ""}

	err := mq.UpsertOn([]string{"name"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "table name not specified") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpsertOn_CallsBeforeInsert(t *testing.T) {
	model := &upsertOnHookModel{Name: "Alice", Email: "alice@example.com"}
	mq := &ModelQuery{model: model, db: mockDBFull("postgres"), table: "users"}
	mq.callBeforeInsert()

	if !model.hook {
		t.Error("BeforeInsert should have been called")
	}
}

func TestUpsertOn_BeforeInsert_Error_Aborts(t *testing.T) {
	model := &upsertOnHookModel{Name: "", Email: "test@test.com"}
	mq := &ModelQuery{model: model, db: mockDBFull("postgres"), table: "users"}

	err := mq.UpsertOn([]string{"email"})
	if err == nil {
		t.Fatal("expected error from BeforeInsert")
	}
	if !strings.Contains(err.Error(), "name required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpsertOn_PopulatesAutoID(t *testing.T) {
	type Node struct {
		ID            int64  `db:"id,pk"`
		PublicID      string `db:"public_id,autoid:nod"`
		ProjectID     string `db:"project_id"`
		QualifiedName string `db:"qualified_name"`
		Name          string `db:"name"`
	}

	node := &Node{ProjectID: "p1", QualifiedName: "pkg.Foo", Name: "Foo"}
	mq := &ModelQuery{model: node, db: mockDBFull("postgres"), table: "nodes"}

	// Simulate what UpsertOn does: callBeforeInsert + populateAutoIDFields
	mq.callBeforeInsert()
	mq.populateAutoIDFields()

	if node.PublicID == "" {
		t.Error("AutoID should have been populated")
	}
	if !strings.HasPrefix(node.PublicID, "nod_") {
		t.Errorf("expected nod_ prefix, got %s", node.PublicID)
	}
}

// hook model for UpsertOn tests
type upsertOnHookModel struct {
	ID    int64  `db:"id,pk"`
	Name  string `db:"name"`
	Email string `db:"email"`
	hook  bool
}

func (m *upsertOnHookModel) BeforeInsert() error {
	m.hook = true
	if m.Name == "" {
		return errors.New("name required")
	}
	return nil
}
