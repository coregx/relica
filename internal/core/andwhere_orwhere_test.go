package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelectQuery_AndWhere tests AndWhere() with string conditions.
func TestSelectQuery_AndWhere(t *testing.T) {
	tests := []struct {
		name        string
		dialect     string
		buildQuery  func(*QueryBuilder) *SelectQuery
		expectedSQL string
		expectedLen int
	}{
		{
			name:    "AndWhere with single condition - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").AndWhere("status = ?", 1)
			},
			expectedSQL: `SELECT * FROM "users" WHERE status = $1`,
			expectedLen: 1,
		},
		{
			name:    "AndWhere after Where - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					AndWhere("age > ?", 18)
			},
			expectedSQL: `SELECT * FROM "users" WHERE status = $1 AND age > $2`,
			expectedLen: 2,
		},
		{
			name:    "Multiple AndWhere calls - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					AndWhere("status = ?", 1).
					AndWhere("age > ?", 18).
					AndWhere("active = ?", true)
			},
			expectedSQL: `SELECT * FROM "users" WHERE status = $1 AND age > $2 AND active = $3`,
			expectedLen: 3,
		},
		{
			name:    "AndWhere with Expression - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					AndWhere(GreaterThan("age", 18))
			},
			expectedSQL: `SELECT * FROM "users" WHERE status = $1 AND "age">$2`,
			expectedLen: 2,
		},
		{
			name:    "AndWhere - MySQL",
			dialect: "mysql",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					AndWhere("age > ?", 18)
			},
			expectedSQL: "SELECT * FROM `users` WHERE status = ? AND age > ?",
			expectedLen: 2,
		},
		{
			name:    "AndWhere - SQLite",
			dialect: "sqlite",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					AndWhere("age > ?", 18)
			},
			expectedSQL: `SELECT * FROM "users" WHERE status = ? AND age > ?`,
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			query := tt.buildQuery(qb)
			q := query.Build()

			require.NotNil(t, q)
			assert.Equal(t, tt.expectedSQL, q.sql)
			assert.Len(t, q.params, tt.expectedLen)
		})
	}
}

// TestSelectQuery_OrWhere tests OrWhere() with string conditions.
func TestSelectQuery_OrWhere(t *testing.T) {
	tests := []struct {
		name        string
		dialect     string
		buildQuery  func(*QueryBuilder) *SelectQuery
		expectedSQL string
		expectedLen int
	}{
		{
			name:    "OrWhere with single condition - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").OrWhere("status = ?", 1)
			},
			expectedSQL: `SELECT * FROM "users" WHERE status = $1`,
			expectedLen: 1,
		},
		{
			name:    "OrWhere after Where - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					OrWhere("role = ?", "admin")
			},
			expectedSQL: `SELECT * FROM "users" WHERE (status = $1) OR (role = $2)`,
			expectedLen: 2,
		},
		{
			name:    "Multiple OrWhere calls - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					OrWhere("role = ?", "admin").
					OrWhere("priority = ?", "high")
			},
			expectedSQL: `SELECT * FROM "users" WHERE ((status = $1) OR (role = $2)) OR (priority = $3)`,
			expectedLen: 3,
		},
		{
			name:    "OrWhere with Expression - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					OrWhere(Eq("role", "admin"))
			},
			expectedSQL: `SELECT * FROM "users" WHERE (status = $1) OR ("role"=$2)`,
			expectedLen: 2,
		},
		{
			name:    "OrWhere - MySQL",
			dialect: "mysql",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					OrWhere("role = ?", "admin")
			},
			expectedSQL: "SELECT * FROM `users` WHERE (status = ?) OR (role = ?)",
			expectedLen: 2,
		},
		{
			name:    "OrWhere - SQLite",
			dialect: "sqlite",
			buildQuery: func(qb *QueryBuilder) *SelectQuery {
				return qb.Select("*").From("users").
					Where("status = ?", 1).
					OrWhere("role = ?", "admin")
			},
			expectedSQL: `SELECT * FROM "users" WHERE (status = ?) OR (role = ?)`,
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			query := tt.buildQuery(qb)
			q := query.Build()

			require.NotNil(t, q)
			assert.Equal(t, tt.expectedSQL, q.sql)
			assert.Len(t, q.params, tt.expectedLen)
		})
	}
}

// TestSelectQuery_AndWhere_OrWhere_Combined tests mixing AndWhere and OrWhere.
func TestSelectQuery_AndWhere_OrWhere_Combined(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// WHERE status = 1 AND age > 18 OR role = admin
	// Should produce: (status = 1 AND age > 18) OR (role = admin)
	query := qb.Select("*").From("users").
		Where("status = ?", 1).
		AndWhere("age > ?", 18).
		OrWhere("role = ?", "admin")

	q := query.Build()
	require.NotNil(t, q)
	assert.Equal(t, `SELECT * FROM "users" WHERE (status = $1 AND age > $2) OR (role = $3)`, q.sql)
	assert.Len(t, q.params, 3)
	assert.Equal(t, []interface{}{1, 18, "admin"}, q.params)
}

