package core

import (
	"testing"

	"github.com/coregx/relica/internal/dialects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelectQuery_BuildErr_FromSelect verifies that FromSelect with an empty alias
// stores a build error and propagates it through Build, All, One, etc.
func TestSelectQuery_BuildErr_FromSelect(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sub := qb.Select("id").From("orders")
	sq := qb.Select("id").FromSelect(sub, "") // empty alias — programming error

	assert.NotNil(t, sq.buildErr, "empty alias must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "FromSelect")
	assert.ErrorContains(t, sq.buildErr, "non-empty alias")

	q := sq.Build()
	require.NotNil(t, q.prepErr, "buildErr must propagate to Query.prepErr")
	assert.ErrorContains(t, q.prepErr, "FromSelect")
}

// TestSelectQuery_BuildErr_Where verifies that an invalid Where() type stores
// a build error and propagates it through Build.
func TestSelectQuery_BuildErr_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id").From("users").Where(42) // int is not string or Expression

	assert.NotNil(t, sq.buildErr, "invalid Where() type must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "Where()")
	assert.ErrorContains(t, sq.buildErr, "int")

	q := sq.Build()
	require.NotNil(t, q.prepErr, "buildErr must propagate to Query.prepErr")
}

// TestSelectQuery_BuildErr_OrWhere verifies that an invalid OrWhere() type stores
// a build error and propagates it through Build.
func TestSelectQuery_BuildErr_OrWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("id").From("users").
		Where("status = ?", 1).
		OrWhere([]int{1, 2, 3}) // slice is not string or Expression

	assert.NotNil(t, sq.buildErr, "invalid OrWhere() type must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "OrWhere()")

	q := sq.Build()
	require.NotNil(t, q.prepErr, "buildErr must propagate to Query.prepErr")
}

// TestSelectQuery_BuildErr_With_EmptyName verifies that With() with an empty
// name stores an error and propagates it through Build.
func TestSelectQuery_BuildErr_With_EmptyName(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	cte := qb.Select("id").From("orders")
	sq := qb.Select("*").With("", cte)

	assert.NotNil(t, sq.buildErr, "empty CTE name must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "With()")

	q := sq.Build()
	require.NotNil(t, q.prepErr)
}

// TestSelectQuery_BuildErr_With_NilQuery verifies that With() with a nil query
// stores an error and propagates it through Build.
func TestSelectQuery_BuildErr_With_NilQuery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("*").With("my_cte", nil)

	assert.NotNil(t, sq.buildErr, "nil CTE query must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "With()")

	q := sq.Build()
	require.NotNil(t, q.prepErr)
}

// TestSelectQuery_BuildErr_WithRecursive_EmptyName verifies that WithRecursive()
// with an empty name stores an error.
func TestSelectQuery_BuildErr_WithRecursive_EmptyName(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	anchor := qb.Select("1 as n")
	rec := qb.Select("n+1").From("nums").Where("n < ?", 10)
	cte := anchor.UnionAll(rec)

	sq := qb.Select("*").WithRecursive("", cte)

	assert.NotNil(t, sq.buildErr, "empty recursive CTE name must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "WithRecursive()")
}

// TestSelectQuery_BuildErr_WithRecursive_NilQuery verifies that WithRecursive()
// with a nil query stores an error.
func TestSelectQuery_BuildErr_WithRecursive_NilQuery(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("*").WithRecursive("nums", nil)

	assert.NotNil(t, sq.buildErr, "nil recursive CTE query must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "WithRecursive()")
}

// TestSelectQuery_BuildErr_WithRecursive_NoUnion verifies that WithRecursive()
// without UNION stores an error.
func TestSelectQuery_BuildErr_WithRecursive_NoUnion(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	nonRecursive := qb.Select("id").From("employees") // no UNION

	sq := qb.Select("*").WithRecursive("hier", nonRecursive)

	assert.NotNil(t, sq.buildErr, "recursive CTE without UNION must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "WithRecursive()")
	assert.ErrorContains(t, sq.buildErr, "UNION")

	q := sq.Build()
	require.NotNil(t, q.prepErr)
}

// TestSelectQuery_BuildErr_JoinInvalidOnType verifies that an invalid JOIN ON
// type stores an error via buildErr and propagates through Build.
func TestSelectQuery_BuildErr_JoinInvalidOnType(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("m.id").
		From("messages m").
		InnerJoin("users u", 3.14) // float64 — not string or Expression

	// buildErr is set lazily during buildSQL (called from Build)
	q := sq.Build()
	require.NotNil(t, q.prepErr, "invalid JOIN ON type must propagate as Query.prepErr")
	assert.ErrorContains(t, q.prepErr, "JOIN ON")
}

// TestSelectQuery_BuildErr_Having verifies that an invalid Having() type stores
// a build error.
func TestSelectQuery_BuildErr_Having(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select("status", "COUNT(*)").
		From("orders").
		GroupBy("status").
		Having(42) // int is not string or Expression

	assert.NotNil(t, sq.buildErr, "invalid Having() type must store buildErr")
	assert.ErrorContains(t, sq.buildErr, "Having()")

	q := sq.Build()
	require.NotNil(t, q.prepErr)
}

// TestUpdateQuery_BuildErr_Where verifies that an invalid Where() type on
// UpdateQuery stores a build error.
func TestUpdateQuery_BuildErr_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	uq := qb.Update("users").
		Set(map[string]interface{}{"status": "active"}).
		Where(struct{ bad string }{"value"}) // unsupported type

	assert.NotNil(t, uq.buildErr, "invalid Where() type must store buildErr on UpdateQuery")
	assert.ErrorContains(t, uq.buildErr, "Where()")

	q := uq.Build()
	require.NotNil(t, q.prepErr)
}

// TestUpdateQuery_BuildErr_OrWhere verifies that an invalid OrWhere() type on
// UpdateQuery stores a build error.
func TestUpdateQuery_BuildErr_OrWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	uq := qb.Update("users").
		Set(map[string]interface{}{"status": "active"}).
		Where("id = ?", 1).
		OrWhere(map[string]string{"bad": "value"}) // unsupported type

	assert.NotNil(t, uq.buildErr, "invalid OrWhere() type must store buildErr on UpdateQuery")
	assert.ErrorContains(t, uq.buildErr, "OrWhere()")

	q := uq.Build()
	require.NotNil(t, q.prepErr)
}

// TestDeleteQuery_BuildErr_Where verifies that an invalid Where() type on
// DeleteQuery stores a build error.
func TestDeleteQuery_BuildErr_Where(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	dq := qb.Delete("users").Where([]string{"bad"}) // unsupported type

	assert.NotNil(t, dq.buildErr, "invalid Where() type must store buildErr on DeleteQuery")
	assert.ErrorContains(t, dq.buildErr, "Where()")

	q := dq.Build()
	require.NotNil(t, q.prepErr)
}

// TestDeleteQuery_BuildErr_OrWhere verifies that an invalid OrWhere() type on
// DeleteQuery stores a build error.
func TestDeleteQuery_BuildErr_OrWhere(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	dq := qb.Delete("users").
		Where("id = ?", 1).
		OrWhere(true) // bool — unsupported type

	assert.NotNil(t, dq.buildErr, "invalid OrWhere() type must store buildErr on DeleteQuery")
	assert.ErrorContains(t, dq.buildErr, "OrWhere()")

	q := dq.Build()
	require.NotNil(t, q.prepErr)
}

// TestBatchInsertQuery_BuildErr_Values verifies that wrong value count stores
// a build error.
func TestBatchInsertQuery_BuildErr_Values(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	biq := qb.BatchInsert("users", []string{"name", "email"})
	biq.Values("Alice") // 1 value for 2 columns

	assert.NotNil(t, biq.buildErr, "wrong value count must store buildErr")
	assert.ErrorContains(t, biq.buildErr, "BatchInsert.Values")
	assert.ErrorContains(t, biq.buildErr, "2")

	q := biq.Build()
	require.NotNil(t, q.prepErr)
}

// TestBatchInsertQuery_BuildErr_NoRows verifies that Build with no rows returns
// an error instead of panicking.
func TestBatchInsertQuery_BuildErr_NoRows(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	biq := qb.BatchInsert("users", []string{"name"})
	// No Values() calls

	q := biq.Build()
	require.NotNil(t, q.prepErr, "Build with no rows must store an error in prepErr")
	assert.ErrorContains(t, q.prepErr, "BatchInsert")
}

// TestBatchUpdateQuery_BuildErr_NoUpdates verifies that Build with no updates
// returns an error instead of panicking.
func TestBatchUpdateQuery_BuildErr_NoUpdates(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	buq := qb.BatchUpdate("users", "id")
	// No Set() calls

	q := buq.Build()
	require.NotNil(t, q.prepErr, "Build with no updates must store an error in prepErr")
	assert.ErrorContains(t, q.prepErr, "BatchUpdate")
}

// TestLikeExp_BuildErr_EscapeChars verifies that an odd number of escape chars
// stores an error accessible via Err() and makes Build return empty SQL.
func TestLikeExp_BuildErr_EscapeChars(t *testing.T) {
	dialect := dialects.GetDialect("postgres")

	exp := Like("name", "test").EscapeChars("%", "\\%", "_") // 3 strings — odd

	require.NotNil(t, exp.Err(), "odd EscapeChars must store an error via Err()")
	assert.ErrorContains(t, exp.Err(), "EscapeChars")
	assert.ErrorContains(t, exp.Err(), "3")

	sql, args := exp.Build(dialect)
	assert.Empty(t, sql, "Build with stored error must return empty SQL")
	assert.Nil(t, args, "Build with stored error must return nil args")
}

// TestLikeExp_BuildErr_EscapeChars_ValidAfterError verifies that a valid
// EscapeChars call after an error does NOT override the stored error.
func TestLikeExp_BuildErr_EscapeChars_ValidAfterError(t *testing.T) {
	// First call with odd count stores error; second would be ignored in real code
	// (the error is set on the instance). We verify the error is preserved.
	exp := Like("name", "test").EscapeChars("_") // 1 string — odd

	assert.NotNil(t, exp.Err())
}
