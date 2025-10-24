package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSelectQuery_Where_Expression tests SelectQuery.Where() with Expression API
func TestSelectQuery_Where_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().From("users").Where(And(
		Eq("status", 1),
		GreaterThan("age", 18),
	))

	q := query.Build()
	assert.Contains(t, q.sql, `SELECT * FROM "users" WHERE`)
	assert.Len(t, q.params, 2)
	assert.Equal(t, 1, q.params[0])
	assert.Equal(t, 18, q.params[1])
}

// TestUpdateQuery_Where_Expression tests UpdateQuery.Where() with Expression API
func TestUpdateQuery_Where_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{"status": 2}).
		Where(Eq("id", 123))

	q := query.Build()
	assert.Contains(t, q.sql, `UPDATE "users" SET`)
	assert.Contains(t, q.sql, `WHERE`)
	assert.Len(t, q.params, 2)
	assert.Equal(t, 2, q.params[0])   // SET value
	assert.Equal(t, 123, q.params[1]) // WHERE value
}

// TestDeleteQuery_Where_Expression tests DeleteQuery.Where() with Expression API
func TestDeleteQuery_Where_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where(In("status", 0, 1, 2))

	q := query.Build()
	assert.Contains(t, q.sql, `DELETE FROM "users" WHERE`)
	assert.Len(t, q.params, 3)
	assert.Equal(t, []interface{}{0, 1, 2}, q.params)
}

// TestWhere_BackwardCompatibility tests that string-based Where() still works
func TestWhere_BackwardCompatibility(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// SELECT with string Where
	sq := qb.Select().From("users").Where("status = ?", 1)
	qSelect := sq.Build()
	assert.Contains(t, qSelect.sql, "WHERE status = $1")

	// UPDATE with string Where
	uq := qb.Update("users").Set(map[string]interface{}{"name": "Alice"}).Where("id = ?", 123)
	qUpdate := uq.Build()
	assert.Contains(t, qUpdate.sql, "WHERE id = $2")

	// DELETE with string Where
	dq := qb.Delete("users").Where("id = ?", 456)
	qDelete := dq.Build()
	assert.Contains(t, qDelete.sql, "WHERE id = $1")
}

// TestWhere_Panic tests that invalid Where() arguments panic
func TestWhere_Panic(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	assert.Panics(t, func() {
		qb.Select().From("users").Where(123) // int is not string or Expression
	})

	assert.Panics(t, func() {
		qb.Update("users").Set(map[string]interface{}{"x": 1}).Where([]string{"bad"})
	})

	assert.Panics(t, func() {
		qb.Delete("users").Where(map[string]int{"bad": 1})
	})
}
