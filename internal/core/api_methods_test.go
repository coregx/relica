package core

import (
	"context"
	"strings"
	"testing"

	"github.com/coregx/relica/internal/dialects"
)

// ─── DB.SqlDB ────────────────────────────────────────────────────────────────

// TestDB_SqlDB verifies that SqlDB() returns a non-nil *sql.DB.
func TestDB_SqlDB(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	if got := db.SqlDB(); got == nil {
		t.Fatal("SqlDB() returned nil, want non-nil *sql.DB")
	}
}

// TestDB_SqlDB_SameConnection verifies that SqlDB() returns the exact *sql.DB
// instance that the DB was constructed with (no copy, no wrapper).
func TestDB_SqlDB_SameConnection(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	got := db.SqlDB()
	if got != db.sqlDB {
		t.Errorf("SqlDB() returned different pointer: got %p, want %p", got, db.sqlDB)
	}
}

// ─── DB.PingContext ───────────────────────────────────────────────────────────

// TestDB_PingContext verifies that PingContext succeeds for a healthy connection.
func TestDB_PingContext(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext() error = %v, want nil", err)
	}
}

// ─── DB.DriverName ───────────────────────────────────────────────────────────

// TestDB_DriverName_SQLite verifies DriverName() for a SQLite-dialect DB.
func TestDB_DriverName_SQLite(t *testing.T) {
	db := mockDBFull("sqlite")

	if got := db.DriverName(); got != "sqlite" {
		t.Errorf("DriverName() = %q, want %q", got, "sqlite")
	}
}

// TestDB_DriverName_Postgres verifies DriverName() for a Postgres-dialect DB.
func TestDB_DriverName_Postgres(t *testing.T) {
	db := mockDBFull("postgres")

	if got := db.DriverName(); got != "postgres" {
		t.Errorf("DriverName() = %q, want %q", got, "postgres")
	}
}

// TestDB_DriverName_MySQL verifies DriverName() for a MySQL-dialect DB.
func TestDB_DriverName_MySQL(t *testing.T) {
	db := mockDBFull("mysql")

	if got := db.DriverName(); got != "mysql" {
		t.Errorf("DriverName() = %q, want %q", got, "mysql")
	}
}

// ─── SelectQuery.ForUpdate ────────────────────────────────────────────────────

