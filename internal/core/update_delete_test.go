package core

import (
	"strings"
	"testing"
)

// TestUpdateQuery_PostgreSQL tests UPDATE SQL generation for PostgreSQL.
func TestUpdateQuery_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"name":  "Alice",
			"email": "alice@example.com",
		}).
		Where("id = ?", 1)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure - columns should be in alphabetical order
	expectedSQL := `UPDATE "users" SET "email" = $1, "name" = $2 WHERE id = $3`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters - should be in alphabetical order, then WHERE params
	want := []interface{}{"alice@example.com", "Alice", 1}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestUpdateQuery_MySQL tests UPDATE SQL generation for MySQL.
func TestUpdateQuery_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"name":  "Bob",
			"email": "bob@example.com",
		}).
		Where("id = ?", 2)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := "UPDATE `users` SET `email` = ?, `name` = ? WHERE id = ?"
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{"bob@example.com", "Bob", 2}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestUpdateQuery_SQLite tests UPDATE SQL generation for SQLite.
func TestUpdateQuery_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"name":   "Charlie",
			"status": "active",
		}).
		Where("id = ?", 3)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := `UPDATE "users" SET "name" = ?, "status" = ? WHERE id = ?`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{"Charlie", "active", 3}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestUpdateQuery_MultipleWhere tests UPDATE with multiple WHERE conditions.
func TestUpdateQuery_MultipleWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"status": "inactive",
		}).
		Where("created_at < ?", "2024-01-01").
		Where("last_login IS NULL")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, `UPDATE "users" SET "status" = $1`) {
		t.Errorf("%q does not contain %q", q.sql, `UPDATE "users" SET "status" = $1`)
	}
	if !strings.Contains(q.sql, "WHERE created_at < $2 AND last_login IS NULL") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE created_at < $2 AND last_login IS NULL")
	}

	// Verify parameters
	want := []interface{}{"inactive", "2024-01-01"}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestUpdateQuery_NoWhere tests UPDATE without WHERE clause (updates all rows).
func TestUpdateQuery_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"last_check": "2025-01-01",
		})

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure - no WHERE clause
	expectedSQL := `UPDATE "users" SET "last_check" = $1`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{"2025-01-01"}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestUpdateQuery_ParameterOrdering tests that parameters are in deterministic order.
func TestUpdateQuery_ParameterOrdering(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"zzz": "last",
			"aaa": "first",
			"mmm": "middle",
		}).
		Where("id = ?", 1)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Parameters should be in sorted key order, then WHERE params
	want := []interface{}{"first", "middle", "last", 1}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}

	// SQL should have columns in alphabetical order
	sql := q.sql
	aIdx := strings.Index(sql, "aaa")
	mIdx := strings.Index(sql, "mmm")
	zIdx := strings.Index(sql, "zzz")

	if aIdx >= mIdx {
		t.Errorf("aaa should come before mmm: aIdx=%d, mIdx=%d", aIdx, mIdx)
	}
	if mIdx >= zIdx {
		t.Errorf("mmm should come before zzz: mIdx=%d, zIdx=%d", mIdx, zIdx)
	}
}

// TestUpdateQuery_ComplexWhere tests UPDATE with complex WHERE conditions.
func TestUpdateQuery_ComplexWhere(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Update("orders").
		Set(map[string]interface{}{
			"status":     "canceled",
			"updated_at": "2025-01-15",
		}).
		Where("status = ?", "pending").
		Where("created_at < ?", "2025-01-01").
		Where("amount < ?", 100)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, "UPDATE `orders` SET `status` = ?, `updated_at` = ?") {
		t.Errorf("%q does not contain %q", q.sql, "UPDATE `orders` SET `status` = ?, `updated_at` = ?")
	}
	if !strings.Contains(q.sql, "WHERE status = ? AND created_at < ? AND amount < ?") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE status = ? AND created_at < ? AND amount < ?")
	}

	// Verify parameters (sorted columns, then WHERE params in order)
	want := []interface{}{"canceled", "2025-01-15", "pending", "2025-01-01", 100}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestDeleteQuery_PostgreSQL tests DELETE SQL generation for PostgreSQL.
