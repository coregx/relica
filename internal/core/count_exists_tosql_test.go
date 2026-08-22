// Copyright (c) 2025 COREGX. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package core

import (
	"strings"
	"testing"
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
			if sql != tc.wantSQL {
				t.Errorf("got %v, want %v", sql, tc.wantSQL)
			}
			if len(params) != len(tc.wantParams) {
				t.Errorf("got %v, want %v", params, tc.wantParams)
			}
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
			wantSQL:    `SELECT * FROM "users" WHERE "id" = $1`,
			wantParams: []interface{}{1},
		},
		{
			name:       "mysql: WHERE with positional placeholder",
			dialect:    "mysql",
			wantSQL:    "SELECT * FROM `users` WHERE `id` = ?",
			wantParams: []interface{}{1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := mockDB(tc.dialect)
			qb := &QueryBuilder{db: db}

			sql, params := qb.Select().From("users").Where(Eq("id", 1)).ToSQL()
			if sql != tc.wantSQL {
				t.Errorf("got %v, want %v", sql, tc.wantSQL)
			}
			if len(params) != len(tc.wantParams) {
				t.Errorf("got %v, want %v", params, tc.wantParams)
			}
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

	checks := []string{
		`SELECT "id", "name" FROM "users"`,
		`WHERE "status" = $1 AND "age" > $2`,
		`ORDER BY "name" ASC`,
		`LIMIT 10`,
	}
	for _, s := range checks {
		if !strings.Contains(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}
	want := []interface{}{1, 18}
	if len(params) != len(want) {
		t.Errorf("got %v, want %v", params, want)
	} else {
		for i, p := range want {
			if params[i] != p {
				t.Errorf("param[%d]: got %v, want %v", i, params[i], p)
			}
		}
	}
}

func TestSelectQuery_ToSQL_DoesNotExecute(t *testing.T) {
	// ToSQL must not require a real DB connection — mockDB has no sql.DB
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select().From("orders").Where(Eq("user_id", 42)).ToSQL()

	if sql == "" {
		t.Fatal("expected non-empty SQL")
	}
	if len(params) != 1 {
		t.Fatalf("expected length %d, got %d", 1, len(params))
	}
	if params[0] != 42 {
		t.Errorf("got %v, want %v", params[0], 42)
	}
}

func TestSelectQuery_ToSQL_WithColumns(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select("id", "name", "email").From("users").ToSQL()

	want := `SELECT "id", "name", "email" FROM "users"`
	if sql != want {
		t.Errorf("got %v, want %v", sql, want)
	}
	if len(params) != 0 {
		t.Errorf("expected empty params, got %d", len(params))
	}
}

func TestSelectQuery_ToSQL_WithLimit_Offset(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Select().From("posts").Limit(20).Offset(40).ToSQL()

	want := "SELECT * FROM `posts` LIMIT 20 OFFSET 40"
	if sql != want {
		t.Errorf("got %v, want %v", sql, want)
	}
	if len(params) != 0 {
		t.Errorf("expected empty params, got %d", len(params))
	}
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

	checks := []string{`UPDATE "users" SET`, `"status" = $1`, `WHERE "id" = $2`}
	for _, s := range checks {
		if !strings.Contains(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}
	want := []interface{}{2, 1}
	if len(params) != len(want) || params[0] != want[0] || params[1] != want[1] {
		t.Errorf("got %v, want %v", params, want)
	}
}

func TestUpdateQuery_ToSQL_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Update("users").
		Set(map[string]interface{}{"name": "Alice"}).
		Where(Eq("id", 5)).
		ToSQL()

	checks := []string{"UPDATE `users` SET", "`name` = ?", "WHERE `id` = ?"}
	for _, s := range checks {
		if !strings.Contains(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}
	want := []interface{}{"Alice", 5}
	if len(params) != len(want) || params[0] != want[0] || params[1] != want[1] {
		t.Errorf("got %v, want %v", params, want)
	}
}

func TestUpdateQuery_ToSQL_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Update("sessions").
		Set(map[string]interface{}{"active": false}).
		ToSQL()

	if !strings.Contains(sql, `UPDATE "sessions" SET`) {
		t.Errorf("%q does not contain %q", sql, `UPDATE "sessions" SET`)
	}
	if strings.Contains(sql, "WHERE") {
		t.Errorf("%q should not contain %q", sql, "WHERE")
	}
	if len(params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(params))
	}
}

func TestUpdateQuery_ToSQL_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Update("products").
		Set(map[string]interface{}{"price": 99}).
		Where(Eq("id", 10)).
		ToSQL()

	checks := []string{`UPDATE "products" SET`, `"price" = ?`, `WHERE "id" = ?`}
	for _, s := range checks {
		if !strings.Contains(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}
	want := []interface{}{99, 10}
	if len(params) != len(want) || params[0] != want[0] || params[1] != want[1] {
		t.Errorf("got %v, want %v", params, want)
	}
}

// ============================================================================
// ToSQL tests — DeleteQuery
// ============================================================================

func TestDeleteQuery_ToSQL_Postgres(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("users").Where(Eq("id", 1)).ToSQL()

	want := `DELETE FROM "users" WHERE "id" = $1`
	if sql != want {
		t.Errorf("got %v, want %v", sql, want)
	}
	if len(params) != 1 || params[0] != 1 {
		t.Errorf("got %v, want %v", params, []interface{}{1})
	}
}

func TestDeleteQuery_ToSQL_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("sessions").Where(Eq("user_id", 99)).ToSQL()

	want := "DELETE FROM `sessions` WHERE `user_id` = ?"
	if sql != want {
		t.Errorf("got %v, want %v", sql, want)
	}
	if len(params) != 1 || params[0] != 99 {
		t.Errorf("got %v, want %v", params, []interface{}{99})
	}
}

