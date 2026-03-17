// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ToSQL tests — SelectQuery
// ============================================================================

func TestSelectQuery_ToSQL_Simple(t *testing.T) {
	tests := []struct {
		name       string
		dialect    string
		wantSQL    string
		wantParams []interface{}
	}{
		{
			name:       "postgres: no WHERE",
			dialect:    "postgres",
			wantSQL:    `SELECT * FROM "users"`,
			wantParams: nil,
		},
		{
			name:       "mysql: no WHERE",
			dialect:    "mysql",
			wantSQL:    "SELECT * FROM `users`",
			wantParams: nil,
		},
		{
			name:       "sqlite: no WHERE",
			dialect:    "sqlite3",
			wantSQL:    `SELECT * FROM "users"`,
			wantParams: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := mockDB(tc.dialect)
			qb := &QueryBuilder{db: db}

			sql, params := qb.Select().From("users").ToSQL()
			assert.Equal(t, tc.wantSQL, sql)
			assert.Equal(t, tc.wantParams, params)
		})
	}
}

func TestSelectQuery_ToSQL_WithWhere(t *testing.T) {
	tests := []struct {
		name       string
		dialect    string
		wantSQL    string
		wantParams []interface{}
	}{
		{
			name:       "postgres: WHERE with positional placeholder",
			dialect:    "postgres",
			wantSQL:    `SELECT * FROM "users" WHERE "id"=$1`,
			wantParams: []interface{}{1},
		},
		{
			name:       "mysql: WHERE with positional placeholder",
			dialect:    "mysql",
			wantSQL:    "SELECT * FROM `users` WHERE `id`=?",
			wantParams: []interface{}{1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := mockDB(tc.dialect)
			qb := &QueryBuilder{db: db}

			sql, params := qb.Select().From("users").Where(Eq("id", 1)).ToSQL()
			assert.Equal(t, tc.wantSQL, sql)
			assert.Equal(t, tc.wantParams, params)
		})
	}
}

func TestSelectQuery_ToSQL_WithMultipleConditions(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select("id", "name").
		From("users").
		Where(Eq("status", 1)).
		Where(GreaterThan("age", 18)).
		OrderBy("name ASC").
		Limit(10).
		ToSQL()

	assert.Contains(t, sql, `SELECT "id", "name" FROM "users"`)
	assert.Contains(t, sql, `WHERE "status"=$1 AND "age">$2`)
	assert.Contains(t, sql, `ORDER BY "name" ASC`)
	assert.Contains(t, sql, `LIMIT 10`)
	assert.Equal(t, []interface{}{1, 18}, params)
}

func TestSelectQuery_ToSQL_DoesNotExecute(t *testing.T) {
	// ToSQL must not require a real DB connection — mockDB has no sql.DB
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select().From("orders").Where(Eq("user_id", 42)).ToSQL()

	require.NotEmpty(t, sql)
	require.Len(t, params, 1)
	assert.Equal(t, 42, params[0])
}

func TestSelectQuery_ToSQL_WithColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select("id", "name", "email").From("users").ToSQL()

	assert.Equal(t, `SELECT "id", "name", "email" FROM "users"`, sql)
	assert.Empty(t, params)
}

func TestSelectQuery_ToSQL_WithLimit_Offset(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select().From("posts").Limit(20).Offset(40).ToSQL()

	assert.Equal(t, "SELECT * FROM `posts` LIMIT 20 OFFSET 40", sql)
	assert.Empty(t, params)
}

// ============================================================================
// ToSQL tests — UpdateQuery
// ============================================================================

func TestUpdateQuery_ToSQL_Postgres(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Update("users").
		Set(map[string]interface{}{"status": 2}).
		Where(Eq("id", 1)).
		ToSQL()

	assert.Contains(t, sql, `UPDATE "users" SET`)
	assert.Contains(t, sql, `status = $1`)
	assert.Contains(t, sql, `WHERE "id"=$2`)
	assert.Equal(t, []interface{}{2, 1}, params)
}

func TestUpdateQuery_ToSQL_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Update("users").
		Set(map[string]interface{}{"name": "Alice"}).
		Where(Eq("id", 5)).
		ToSQL()

	assert.Contains(t, sql, "UPDATE `users` SET")
	assert.Contains(t, sql, "name = ?")
	assert.Contains(t, sql, "WHERE `id`=?")
	assert.Equal(t, []interface{}{"Alice", 5}, params)
}

func TestUpdateQuery_ToSQL_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Update("sessions").
		Set(map[string]interface{}{"active": false}).
		ToSQL()

	assert.Contains(t, sql, `UPDATE "sessions" SET`)
	assert.NotContains(t, sql, "WHERE")
	assert.Len(t, params, 1)
}