// TestUpdateQuery_AndWhere tests AndWhere() for UPDATE queries.
func TestUpdateQuery_AndWhere(t *testing.T) {
	tests := []struct {
		name        string
		dialect     string
		buildQuery  func(*QueryBuilder) *UpdateQuery
		expectedSQL string
		expectedLen int
	}{
		{
			name:    "AndWhere with single condition - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *UpdateQuery {
				return qb.Update("users").
					Set(map[string]interface{}{"status": 2}).
					AndWhere("id > ?", 100)
			},
			expectedSQL: `UPDATE "users" SET status = $1 WHERE id > $2`,
			expectedLen: 2,
		},
		{
			name:    "AndWhere after Where - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *UpdateQuery {
				return qb.Update("users").
					Set(map[string]interface{}{"status": 2}).
					Where("id > ?", 100).
					AndWhere("active = ?", true)
			},
			expectedSQL: `UPDATE "users" SET status = $1 WHERE id > $2 AND active = $3`,
			expectedLen: 3,
		},
		{
			name:    "AndWhere with Expression - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *UpdateQuery {
				return qb.Update("users").
					Set(map[string]interface{}{"status": 2}).
					Where("id > ?", 100).
					AndWhere(Eq("active", true))
			},
			expectedSQL: `UPDATE "users" SET status = $1 WHERE id > $2 AND "active"=$3`,
			expectedLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			query := tt.buildQuery(qb)
			q := query.Build()

			require.NotNil(t, q)
			assert.Equal(t, tt.expectedSQL, q.sql)
			assert.Len(t, q.params, tt.expectedLen)
		})
	}
}

// TestUpdateQuery_OrWhere tests OrWhere() for UPDATE queries.
func TestUpdateQuery_OrWhere(t *testing.T) {
	tests := []struct {
		name        string
		dialect     string
		buildQuery  func(*QueryBuilder) *UpdateQuery
		expectedSQL string
		expectedLen int
	}{
		{
			name:    "OrWhere with single condition - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *UpdateQuery {
				return qb.Update("users").
					Set(map[string]interface{}{"status": 0}).
					OrWhere("banned = ?", true)
			},
			expectedSQL: `UPDATE "users" SET status = $1 WHERE banned = $2`,
			expectedLen: 2,
		},
		{
			name:    "OrWhere after Where - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *UpdateQuery {
				return qb.Update("users").
					Set(map[string]interface{}{"status": 0}).
					Where("banned = ?", true).
					OrWhere("deleted = ?", true)
			},
			expectedSQL: `UPDATE "users" SET status = $1 WHERE (banned = $2) OR (deleted = $3)`,
			expectedLen: 3,
		},
		{
			name:    "OrWhere with Expression - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *UpdateQuery {
				return qb.Update("users").
					Set(map[string]interface{}{"status": 0}).
					Where("banned = ?", true).
					OrWhere(Eq("deleted", true))
			},
			expectedSQL: `UPDATE "users" SET status = $1 WHERE (banned = $2) OR ("deleted"=$3)`,
			expectedLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			query := tt.buildQuery(qb)
			q := query.Build()

			require.NotNil(t, q)
			assert.Equal(t, tt.expectedSQL, q.sql)
			assert.Len(t, q.params, tt.expectedLen)
		})
	}
}

// TestDeleteQuery_AndWhere tests AndWhere() for DELETE queries.
func TestDeleteQuery_AndWhere(t *testing.T) {
	tests := []struct {
		name        string
		dialect     string
		buildQuery  func(*QueryBuilder) *DeleteQuery
		expectedSQL string
		expectedLen int
	}{
		{
			name:    "AndWhere with single condition - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *DeleteQuery {
				return qb.Delete("users").AndWhere("status = ?", 0)
			},
			expectedSQL: `DELETE FROM "users" WHERE status = $1`,
			expectedLen: 1,
		},
		{
			name:    "AndWhere after Where - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *DeleteQuery {
				return qb.Delete("users").
					Where("status = ?", 0).
					AndWhere("created_at < ?", "2020-01-01")
			},
			expectedSQL: `DELETE FROM "users" WHERE status = $1 AND created_at < $2`,
			expectedLen: 2,
		},
		{
			name:    "AndWhere with Expression - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *DeleteQuery {
				return qb.Delete("users").
					Where("status = ?", 0).
					AndWhere(LessThan("created_at", "2020-01-01"))
			},
			expectedSQL: `DELETE FROM "users" WHERE status = $1 AND "created_at"<$2`,
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			query := tt.buildQuery(qb)
			q := query.Build()

			require.NotNil(t, q)
			assert.Equal(t, tt.expectedSQL, q.sql)
			assert.Len(t, q.params, tt.expectedLen)
		})
	}
}

