package core

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// setupRowColumnTestDB creates an in-memory SQLite database for Row/Column testing.
func setupRowColumnTestDB(t *testing.T) *DB {
	t.Helper()

	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create test table.
	_, err = db.sqlDB.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			age INTEGER NOT NULL,
			status TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Insert test data.
	_, err = db.sqlDB.Exec(`
		INSERT INTO users (id, name, email, age, status) VALUES
		(1, 'Alice', 'alice@example.com', 30, 'active'),
		(2, 'Bob', 'bob@example.com', 25, 'active'),
		(3, 'Charlie', 'charlie@example.com', 35, 'inactive'),
		(4, 'Diana', 'diana@example.com', 28, 'active')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	return db
}

// TestQueryRow tests Query.Row() method with various scenarios.
func TestQueryRow(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	t.Run("SingleValue", func(t *testing.T) {
		var count int
		q := &Query{
			sql:    "SELECT COUNT(*) FROM users",
			params: []interface{}{},
			db:     db,
		}
		err := q.Row(&count)
		if err != nil {
			t.Fatalf("Row() failed: %v", err)
		}
		if count != 4 {
			t.Errorf("expected count=4, got %d", count)
		}
	})

	t.Run("MultipleValues", func(t *testing.T) {
		var name string
		var age int
		q := &Query{
			sql:    "SELECT name, age FROM users WHERE id = ?",
			params: []interface{}{1},
			db:     db,
		}
		err := q.Row(&name, &age)
		if err != nil {
			t.Fatalf("Row() failed: %v", err)
		}
		if name != "Alice" {
			t.Errorf("expected name='Alice', got '%s'", name)
		}
		if age != 30 {
			t.Errorf("expected age=30, got %d", age)
		}
	})

	t.Run("NoRowsReturnsError", func(t *testing.T) {
		var name string
		q := &Query{
			sql:    "SELECT name FROM users WHERE id = ?",
			params: []interface{}{999},
			db:     db,
		}
		err := q.Row(&name)
		if err == nil || err.Error() != sql.ErrNoRows.Error() {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("WithContext", func(t *testing.T) {
		ctx := context.Background()
		var name string
		q := &Query{
			sql:    "SELECT name FROM users WHERE id = ?",
			params: []interface{}{1},
			db:     db,
			ctx:    ctx,
		}
		err := q.Row(&name)
		if err != nil {
			t.Fatalf("Row() with context failed: %v", err)
		}
		if name != "Alice" {
			t.Errorf("expected name='Alice', got '%s'", name)
		}
	})

	t.Run("ThreeColumns", func(t *testing.T) {
		var id int
		var name string
		var email string
		q := &Query{
			sql:    "SELECT id, name, email FROM users WHERE id = ?",
			params: []interface{}{2},
			db:     db,
		}
		err := q.Row(&id, &name, &email)
		if err != nil {
			t.Fatalf("Row() failed: %v", err)
		}
		if id != 2 || name != "Bob" || email != "bob@example.com" {
			t.Errorf("expected id=2, name='Bob', email='bob@example.com', got id=%d, name='%s', email='%s'",
				id, name, email)
		}
	})
}

// TestSelectQueryRow tests SelectQuery.Row() method.
func TestSelectQueryRow(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	qb := &QueryBuilder{db: db}

	t.Run("SelectSingleColumn", func(t *testing.T) {
		var name string
		sq := qb.Select("name").From("users").Where("id = ?", 1)
		err := sq.Row(&name)
		if err != nil {
			t.Fatalf("Row() failed: %v", err)
		}
		if name != "Alice" {
			t.Errorf("expected name='Alice', got '%s'", name)
		}
	})

	t.Run("SelectMultipleColumns", func(t *testing.T) {
		var name string
		var email string
		var age int
		sq := qb.Select("name", "email", "age").
			From("users").
			Where("id = ?", 3)
		err := sq.Row(&name, &email, &age)
		if err != nil {
			t.Fatalf("Row() failed: %v", err)
		}
		if name != "Charlie" || email != "charlie@example.com" || age != 35 {
			t.Errorf("unexpected values: name='%s', email='%s', age=%d", name, email, age)
		}
	})

	t.Run("WithWhereClause", func(t *testing.T) {
		var status string
		sq := qb.Select("status").
			From("users").
			Where("name = ?", "Diana")
		err := sq.Row(&status)
		if err != nil {
			t.Fatalf("Row() failed: %v", err)
		}
		if status != "active" {
			t.Errorf("expected status='active', got '%s'", status)
		}
	})
}

// TestQueryColumn tests Query.Column() method with various scenarios.
func TestQueryColumn(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	t.Run("IntegerColumn", func(t *testing.T) {
		var ids []int
		q := &Query{
			sql:    "SELECT id FROM users ORDER BY id",
			params: []interface{}{},
			db:     db,
		}
		err := q.Column(&ids)
		if err != nil {
			t.Fatalf("Column() failed: %v", err)
		}
		if len(ids) != 4 {
			t.Fatalf("expected 4 ids, got %d", len(ids))
		}
		expectedIDs := []int{1, 2, 3, 4}
		for i, expected := range expectedIDs {
			if ids[i] != expected {
				t.Errorf("ids[%d]: expected %d, got %d", i, expected, ids[i])
			}
		}
	})

	t.Run("StringColumn", func(t *testing.T) {
		var names []string
		q := &Query{
			sql:    "SELECT name FROM users WHERE status = ? ORDER BY name",
			params: []interface{}{"active"},
			db:     db,
		}
		err := q.Column(&names)
		if err != nil {
			t.Fatalf("Column() failed: %v", err)
		}
		if len(names) != 3 {
			t.Fatalf("expected 3 names, got %d", len(names))
		}
		expectedNames := []string{"Alice", "Bob", "Diana"}
		for i, expected := range expectedNames {
			if names[i] != expected {
				t.Errorf("names[%d]: expected '%s', got '%s'", i, expected, names[i])
			}
		}
	})

	t.Run("EmptyResult", func(t *testing.T) {
		var ids []int
		q := &Query{
			sql:    "SELECT id FROM users WHERE id > ?",
			params: []interface{}{1000},
			db:     db,
		}
		err := q.Column(&ids)
		if err != nil {
			t.Fatalf("Column() failed: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("expected empty slice, got %d elements", len(ids))
		}
	})

	t.Run("WithContext", func(t *testing.T) {
		ctx := context.Background()
		var emails []string
		q := &Query{
			sql:    "SELECT email FROM users ORDER BY id",
			params: []interface{}{},
			db:     db,
			ctx:    ctx,
		}
		err := q.Column(&emails)
		if err != nil {
			t.Fatalf("Column() with context failed: %v", err)
		}
		if len(emails) != 4 {
			t.Fatalf("expected 4 emails, got %d", len(emails))
		}
	})

	t.Run("InvalidParameterNotPointer", func(t *testing.T) {
		var ids []int
		q := &Query{
			sql:    "SELECT id FROM users",
			params: []interface{}{},
			db:     db,
		}
		err := q.Column(ids)
		if err == nil {
			t.Fatal("expected error for non-pointer parameter, got nil")
		}
	})

	t.Run("InvalidParameterNilPointer", func(t *testing.T) {
		var ids *[]int
		q := &Query{
			sql:    "SELECT id FROM users",
			params: []interface{}{},
			db:     db,
		}
		err := q.Column(ids)
		if err == nil {
			t.Fatal("expected error for nil pointer, got nil")
		}
	})

	t.Run("InvalidParameterNotSlice", func(t *testing.T) {
		var id int
		q := &Query{
			sql:    "SELECT id FROM users",
			params: []interface{}{},
			db:     db,
		}
		err := q.Column(&id)
		if err == nil {
			t.Fatal("expected error for non-slice parameter, got nil")
		}
	})
}

// TestSelectQueryColumn tests SelectQuery.Column() method.
func TestSelectQueryColumn(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	qb := &QueryBuilder{db: db}

	t.Run("SelectIDs", func(t *testing.T) {
		var ids []int
		sq := qb.Select("id").
			From("users").
			Where("status = ?", "active").
			OrderBy("id")
		err := sq.Column(&ids)
		if err != nil {
			t.Fatalf("Column() failed: %v", err)
		}
		if len(ids) != 3 {
			t.Fatalf("expected 3 ids, got %d", len(ids))
		}
		expectedIDs := []int{1, 2, 4}
		for i, expected := range expectedIDs {
			if ids[i] != expected {
				t.Errorf("ids[%d]: expected %d, got %d", i, expected, ids[i])
			}
		}
	})

	t.Run("SelectNames", func(t *testing.T) {
		var names []string
		sq := qb.Select("name").
			From("users").
			OrderBy("name")
		err := sq.Column(&names)
		if err != nil {
			t.Fatalf("Column() failed: %v", err)
		}
		if len(names) != 4 {
			t.Fatalf("expected 4 names, got %d", len(names))
		}
		expectedNames := []string{"Alice", "Bob", "Charlie", "Diana"}
		for i, expected := range expectedNames {
			if names[i] != expected {
				t.Errorf("names[%d]: expected '%s', got '%s'", i, expected, names[i])
			}
		}
	})

	t.Run("WithLimit", func(t *testing.T) {
		var names []string
		sq := qb.Select("name").
			From("users").
			OrderBy("id").
			Limit(2)
		err := sq.Column(&names)
		if err != nil {
			t.Fatalf("Column() failed: %v", err)
		}
		if len(names) != 2 {
			t.Fatalf("expected 2 names, got %d", len(names))
		}
		if names[0] != "Alice" || names[1] != "Bob" {
			t.Errorf("unexpected names: %v", names)
		}
	})
}

// TestRowColumnWithTransaction tests Row() and Column() within a transaction.
func TestRowColumnWithTransaction(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	t.Run("RowInTransaction", func(t *testing.T) {
		tx, err := db.Begin(context.Background())
		if err != nil {
			t.Fatalf("Begin() failed: %v", err)
		}
		defer func() { _ = tx.Rollback() }()

		var count int
		q := &Query{
			sql:    "SELECT COUNT(*) FROM users",
			params: []interface{}{},
			db:     db,
			tx:     tx.tx,
		}
		err = q.Row(&count)
		if err != nil {
			t.Fatalf("Row() in transaction failed: %v", err)
		}
		if count != 4 {
			t.Errorf("expected count=4, got %d", count)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() failed: %v", err)
		}
	})

	t.Run("ColumnInTransaction", func(t *testing.T) {
		tx, err := db.Begin(context.Background())
		if err != nil {
			t.Fatalf("Begin() failed: %v", err)
		}
		defer func() { _ = tx.Rollback() }()

		var names []string
		q := &Query{
			sql:    "SELECT name FROM users ORDER BY name",
			params: []interface{}{},
			db:     db,
			tx:     tx.tx,
		}
		err = q.Column(&names)
		if err != nil {
			t.Fatalf("Column() in transaction failed: %v", err)
		}
		if len(names) != 4 {
			t.Errorf("expected 4 names, got %d", len(names))
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() failed: %v", err)
		}
	})
}

// TestRowColumnInt64 tests Column() with int64 slice.
func TestRowColumnInt64(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	t.Run("Int64Column", func(t *testing.T) {
		var ids []int64
		q := &Query{
			sql:    "SELECT id FROM users ORDER BY id",
			params: []interface{}{},
			db:     db,
		}
		err := q.Column(&ids)
		if err != nil {
			t.Fatalf("Column() failed: %v", err)
		}
		if len(ids) != 4 {
			t.Fatalf("expected 4 ids, got %d", len(ids))
		}
		expectedIDs := []int64{1, 2, 3, 4}
		for i, expected := range expectedIDs {
			if ids[i] != expected {
				t.Errorf("ids[%d]: expected %d, got %d", i, expected, ids[i])
			}
		}
	})
}

// TestRowWithNullValues tests Row() with NULL values handling.
func TestRowWithNullValues(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	// Create table with nullable column.
	_, err := db.sqlDB.Exec(`
		CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create products table: %v", err)
	}

	// Insert data with NULL.
	_, err = db.sqlDB.Exec(`
		INSERT INTO products (id, name, description) VALUES
		(1, 'Product1', 'Description1'),
		(2, 'Product2', NULL)
	`)
	if err != nil {
		t.Fatalf("failed to insert products: %v", err)
	}

	t.Run("RowWithNullString", func(t *testing.T) {
		var name string
		var description sql.NullString
		q := &Query{
			sql:    "SELECT name, description FROM products WHERE id = ?",
			params: []interface{}{2},
			db:     db,
		}
		err := q.Row(&name, &description)
		if err != nil {
			t.Fatalf("Row() failed: %v", err)
		}
		if name != "Product2" {
			t.Errorf("expected name='Product2', got '%s'", name)
		}
		if description.Valid {
			t.Errorf("expected NULL description, got '%s'", description.String)
		}
	})
}