func TestUpdateQuery_ToSQL_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Update("products").
		Set(map[string]interface{}{"price": 99}).
		Where(Eq("id", 10)).
		ToSQL()

	assert.Contains(t, sql, `UPDATE "products" SET`)
	assert.Contains(t, sql, "price = ?")
	assert.Contains(t, sql, `WHERE "id"=?`)
	assert.Equal(t, []interface{}{99, 10}, params)
}

// ============================================================================
// ToSQL tests — DeleteQuery
// ============================================================================

func TestDeleteQuery_ToSQL_Postgres(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("users").Where(Eq("id", 1)).ToSQL()

	assert.Equal(t, `DELETE FROM "users" WHERE "id"=$1`, sql)
	assert.Equal(t, []interface{}{1}, params)
}

func TestDeleteQuery_ToSQL_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("sessions").Where(Eq("user_id", 99)).ToSQL()

	assert.Equal(t, "DELETE FROM `sessions` WHERE `user_id`=?", sql)
	assert.Equal(t, []interface{}{99}, params)
}

func TestDeleteQuery_ToSQL_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("logs").Where(In("level", "debug", "trace")).ToSQL()

	assert.Contains(t, sql, `DELETE FROM "logs" WHERE "level" IN (?, ?)`)
	assert.Equal(t, []interface{}{"debug", "trace"}, params)
}

func TestDeleteQuery_ToSQL_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("temp_data").ToSQL()

	assert.Equal(t, `DELETE FROM "temp_data"`, sql)
	assert.Empty(t, params)
}

func TestDeleteQuery_ToSQL_MultipleConditions(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("events").
		Where(Eq("status", "archived")).
		Where(LessThan("created_at", "2020-01-01")).
		ToSQL()

	assert.Contains(t, sql, `DELETE FROM "events" WHERE`)
	assert.Contains(t, sql, `"status"=$1`)
	assert.Contains(t, sql, `"created_at"<$2`)
	assert.Equal(t, []interface{}{"archived", "2020-01-01"}, params)
}

// ============================================================================
// Count — SQL generation tests
// ============================================================================

func TestSelectQuery_Count_SQL_Postgres(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	countQuery := &SelectQuery{
		builder: qb,
		columns: []string{"COUNT(*)"},
		fromSrc: &fromSource{isSubquery: false, table: "users"},
		table:   "users",
		where:   []string{`"status"=?`},
		params:  []interface{}{1},
	}

	sql, params := countQuery.buildSQL(db.dialect)

	assert.Equal(t, `SELECT COUNT(*) FROM "users" WHERE "status"=$1`, sql)
	assert.Equal(t, []interface{}{1}, params)
}

func TestSelectQuery_Count_SQL_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	countQuery := &SelectQuery{
		builder: qb,
		columns: []string{"COUNT(*)"},
		fromSrc: &fromSource{isSubquery: false, table: "orders"},
		table:   "orders",
		where:   []string{"`user_id`=?"},
		params:  []interface{}{42},
	}

	sql, params := countQuery.buildSQL(db.dialect)

	assert.Equal(t, "SELECT COUNT(*) FROM `orders` WHERE `user_id`=?", sql)
	assert.Equal(t, []interface{}{42}, params)
}

func TestSelectQuery_Count_IgnoresOriginalColumns(t *testing.T) {
	// Count() must use COUNT(*) regardless of what columns were in Select()
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id", "name", "email").
		From("users").
		Where(Eq("role", "admin"))

	// Verify the original query has columns
	assert.Equal(t, []string{"id", "name", "email"}, sq.columns)

	// Build the count query that Count() would construct — same as internal logic
	countQuery := &SelectQuery{
		builder:       sq.builder,
		columns:       []string{"COUNT(*)"},
		fromSrc:       sq.fromSrc,
		table:         sq.table,
		joins:         sq.joins,
		where:         sq.where,
		params:        sq.params,
		groupBy:       sq.groupBy,
		havingClauses: sq.havingClauses,
		ctx:           sq.ctx,
	}

	sql, _ := countQuery.buildSQL(db.dialect)
	assert.Contains(t, sql, "SELECT COUNT(*)")
	assert.NotContains(t, sql, `"id"`)
	assert.NotContains(t, sql, `"name"`)
}

func TestSelectQuery_Count_SQL_WithGroupBy(t *testing.T) {
	// COUNT(*) with GROUP BY should work correctly
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	countQuery := &SelectQuery{
		builder: qb,
		columns: []string{"COUNT(*)"},
		fromSrc: &fromSource{isSubquery: false, table: "orders"},
		table:   "orders",
		groupBy: []string{"user_id"},
	}

	sql, _ := countQuery.buildSQL(db.dialect)
	assert.Contains(t, sql, "SELECT COUNT(*)")
	assert.Contains(t, sql, `GROUP BY "user_id"`)
}

// ============================================================================
// Exists — SQL generation tests (white-box)
// ============================================================================