// TestDeleteQuery_OrWhere tests OrWhere() for DELETE queries.
func TestDeleteQuery_OrWhere(t *testing.T) {
	tests := []struct {
		name        string
		dialect     string
		buildQuery  func(*QueryBuilder) *DeleteQuery
		expectedSQL string
		expectedLen int
	}{
		{
			name:    "OrWhere with single condition - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *DeleteQuery {
				return qb.Delete("users").OrWhere("banned = ?", true)
			},
			expectedSQL: `DELETE FROM "users" WHERE banned = $1`,
			expectedLen: 1,
		},
		{
			name:    "OrWhere after Where - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *DeleteQuery {
				return qb.Delete("users").
					Where("banned = ?", true).
					OrWhere("deleted = ?", true)
			},
			expectedSQL: `DELETE FROM "users" WHERE (banned = $1) OR (deleted = $2)`,
			expectedLen: 2,
		},
		{
			name:    "OrWhere with Expression - PostgreSQL",
			dialect: "postgres",
			buildQuery: func(qb *QueryBuilder) *DeleteQuery {
				return qb.Delete("users").
					Where("banned = ?", true).
					OrWhere(Eq("deleted", true))
			},
			expectedSQL: `DELETE FROM "users" WHERE (banned = $1) OR ("deleted"=$2)`,
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := mockDB(tt.dialect)
			qb := &QueryBuilder{db: db}
			query := tt.buildQuery(qb)
			q := query.Build()

			require.NotNil(t, q)
			assert.Equal(t, tt.expectedSQL, q.sql)
			assert.Len(t, q.params, tt.expectedLen)
		})
	}
}

// TestAndWhere_OrWhere_EmptyExpression tests that empty expressions are handled gracefully.
func TestAndWhere_OrWhere_EmptyExpression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Empty expression in OrWhere should be ignored.
	query := qb.Select("*").From("users").
		Where("status = ?", 1).
		OrWhere(HashExp{}) // Empty expression

	q := query.Build()
	require.NotNil(t, q)
	// Should only have the first WHERE condition.
	assert.Equal(t, `SELECT * FROM "users" WHERE status = $1`, q.sql)
	assert.Len(t, q.params, 1)
}

// TestAndWhere_OrWhere_Panic tests that invalid arguments panic.
func TestAndWhere_OrWhere_Panic(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	t.Run("AndWhere with invalid type", func(t *testing.T) {
		assert.Panics(t, func() {
			qb.Select().From("users").AndWhere(123)
		})
	})

	t.Run("OrWhere with invalid type", func(t *testing.T) {
		assert.Panics(t, func() {
			qb.Select().From("users").OrWhere([]string{"bad"})
		})
	})

	t.Run("UpdateQuery AndWhere with invalid type", func(t *testing.T) {
		assert.Panics(t, func() {
			qb.Update("users").Set(map[string]interface{}{"x": 1}).AndWhere(123)
		})
	})

	t.Run("UpdateQuery OrWhere with invalid type", func(t *testing.T) {
		assert.Panics(t, func() {
			qb.Update("users").Set(map[string]interface{}{"x": 1}).OrWhere(map[string]int{"bad": 1})
		})
	})

	t.Run("DeleteQuery AndWhere with invalid type", func(t *testing.T) {
		assert.Panics(t, func() {
			qb.Delete("users").AndWhere(123)
		})
	})

	t.Run("DeleteQuery OrWhere with invalid type", func(t *testing.T) {
		assert.Panics(t, func() {
			qb.Delete("users").OrWhere(map[string]int{"bad": 1})
		})
	})
}

// TestAndWhere_OrWhere_ComplexScenario tests a realistic complex query.
func TestAndWhere_OrWhere_ComplexScenario(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// Build dynamic query: users where (status=1 AND age>18 AND city='NYC') OR (role='admin')
	query := qb.Select("*").From("users").
		Where("status = ?", 1).
		AndWhere("age > ?", 18).
		AndWhere("city = ?", "NYC").
		OrWhere("role = ?", "admin")

	q := query.Build()
	require.NotNil(t, q)

	expectedSQL := `SELECT * FROM "users" WHERE (status = $1 AND age > $2 AND city = $3) OR (role = $4)`
	assert.Equal(t, expectedSQL, q.sql)
	assert.Equal(t, []interface{}{1, 18, "NYC", "admin"}, q.params)
}

// TestAndWhere_OrWhere_HashExp tests using HashExp with AndWhere/OrWhere.
func TestAndWhere_OrWhere_HashExp(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select("*").From("users").
		Where(HashExp{"status": 1}).
		AndWhere(HashExp{"age": 18, "city": "NYC"}).
		OrWhere(HashExp{"role": "admin"})

	q := query.Build()
	require.NotNil(t, q)

	// HashExp with multiple keys uses AND internally and keys are sorted alphabetically.
	// WHERE status=1 AND (age=18 AND city=NYC) OR (role=admin)
	// After sorting: WHERE status=1 AND (age=18 AND city=NYC) OR (role=admin)
	// With parentheses: (status=1 AND age=18 AND city=NYC) OR (role=admin)
	assert.Contains(t, q.sql, `WHERE (`)
	assert.Contains(t, q.sql, `"status"=$1`)
	assert.Contains(t, q.sql, `"age"=$2`)
	assert.Contains(t, q.sql, `"city"=$3`)
	assert.Contains(t, q.sql, `OR ("role"=$4)`)
	assert.Len(t, q.params, 4)
}