func TestDeleteQuery_PostgreSQL(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where("id = ?", 1)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := `DELETE FROM "users" WHERE id = $1`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{1}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestDeleteQuery_MySQL tests DELETE SQL generation for MySQL.
func TestDeleteQuery_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where("id = ?", 2)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := "DELETE FROM `users` WHERE id = ?"
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{2}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestDeleteQuery_SQLite tests DELETE SQL generation for SQLite.
func TestDeleteQuery_SQLite(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where("id = ?", 3)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := `DELETE FROM "users" WHERE id = ?`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{3}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestDeleteQuery_MultipleWhere tests DELETE with multiple WHERE conditions.
func TestDeleteQuery_MultipleWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").
		Where("status = ?", "deleted").
		Where("created_at < ?", "2024-01-01")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := `DELETE FROM "users" WHERE status = $1 AND created_at < $2`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{"deleted", "2024-01-01"}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestDeleteQuery_NoWhere tests DELETE without WHERE clause (deletes all rows).
func TestDeleteQuery_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure - no WHERE clause
	expectedSQL := `DELETE FROM "users"`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify no parameters
	if len(q.params) != 0 {
		t.Errorf("expected empty params, got %d: %v", len(q.params), q.params)
	}
}

// TestDeleteQuery_ComplexWhere tests DELETE with complex WHERE conditions.
func TestDeleteQuery_ComplexWhere(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("logs").
		Where("level = ?", "debug").
		Where("created_at < ?", "2025-01-01").
		Where("user_id IS NULL")

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	if !strings.Contains(q.sql, "DELETE FROM `logs`") {
		t.Errorf("%q does not contain %q", q.sql, "DELETE FROM `logs`")
	}
	if !strings.Contains(q.sql, "WHERE level = ? AND created_at < ? AND user_id IS NULL") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE level = ? AND created_at < ? AND user_id IS NULL")
	}

	// Verify parameters
	want := []interface{}{"debug", "2025-01-01"}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestUpdateQuery_SingleColumn tests UPDATE with single column.
func TestUpdateQuery_SingleColumn(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{
			"last_login": "2025-01-15 10:30:00",
		}).
		Where("id = ?", 42)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := `UPDATE "users" SET "last_login" = $1 WHERE id = $2`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{"2025-01-15 10:30:00", 42}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestDeleteQuery_SingleCondition tests DELETE with single WHERE condition.
func TestDeleteQuery_SingleCondition(t *testing.T) {
	db := mockDB("sqlite")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("sessions").Where("expired = ?", true)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL structure
	expectedSQL := `DELETE FROM "sessions" WHERE expired = ?`
	if q.sql != expectedSQL {
		t.Errorf("got %v, want %v", q.sql, expectedSQL)
	}

	// Verify parameters
	want := []interface{}{true}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}

// TestUpdateQuery_MultiColumnMultiWhere tests UPDATE with many columns and conditions.
func TestUpdateQuery_MultiColumnMultiWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("products").
		Set(map[string]interface{}{
			"price":      99.99,
			"stock":      50,
			"updated_at": "2025-01-15",
			"discount":   10,
		}).
		Where("category = ?", "electronics").
		Where("in_stock = ?", true).
		Where("price < ?", 150.00)

	q := query.Build()
	if q == nil {
		t.Fatal("expected non-nil")
	}

	// Verify SQL contains all columns (alphabetically sorted)
	if !strings.Contains(q.sql, `"discount" = $1`) {
		t.Errorf("%q does not contain %q", q.sql, `"discount" = $1`)
	}
	if !strings.Contains(q.sql, `"price" = $2`) {
		t.Errorf("%q does not contain %q", q.sql, `"price" = $2`)
	}
	if !strings.Contains(q.sql, `"stock" = $3`) {
		t.Errorf("%q does not contain %q", q.sql, `"stock" = $3`)
	}
	if !strings.Contains(q.sql, `"updated_at" = $4`) {
		t.Errorf("%q does not contain %q", q.sql, `"updated_at" = $4`)
	}

	// Verify WHERE clause
	if !strings.Contains(q.sql, "WHERE category = $5 AND in_stock = $6 AND price < $7") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE category = $5 AND in_stock = $6 AND price < $7")
	}

	// Verify parameters (sorted SET columns, then WHERE params in order)
	want := []interface{}{10, 99.99, 50, "2025-01-15", "electronics", true, 150.00}
	if len(q.params) != len(want) {
		t.Errorf("params length: got %d, want %d", len(q.params), len(want))
	} else {
		for i := range want {
			if q.params[i] != want[i] {
				t.Errorf("params[%d]: got %v, want %v", i, q.params[i], want[i])
			}
		}
	}
}