func TestDeleteQuery_ToSQL_SQLite(t *testing.T) {
	db := mockDB("sqlite3")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("logs").Where(In("level", "debug", "trace")).ToSQL()

	if !strings.Contains(sql, `DELETE FROM "logs" WHERE "level" IN (?, ?)`) {
		t.Errorf("%q does not contain %q", sql, `DELETE FROM "logs" WHERE "level" IN (?, ?)`)
	}
	want := []interface{}{"debug", "trace"}
	if len(params) != len(want) || params[0] != want[0] || params[1] != want[1] {
		t.Errorf("got %v, want %v", params, want)
	}
}

func TestDeleteQuery_ToSQL_NoWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("temp_data").ToSQL()

	want := `DELETE FROM "temp_data"`
	if sql != want {
		t.Errorf("got %v, want %v", sql, want)
	}
	if len(params) != 0 {
		t.Errorf("expected empty params, got %d", len(params))
	}
}

func TestDeleteQuery_ToSQL_MultipleConditions(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sql, params := qb.Delete("events").
		Where(Eq("status", "archived")).
		Where(LessThan("created_at", "2020-01-01")).
		ToSQL()

	checks := []string{
		`DELETE FROM "events" WHERE`,
		`"status" = $1`,
		`"created_at" < $2`,
	}
	for _, s := range checks {
		if !strings.Contains(sql, s) {
			t.Errorf("%q does not contain %q", sql, s)
		}
	}
	want := []interface{}{"archived", "2020-01-01"}
	if len(params) != len(want) || params[0] != want[0] || params[1] != want[1] {
		t.Errorf("got %v, want %v", params, want)
	}
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

	want := `SELECT COUNT(*) FROM "users" WHERE "status"=$1`
	if sql != want {
		t.Errorf("got %v, want %v", sql, want)
	}
	if len(params) != 1 || params[0] != 1 {
		t.Errorf("got %v, want %v", params, []interface{}{1})
	}
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

	want := "SELECT COUNT(*) FROM `orders` WHERE `user_id`=?"
	if sql != want {
		t.Errorf("got %v, want %v", sql, want)
	}
	if len(params) != 1 || params[0] != 42 {
		t.Errorf("got %v, want %v", params, []interface{}{42})
	}
}

func TestSelectQuery_Count_IgnoresOriginalColumns(t *testing.T) {
	// Count() must use COUNT(*) regardless of what columns were in Select()
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id", "name", "email").
		From("users").
		Where(Eq("role", "admin"))

	// Verify the original query has columns
	want := []string{"id", "name", "email"}
	if len(sq.columns) != len(want) {
		t.Errorf("got %v, want %v", sq.columns, want)
	}

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
	if !strings.Contains(sql, "SELECT COUNT(*)") {
		t.Errorf("%q does not contain %q", sql, "SELECT COUNT(*)")
	}
	if strings.Contains(sql, `"id"`) {
		t.Errorf("%q should not contain %q", sql, `"id"`)
	}
	if strings.Contains(sql, `"name"`) {
		t.Errorf("%q should not contain %q", sql, `"name"`)
	}
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
	if !strings.Contains(sql, "SELECT COUNT(*)") {
		t.Errorf("%q does not contain %q", sql, "SELECT COUNT(*)")
	}
	if !strings.Contains(sql, `GROUP BY "user_id"`) {
		t.Errorf("%q does not contain %q", sql, `GROUP BY "user_id"`)
	}
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

	want := `SELECT EXISTS(SELECT 1 FROM "users" WHERE "email" = $1)`
	if existsSQL != want {
		t.Errorf("got %v, want %v", existsSQL, want)
	}
	if len(innerParams) != 1 || innerParams[0] != "alice@example.com" {
		t.Errorf("got %v, want %v", innerParams, []interface{}{"alice@example.com"})
	}
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

	want := "SELECT EXISTS(SELECT 1 FROM `users` WHERE `id` = ?)"
	if existsSQL != want {
		t.Errorf("got %v, want %v", existsSQL, want)
	}
	if len(innerParams) != 1 || innerParams[0] != 7 {
		t.Errorf("got %v, want %v", innerParams, []interface{}{7})
	}
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

	if !strings.Contains(existsSQL, "SELECT EXISTS(") {
		t.Errorf("%q does not contain %q", existsSQL, "SELECT EXISTS(")
	}
	if !strings.Contains(existsSQL, "INNER JOIN") {
		t.Errorf("%q does not contain %q", existsSQL, "INNER JOIN")
	}
	if !strings.Contains(existsSQL, `"status" = $1`) {
		t.Errorf("%q does not contain %q", existsSQL, `"status" = $1`)
	}
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

	want := `SELECT EXISTS(SELECT 1 FROM "products" WHERE "sku" = ?)`
	if existsSQL != want {
		t.Errorf("got %v, want %v", existsSQL, want)
	}
	if len(innerParams) != 1 || innerParams[0] != "ABC-123" {
		t.Errorf("got %v, want %v", innerParams, []interface{}{"ABC-123"})
	}
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

	if sql1 != sql2 {
		t.Errorf("got %v, want %v", sql2, sql1)
	}
	if len(params1) != len(params2) {
		t.Errorf("got %v, want %v", params2, params1)
	}
}