// TestSelectQuery_ForUpdate_Postgres verifies that ForUpdate appends "FOR UPDATE"
// to the generated SQL on PostgreSQL (where row-level locking is supported).
func TestSelectQuery_ForUpdate_Postgres(t *testing.T) {
	db := mockDBFull("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select().From("jobs").ForUpdate().ToSQL()
	if !strings.Contains(sql, "FOR UPDATE") {
		t.Errorf("Postgres ForUpdate: SQL %q does not contain %q", sql, "FOR UPDATE")
	}
}

// TestSelectQuery_ForUpdate_MySQL verifies that ForUpdate appends "FOR UPDATE"
// to the generated SQL on MySQL.
func TestSelectQuery_ForUpdate_MySQL(t *testing.T) {
	db := mockDBFull("mysql")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select().From("jobs").ForUpdate().ToSQL()
	if !strings.Contains(sql, "FOR UPDATE") {
		t.Errorf("MySQL ForUpdate: SQL %q does not contain %q", sql, "FOR UPDATE")
	}
}

// TestSelectQuery_ForUpdate_SQLite_Ignored verifies that ForUpdate is silently
// ignored on SQLite (no row-level locking support). The query must succeed and
// must NOT contain "FOR UPDATE".
func TestSelectQuery_ForUpdate_SQLite_Ignored(t *testing.T) {
	db := mockDBFull("sqlite3")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select().From("jobs").ForUpdate().ToSQL()
	if strings.Contains(sql, "FOR UPDATE") {
		t.Errorf("SQLite ForUpdate: SQL %q must NOT contain %q", sql, "FOR UPDATE")
	}
}

// TestSelectQuery_ForShare_Postgres verifies that ForShare appends "FOR SHARE"
// to the generated SQL on PostgreSQL.
func TestSelectQuery_ForShare_Postgres(t *testing.T) {
	db := mockDBFull("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select().From("sessions").ForShare().ToSQL()
	if !strings.Contains(sql, "FOR SHARE") {
		t.Errorf("Postgres ForShare: SQL %q does not contain %q", sql, "FOR SHARE")
	}
}

// TestSelectQuery_ForShare_SQLite_Ignored verifies ForShare is ignored on SQLite.
func TestSelectQuery_ForShare_SQLite_Ignored(t *testing.T) {
	db := mockDBFull("sqlite")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select().From("sessions").ForShare().ToSQL()
	if strings.Contains(sql, "FOR SHARE") {
		t.Errorf("SQLite ForShare: SQL %q must NOT contain %q", sql, "FOR SHARE")
	}
}

// TestSelectQuery_ForUpdateSkipLocked verifies that ForUpdateSkipLocked appends
// "FOR UPDATE SKIP LOCKED" to the generated SQL on PostgreSQL.
func TestSelectQuery_ForUpdateSkipLocked(t *testing.T) {
	db := mockDBFull("postgres")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select().From("jobs").ForUpdateSkipLocked().ToSQL()
	if !strings.Contains(sql, "FOR UPDATE SKIP LOCKED") {
		t.Errorf("ForUpdateSkipLocked: SQL %q does not contain %q", sql, "FOR UPDATE SKIP LOCKED")
	}
}

// TestSelectQuery_ForUpdateSkipLocked_SQLite_Ignored verifies ForUpdateSkipLocked
// is ignored on SQLite.
func TestSelectQuery_ForUpdateSkipLocked_SQLite_Ignored(t *testing.T) {
	db := mockDBFull("sqlite")
	qb := &QueryBuilder{db: db}

	sql, _ := qb.Select().From("jobs").ForUpdateSkipLocked().ToSQL()
	if strings.Contains(sql, "SKIP LOCKED") {
		t.Errorf("SQLite ForUpdateSkipLocked: SQL %q must NOT contain %q", sql, "SKIP LOCKED")
	}
}

// ─── IsNull / IsNotNull ───────────────────────────────────────────────────────

// TestIsNull_SQL verifies that IsNull(col) generates "col IS NULL".
func TestIsNull_SQL(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// IsNull delegates to Eq(col, nil), which produces "col IS NULL".
	expr := Eq("deleted_at", nil)
	sql, args := expr.Build(dialect)

	if !strings.Contains(sql, "IS NULL") {
		t.Errorf("IsNull: SQL %q does not contain %q", sql, "IS NULL")
	}
	if len(args) != 0 {
		t.Errorf("IsNull: expected no args, got %v", args)
	}
}

// TestIsNotNull_SQL verifies that IsNotNull(col) generates "col IS NOT NULL".
func TestIsNotNull_SQL(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	// IsNotNull delegates to NotEq(col, nil), which produces "col IS NOT NULL".
	expr := NotEq("deleted_at", nil)
	sql, args := expr.Build(dialect)

	if !strings.Contains(sql, "IS NOT NULL") {
		t.Errorf("IsNotNull: SQL %q does not contain %q", sql, "IS NOT NULL")
	}
	if len(args) != 0 {
		t.Errorf("IsNotNull: expected no args, got %v", args)
	}
}

// TestIsNull_Column_Quoting verifies the column name is quoted correctly.
func TestIsNull_Column_Quoting(t *testing.T) {
	tests := []struct {
		dialectName string
		col         string
		wantContain string
	}{
		{"postgres", "deleted_at", `"deleted_at" IS NULL`},
		{"mysql", "deleted_at", "`deleted_at` IS NULL"},
		{"sqlite", "deleted_at", `"deleted_at" IS NULL`},
	}

	for _, tt := range tests {
		t.Run(tt.dialectName, func(t *testing.T) {
			dialect := dialects.GetDialect(tt.dialectName)
			expr := Eq(tt.col, nil)
			sql, _ := expr.Build(dialect)
			if !strings.Contains(sql, tt.wantContain) {
				t.Errorf("dialect %s: SQL %q does not contain %q", tt.dialectName, sql, tt.wantContain)
			}
		})
	}
}

// ─── GreaterOrEqualCol / LessOrEqualCol ──────────────────────────────────────

// TestGreaterOrEqualCol_SQL verifies that GreaterOrEqualCol generates
// "col1 >= col2" with proper identifier quoting (no positional args).
func TestGreaterOrEqualCol_SQL(t *testing.T) {
	tests := []struct {
		dialectName string
		col1        string
		col2        string
		wantOp      string
	}{
		{"postgres", "price", "min_price", `"price" >= "min_price"`},
		{"mysql", "price", "min_price", "`price` >= `min_price`"},
		{"sqlite", "price", "min_price", `"price" >= "min_price"`},
	}

	for _, tt := range tests {
		t.Run(tt.dialectName, func(t *testing.T) {
			dialect := dialects.GetDialect(tt.dialectName)
			expr := GreaterOrEqualCol(tt.col1, tt.col2)
			sql, args := expr.Build(dialect)

			if !strings.Contains(sql, tt.wantOp) {
				t.Errorf("GreaterOrEqualCol [%s]: SQL %q does not contain %q", tt.dialectName, sql, tt.wantOp)
			}
			if len(args) != 0 {
				t.Errorf("GreaterOrEqualCol: expected no args (col-to-col), got %v", args)
			}
		})
	}
}

// TestLessOrEqualCol_SQL verifies that LessOrEqualCol generates "col1 <= col2".
func TestLessOrEqualCol_SQL(t *testing.T) {
	tests := []struct {
		dialectName string
		col1        string
		col2        string
		wantOp      string
	}{
		{"postgres", "score", "max_score", `"score" <= "max_score"`},
		{"mysql", "score", "max_score", "`score` <= `max_score`"},
		{"sqlite", "score", "max_score", `"score" <= "max_score"`},
	}

	for _, tt := range tests {
		t.Run(tt.dialectName, func(t *testing.T) {
			dialect := dialects.GetDialect(tt.dialectName)
			expr := LessOrEqualCol(tt.col1, tt.col2)
			sql, args := expr.Build(dialect)

			if !strings.Contains(sql, tt.wantOp) {
				t.Errorf("LessOrEqualCol [%s]: SQL %q does not contain %q", tt.dialectName, sql, tt.wantOp)
			}
			if len(args) != 0 {
				t.Errorf("LessOrEqualCol: expected no args (col-to-col), got %v", args)
			}
		})
	}
}

// TestGreaterOrEqualCol_DottedIdentifiers verifies quoting with dotted
// (schema.column) identifiers.
func TestGreaterOrEqualCol_DottedIdentifiers(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	expr := GreaterOrEqualCol("o.created_at", "u.start_date")
	sql, _ := expr.Build(dialect)

	// Each dotted part must be quoted separately.
	if !strings.Contains(sql, `"o"."created_at"`) || !strings.Contains(sql, `"u"."start_date"`) {
		t.Errorf("dotted identifiers: SQL %q does not contain expected quoting", sql)
	}
	if !strings.Contains(sql, ">=") {
		t.Errorf("dotted identifiers: SQL %q does not contain >=", sql)
	}
}

// TestLessOrEqualCol_NoArgs verifies LessOrEqualCol never produces bind params.
func TestLessOrEqualCol_NoArgs(t *testing.T) {
	dialect := dialects.GetDialect("postgres")
	expr := LessOrEqualCol("a", "b")
	_, args := expr.Build(dialect)
	if len(args) != 0 {
		t.Errorf("LessOrEqualCol: expected 0 args, got %v", args)
	}
}

// ─── ModelQuery.Find — error cases ───────────────────────────────────────────

// TestFind_SinglePK_WrongArgCount verifies that Find() returns an error when
// the number of PK values does not match the number of PK columns.
func TestFind_SinglePK_WrongArgCount(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type Item struct {
		ID   int    `db:"id,pk"`
		Name string `db:"name"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &Item{},
		table:   "items",
		exclude: make(map[string]bool),
	}

	// Single PK struct but two values passed — must error.
	err := mq.Find(1, 2)
	if err == nil {
		t.Fatal("Find(1, 2) on single-PK model: expected error, got nil")
	}
}

// TestFind_EmptyTable_Error verifies that Find() returns an error when
// the ModelQuery has an empty table name.
func TestFind_EmptyTable_Error(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type Item struct {
		ID   int    `db:"id,pk"`
		Name string `db:"name"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &Item{},
		table:   "", // deliberately empty
		exclude: make(map[string]bool),
	}

	err := mq.Find(1)
	if err == nil {
		t.Fatal("Find on empty-table ModelQuery: expected error, got nil")
	}
}

// TestFind_NoPK_Error verifies that Find() returns an error when the model
// struct has no primary key field.
func TestFind_NoPK_Error(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	// Struct without any PK annotation and no "ID"/"Id" field.
	type NoPKModel struct {
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &NoPKModel{},
		table:   "no_pk_table",
		exclude: make(map[string]bool),
	}

	err := mq.Find(42)
	if err == nil {
		t.Fatal("Find on model without PK: expected error, got nil")
	}
}

// TestFind_ZeroPKArgs_Error verifies that Find() with no arguments returns
// an error when the struct has a single-column PK (wrong arg count: 0 vs 1).
func TestFind_ZeroPKArgs_Error(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type Item struct {
		ID   int    `db:"id,pk"`
		Name string `db:"name"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &Item{},
		table:   "items",
		exclude: make(map[string]bool),
	}

	err := mq.Find() // no pk values
	if err == nil {
		t.Fatal("Find() with no args on single-PK model: expected error, got nil")
	}
}

// TestFind_CompositePK_WrongArgCount verifies composite PK arg count validation.
func TestFind_CompositePK_WrongArgCount(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type OrderItem struct {
		OrderID int `db:"order_id,pk"`
		ItemID  int `db:"item_id,pk"`
		Qty     int `db:"qty"`
	}

	mq := &ModelQuery{
		db:      db,
		model:   &OrderItem{},
		table:   "order_items",
		exclude: make(map[string]bool),
	}

	// Composite PK needs exactly 2 values; passing 1 must error.
	err := mq.Find(1)
	if err == nil {
		t.Fatal("Find(1) on composite-PK model: expected error, got nil")
	}
}

// ─── SelectQuery.Model — error cases ─────────────────────────────────────────

// TestSelectQuery_Model_NilDest verifies that Model(nil) returns an error.
func TestSelectQuery_Model_NilDest(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	qb := &QueryBuilder{db: db}
	err := qb.Select().From("some_table").Model(nil)
	if err == nil {
		t.Fatal("Model(nil): expected error, got nil")
	}
}

// TestSelectQuery_Model_NotPointer verifies that Model with a non-pointer value returns an error.
func TestSelectQuery_Model_NotPointer(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type Row struct {
		ID int `db:"id"`
	}

	qb := &QueryBuilder{db: db}
	err := qb.Select().From("rows").Model(Row{ID: 1})
	if err == nil {
		t.Fatal("Model(non-pointer): expected error, got nil")
	}
}

// TestSelectQuery_Model_NotStruct verifies that Model with a pointer to a non-struct returns an error.
func TestSelectQuery_Model_NotStruct(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	qb := &QueryBuilder{db: db}
	n := 42
	err := qb.Select().From("some_table").Model(&n)
	if err == nil {
		t.Fatal("Model(pointer-to-int): expected error, got nil")
	}
}

// TestSelectQuery_Model_NoPK verifies that Model returns an error when the struct has no PK field.
func TestSelectQuery_Model_NoPK(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	// Struct without db:"pk" tag, without "ID"/"Id" field.
	type NoPKRow struct {
		Name  string `db:"name"`
		Email string `db:"email"`
	}

	qb := &QueryBuilder{db: db}
	dest := NoPKRow{Name: "alice", Email: "a@b.com"}
	err := qb.Select().From("no_pk_rows").Model(&dest)
	if err == nil {
		t.Fatal("Model on struct without PK: expected error, got nil")
	}
}

// TestSelectQuery_Model_AllPKZero verifies that Model returns an error when all PK fields are zero.
func TestSelectQuery_Model_AllPKZero(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type ZeroItem struct {
		ID   int    `db:"id,pk"`
		Name string `db:"name"`
	}

	qb := &QueryBuilder{db: db}
	dest := ZeroItem{} // ID == 0 (zero value)
	err := qb.Select().From("zero_items").Model(&dest)
	if err == nil {
		t.Fatal("Model with all-zero PK: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "primary key not set") {
		t.Errorf("expected 'primary key not set' in error, got: %v", err)
	}
}

// TestSelectQuery_Model_AutoTable verifies that Model infers the table name from the struct type
// when no From() has been called.
func TestSelectQuery_Model_AutoTable(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	// Table name should be inferred as "model_items" (struct name lowercased + 's').
	type ModelItem struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	ctx := context.Background()
	// Create table using the inferred name.
	_, err := db.ExecContext(ctx,
		`CREATE TABLE modelitems (id INTEGER, name TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO modelitems (id, name) VALUES (7, 'auto')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	dest := ModelItem{ID: 7}
	qb := &QueryBuilder{db: db}
	// No From() — table must be inferred as "modelitems".
	err = qb.Select().Model(&dest)
	if err != nil {
		t.Fatalf("Model() auto-table: unexpected error: %v", err)
	}
	if dest.Name != "auto" {
		t.Errorf("Model() auto-table: expected Name='auto', got %q", dest.Name)
	}
}

// TestSelectQuery_Model_ExplicitFrom verifies that an explicit From() overrides auto-detection.
func TestSelectQuery_Model_ExplicitFrom(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type Anything struct {
		ID    int64  `db:"id,pk"`
		Label string `db:"label"`
	}

	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		`CREATE TABLE explicit_tbl (id INTEGER, label TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO explicit_tbl (id, label) VALUES (3, 'explicit')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	dest := Anything{ID: 3}
	qb := &QueryBuilder{db: db}
	// From("explicit_tbl") must win over auto-inferred "anythings".
	err = qb.Select().From("explicit_tbl").Model(&dest)
	if err != nil {
		t.Fatalf("Model() explicit From: unexpected error: %v", err)
	}
	if dest.Label != "explicit" {
		t.Errorf("Model() explicit From: expected Label='explicit', got %q", dest.Label)
	}
}

// TestSelectQuery_Model_WhereAppend verifies that a pre-set WHERE clause is combined
// with the PK condition (AND) so both filters are applied.
func TestSelectQuery_Model_WhereAppend(t *testing.T) {
	db := openCovDB(t)
	defer db.Close()

	type StatusItem struct {
		ID     int64  `db:"id,pk"`
		Status int64  `db:"status"`
		Name   string `db:"name"`
	}

	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		`CREATE TABLE status_items (id INTEGER, status INTEGER, name TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Two rows with the same id but different status (unusual, just to test filter).
	_, err = db.ExecContext(ctx,
		`INSERT INTO status_items (id, status, name) VALUES (10, 1, 'active')`)
	if err != nil {
		t.Fatalf("insert row 1: %v", err)
	}

	dest := StatusItem{ID: 10}
	qb := &QueryBuilder{db: db}
	// Pre-set WHERE to require status=1. This should be ANDed with id=10.
	err = qb.Select().From("status_items").Where("status = ?", int64(1)).Model(&dest)
	if err != nil {
		t.Fatalf("Model() with WHERE: unexpected error: %v", err)
	}
	if dest.Name != "active" {
		t.Errorf("Model() with WHERE: expected Name='active', got %q", dest.Name)
	}
}