func TestSelectQuery_Exists_SQL_Postgres(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select().From("users").Where(Eq("email", "alice@example.com"))

	// Use selectExprs with raw "1" (same as Exists() implementation)
	innerQuery := &SelectQuery{
		builder:     sq.builder,
		selectExprs: []RawExp{{SQL: "1"}},
		fromSrc:     sq.fromSrc,
		table:       sq.table,
		where:       sq.where,
		params:      sq.params,
	}

	innerSQL, innerParams := innerQuery.buildSQL(db.dialect)
	existsSQL := "SELECT EXISTS(" + innerSQL + ")"

	assert.Equal(t, `SELECT EXISTS(SELECT 1 FROM "users" WHERE "email"=$1)`, existsSQL)
	assert.Equal(t, []interface{}{"alice@example.com"}, innerParams)
}

func TestSelectQuery_Exists_SQL_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sq := qb.Select().From("users").Where(Eq("id", 7))

	innerQuery := &SelectQuery{
		builder:     sq.builder,
		selectExprs: []RawExp{{SQL: "1"}},
		fromSrc:     sq.fromSrc,
		table:       sq.table,
		where:       sq.where,
		params:      sq.params,
	}

	innerSQL, innerParams := innerQuery.buildSQL(db.dialect)
	existsSQL := "SELECT EXISTS(" + innerSQL + ")"

	assert.Equal(t, "SELECT EXISTS(SELECT 1 FROM `users` WHERE `id`=?)", existsSQL)
	assert.Equal(t, []interface{}{7}, innerParams)
}

func TestSelectQuery_Exists_SQL_WithJoin(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select().
		From("users u").
		InnerJoin("orders o", "o.user_id = u.id").
		Where(Eq("status", "active"))

	innerQuery := &SelectQuery{
		builder:     sq.builder,
		selectExprs: []RawExp{{SQL: "1"}},
		fromSrc:     sq.fromSrc,
		table:       sq.table,
		joins:       sq.joins,
		where:       sq.where,
		params:      sq.params,
	}

	innerSQL, _ := innerQuery.buildSQL(db.dialect)
	existsSQL := "SELECT EXISTS(" + innerSQL + ")"

	assert.Contains(t, existsSQL, "SELECT EXISTS(")
	assert.Contains(t, existsSQL, "INNER JOIN")
	assert.Contains(t, existsSQL, `"status"=$1`)
}

func TestSelectQuery_Exists_SQL_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sq := qb.Select().From("products").Where(Eq("sku", "ABC-123"))

	innerQuery := &SelectQuery{
		builder:     sq.builder,
		selectExprs: []RawExp{{SQL: "1"}},
		fromSrc:     sq.fromSrc,
		table:       sq.table,
		where:       sq.where,
		params:      sq.params,
	}

	innerSQL, innerParams := innerQuery.buildSQL(db.dialect)
	existsSQL := "SELECT EXISTS(" + innerSQL + ")"

	assert.Equal(t, `SELECT EXISTS(SELECT 1 FROM "products" WHERE "sku"=?)`, existsSQL)
	assert.Equal(t, []interface{}{"ABC-123"}, innerParams)
}

// ============================================================================
// ToSQL consistency — same result on multiple calls
// ============================================================================

func TestToSQL_Idempotent(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id").From("users").Where(Eq("status", 1))

	sql1, params1 := sq.ToSQL()
	sql2, params2 := sq.ToSQL()

	assert.Equal(t, sql1, sql2)
	assert.Equal(t, params1, params2)
}

func TestUpdateToSQL_Idempotent(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	uq := qb.Update("users").Set(map[string]interface{}{"status": 0}).Where(Eq("id", 1))

	sql1, params1 := uq.ToSQL()
	sql2, params2 := uq.ToSQL()

	assert.Equal(t, sql1, sql2)
	assert.Equal(t, params1, params2)
}

func TestDeleteToSQL_Idempotent(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	dq := qb.Delete("users").Where(Eq("id", 1))

	sql1, params1 := dq.ToSQL()
	sql2, params2 := dq.ToSQL()

	assert.Equal(t, sql1, sql2)
	assert.Equal(t, params1, params2)
}

// ============================================================================
// ToSQL — Named placeholders support
// ============================================================================

func TestSelectQuery_ToSQL_NamedPlaceholders(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select().From("users").
		Where("id = {:id} AND status = {:status}", Params{"id": 1, "status": "active"}).
		ToSQL()

	assert.Contains(t, sql, "WHERE")
	assert.Equal(t, []interface{}{1, "active"}, params)
}

// ============================================================================
// ToSQL — Expression API support
// ============================================================================

func TestSelectQuery_ToSQL_ExpressionAPI(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select().From("users").
		Where(And(
			Eq("status", 1),
			GreaterThan("age", 18),
		)).
		ToSQL()

	assert.Contains(t, sql, `FROM "users"`)
	assert.Contains(t, sql, "WHERE")
	assert.Equal(t, []interface{}{1, 18}, params)
}
