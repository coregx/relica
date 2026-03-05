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

// TestResolveNamedParams tests the resolveNamedParams helper function
func TestResolveNamedParams(t *testing.T) {
	tests := []struct {
		name         string
		condition    string
		params       []interface{}
		wantSQL      string
		wantArgs     []interface{}
		wantResolved bool
	}{
		{
			name:      "single named param",
			condition: "id = {:id}",
			params:    []interface{}{Params{"id": 1}},
			wantSQL:   "id = ?",
			wantArgs:  []interface{}{1},
		},
		{
			name:      "multiple named params",
			condition: "id = {:id} AND status = {:status}",
			params:    []interface{}{Params{"id": 42, "status": "active"}},
			wantSQL:   "id = ? AND status = ?",
			wantArgs:  []interface{}{42, "active"},
		},
		{
			name:      "repeated named param",
			condition: "id = {:id} OR parent_id = {:id}",
			params:    []interface{}{Params{"id": 7}},
			wantSQL:   "id = ? OR parent_id = ?",
			wantArgs:  []interface{}{7, 7},
		},
		{
			name:      "positional params unchanged",
			condition: "status = ?",
			params:    []interface{}{1},
			wantSQL:   "status = ?",
			wantArgs:  []interface{}{1},
		},
		{
			name:      "no params unchanged",
			condition: "1 = 1",
			params:    nil,
			wantSQL:   "1 = 1",
			wantArgs:  nil,
		},
		{
			name:      "non-Params argument unchanged",
			condition: "id = {:id}",
			params:    []interface{}{"not a Params map"},
			wantSQL:   "id = {:id}",
			wantArgs:  []interface{}{"not a Params map"},
		},
		{
			name:      "multiple positional args unchanged",
			condition: "id = {:id}",
			params:    []interface{}{1, 2},
			wantSQL:   "id = {:id}",
			wantArgs:  []interface{}{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotArgs := resolveNamedParams(tt.condition, tt.params)
			assert.Equal(t, tt.wantSQL, gotSQL)
			assert.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}

// TestSelectQuery_Where_NamedParams tests named placeholders in SelectQuery.Where
func TestSelectQuery_Where_NamedParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().From("users").
		Where("id = {:id} AND status = {:status}", Params{"id": 1, "status": "active"})

	q := query.Build()
	assert.Contains(t, q.sql, "WHERE id = $1 AND status = $2")
	assert.Len(t, q.params, 2)
	assert.Equal(t, 1, q.params[0])
	assert.Equal(t, "active", q.params[1])
}

// TestSelectQuery_Where_NamedParams_MySQL tests named placeholders with MySQL dialect
func TestSelectQuery_Where_NamedParams_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select().From("users").
		Where("id = {:id}", Params{"id": 42})

	q := query.Build()
	assert.Contains(t, q.sql, "WHERE id = ?")
	assert.Len(t, q.params, 1)
	assert.Equal(t, 42, q.params[0])
}

// TestUpdateQuery_Where_NamedParams tests named placeholders in UpdateQuery.Where
func TestUpdateQuery_Where_NamedParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{"name": "Alice"}).
		Where("id = {:id}", Params{"id": 123})

	q := query.Build()
	assert.Contains(t, q.sql, "WHERE id =")
	assert.Equal(t, 123, q.params[len(q.params)-1])
}

// TestDeleteQuery_Where_NamedParams tests named placeholders in DeleteQuery.Where
func TestDeleteQuery_Where_NamedParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").
		Where("id = {:id} AND role = {:role}", Params{"id": 456, "role": "admin"})

	q := query.Build()
	assert.Contains(t, q.sql, "WHERE id =")
	assert.Len(t, q.params, 2)
	assert.Equal(t, 456, q.params[0])
	assert.Equal(t, "admin", q.params[1])
}

// TestOrWhere_NamedParams tests named placeholders in OrWhere
func TestOrWhere_NamedParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().From("users").
		Where("status = {:status}", Params{"status": 1}).
		OrWhere("role = {:role}", Params{"role": "admin"})

	q := query.Build()
	assert.Contains(t, q.sql, "WHERE")
	assert.Contains(t, q.sql, "OR")
	assert.Len(t, q.params, 2)
	assert.Equal(t, 1, q.params[0])
	assert.Equal(t, "admin", q.params[1])
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