// TestRowColumnErrorCases tests error paths for Row() and Column().
func TestRowColumnErrorCases(t *testing.T) {
	db := setupRowColumnTestDB(t)
	defer func() { _ = db.Close() }()

	t.Run("RowInvalidSQL", func(t *testing.T) {
		var name string
		q := &Query{
			sql:    "SELECT invalid_column FROM users",
			params: []interface{}{},
			db:     db,
		}
		err := q.Row(&name)
		if err == nil {
			t.Fatal("expected error for invalid SQL, got nil")
		}
	})

	t.Run("ColumnInvalidSQL", func(t *testing.T) {
		var names []string
		q := &Query{
			sql:    "SELECT invalid_column FROM users",
			params: []interface{}{},
			db:     db,
		}
		err := q.Column(&names)
		if err == nil {
			t.Fatal("expected error for invalid SQL, got nil")
		}
	})

	t.Run("RowScanMismatch", func(t *testing.T) {
		var id int
		q := &Query{
			sql:    "SELECT name FROM users WHERE id = ?",
			params: []interface{}{1},
			db:     db,
		}
		err := q.Row(&id)
		if err == nil {
			t.Fatal("expected error for type mismatch, got nil")
		}
	})

	t.Run("ColumnScanMismatch", func(t *testing.T) {
		var ids []int
		q := &Query{
			sql:    "SELECT name FROM users",
			params: []interface{}{},
			db:     db,
		}
		err := q.Column(&ids)
		if err == nil {
			t.Fatal("expected error for type mismatch, got nil")
		}
	})
}