func TestUpdateToSQL_Idempotent(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	uq := qb.Update("users").Set(map[string]interface{}{"status": 0}).Where(Eq("id", 1))

	sql1, params1 := uq.ToSQL()
	sql2, params2 := uq.ToSQL()

	if sql1 != sql2 {
		t.Errorf("got %v, want %v", sql2, sql1)
	}
	if len(params1) != len(params2) {
		t.Errorf("got %v, want %v", params2, params1)
	}
}

func TestDeleteToSQL_Idempotent(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	dq := qb.Delete("users").Where(Eq("id", 1))

	sql1, params1 := dq.ToSQL()
	sql2, params2 := dq.ToSQL()

	if sql1 != sql2 {
		t.Errorf("got %v, want %v", sql2, sql1)
	}
	if len(params1) != len(params2) {
		t.Errorf("got %v, want %v", params2, params1)
	}
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

	if !strings.Contains(sql, "WHERE") {
		t.Errorf("%q does not contain %q", sql, "WHERE")
	}
	want := []interface{}{1, "active"}
	if len(params) != len(want) {
		t.Errorf("got %v, want %v", params, want)
	}
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

	if !strings.Contains(sql, `FROM "users"`) {
		t.Errorf("%q does not contain %q", sql, `FROM "users"`)
	}
	if !strings.Contains(sql, "WHERE") {
		t.Errorf("%q does not contain %q", sql, "WHERE")
	}
	want := []interface{}{1, 18}
	if len(params) != len(want) || params[0] != want[0] || params[1] != want[1] {
		t.Errorf("got %v, want %v", params, want)
	}
}

// ============================================================================
// ToSQL tests — Query (Insert)
// ============================================================================

func TestQuery_ToSQL_Insert(t *testing.T) {
	tests := []struct {
		name       string
		dialect    string
		wantSQL    string
		wantParams int
	}{
		{"postgres", "postgres", `INSERT INTO "users"`, 2},
		{"mysql", "mysql", "INSERT INTO `users`", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := mockDB(tc.dialect)
			qb := &QueryBuilder{db: db}

			q := qb.Insert("users", map[string]interface{}{
				"name":  "Alice",
				"email": "alice@example.com",
			})

			sql, params := q.ToSQL()
			if !strings.Contains(sql, tc.wantSQL) {
				t.Errorf("%q does not contain %q", sql, tc.wantSQL)
			}
			if !strings.Contains(sql, "name") {
				t.Errorf("%q does not contain %q", sql, "name")
			}
			if !strings.Contains(sql, "email") {
				t.Errorf("%q does not contain %q", sql, "email")
			}
			if len(params) != tc.wantParams {
				t.Errorf("expected length %d, got %d", tc.wantParams, len(params))
			}
		})
	}
}

func TestQuery_ToSQL_ConsistentWithSQLAndParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Insert("users", map[string]interface{}{
		"name": "Alice",
	})

	sql, params := q.ToSQL()
	if sql != q.SQL() {
		t.Errorf("got %v, want %v", sql, q.SQL())
	}
	qParams := q.Params()
	if len(params) != len(qParams) {
		t.Errorf("got %v, want %v", params, qParams)
	}
}

func TestQuery_ToSQL_NewQuery(t *testing.T) {
	db := mockDB("postgres")

	q := &Query{
		sql:    "SELECT 1",
		params: nil,
		db:     db,
	}

	sql, params := q.ToSQL()
	if sql != "SELECT 1" {
		t.Errorf("got %v, want %v", sql, "SELECT 1")
	}
	if params != nil {
		t.Errorf("expected nil, got %v", params)
	}
}

func TestQuery_ToSQL_EmptyInsert(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	q := qb.Insert("users", nil)

	sql, params := q.ToSQL()
	if sql != "" {
		t.Errorf("expected empty SQL, got %q", sql)
	}
	if params != nil {
		t.Errorf("expected nil, got %v", params)
	}
}
