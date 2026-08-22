package core

import (
	"strings"
	"testing"
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
	if !strings.Contains(q.sql, `SELECT * FROM "users" WHERE`) {
		t.Errorf("%q does not contain %q", q.sql, `SELECT * FROM "users" WHERE`)
	}
	if len(q.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(q.params))
	}
	if got := q.params[0]; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}
	if got := q.params[1]; got != 18 {
		t.Errorf("got %v, want %v", got, 18)
	}
}

// TestUpdateQuery_Where_Expression tests UpdateQuery.Where() with Expression API
func TestUpdateQuery_Where_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{"status": 2}).
		Where(Eq("id", 123))

	q := query.Build()
	if !strings.Contains(q.sql, `UPDATE "users" SET`) {
		t.Errorf("%q does not contain %q", q.sql, `UPDATE "users" SET`)
	}
	if !strings.Contains(q.sql, `WHERE`) {
		t.Errorf("%q does not contain %q", q.sql, `WHERE`)
	}
	if len(q.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(q.params))
	}
	if got := q.params[0]; got != 2 { // SET value
		t.Errorf("got %v, want %v", got, 2)
	}
	if got := q.params[1]; got != 123 { // WHERE value
		t.Errorf("got %v, want %v", got, 123)
	}
}

// TestDeleteQuery_Where_Expression tests DeleteQuery.Where() with Expression API
func TestDeleteQuery_Where_Expression(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").Where(In("status", 0, 1, 2))

	q := query.Build()
	if !strings.Contains(q.sql, `DELETE FROM "users" WHERE`) {
		t.Errorf("%q does not contain %q", q.sql, `DELETE FROM "users" WHERE`)
	}
	if len(q.params) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(q.params))
	}
	want := []interface{}{0, 1, 2}
	if got := q.params; len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got %v, want %v", got, want)
				break
			}
		}
	}
}

// TestWhere_BackwardCompatibility tests that string-based Where() still works
func TestWhere_BackwardCompatibility(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	// SELECT with string Where
	sq := qb.Select().From("users").Where("status = ?", 1)
	qSelect := sq.Build()
	if !strings.Contains(qSelect.sql, "WHERE status = $1") {
		t.Errorf("%q does not contain %q", qSelect.sql, "WHERE status = $1")
	}

	// UPDATE with string Where
	uq := qb.Update("users").Set(map[string]interface{}{"name": "Alice"}).Where("id = ?", 123)
	qUpdate := uq.Build()
	if !strings.Contains(qUpdate.sql, "WHERE id = $2") {
		t.Errorf("%q does not contain %q", qUpdate.sql, "WHERE id = $2")
	}

	// DELETE with string Where
	dq := qb.Delete("users").Where("id = ?", 456)
	qDelete := dq.Build()
	if !strings.Contains(qDelete.sql, "WHERE id = $1") {
		t.Errorf("%q does not contain %q", qDelete.sql, "WHERE id = $1")
	}
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
			gotSQL, gotArgs, err := resolveNamedParams(tt.condition, tt.params)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("got %v, want %v", gotSQL, tt.wantSQL)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Errorf("got %v, want %v", gotArgs, tt.wantArgs)
			} else {
				for i := range tt.wantArgs {
					if gotArgs[i] != tt.wantArgs[i] {
						t.Errorf("got %v, want %v", gotArgs, tt.wantArgs)
						break
					}
				}
			}
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
	if !strings.Contains(q.sql, "WHERE id = $1 AND status = $2") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE id = $1 AND status = $2")
	}
	if len(q.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(q.params))
	}
	if got := q.params[0]; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}
	if got := q.params[1]; got != "active" {
		t.Errorf("got %v, want %v", got, "active")
	}
}

// TestSelectQuery_Where_NamedParams_MySQL tests named placeholders with MySQL dialect
func TestSelectQuery_Where_NamedParams_MySQL(t *testing.T) {
	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	query := qb.Select().From("users").
		Where("id = {:id}", Params{"id": 42})

	q := query.Build()
	if !strings.Contains(q.sql, "WHERE id = ?") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE id = ?")
	}
	if len(q.params) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(q.params))
	}
	if got := q.params[0]; got != 42 {
		t.Errorf("got %v, want %v", got, 42)
	}
}

// TestUpdateQuery_Where_NamedParams tests named placeholders in UpdateQuery.Where
func TestUpdateQuery_Where_NamedParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Update("users").
		Set(map[string]interface{}{"name": "Alice"}).
		Where("id = {:id}", Params{"id": 123})

	q := query.Build()
	if !strings.Contains(q.sql, "WHERE id =") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE id =")
	}
	if got := q.params[len(q.params)-1]; got != 123 {
		t.Errorf("got %v, want %v", got, 123)
	}
}

// TestDeleteQuery_Where_NamedParams tests named placeholders in DeleteQuery.Where
func TestDeleteQuery_Where_NamedParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Delete("users").
		Where("id = {:id} AND role = {:role}", Params{"id": 456, "role": "admin"})

	q := query.Build()
	if !strings.Contains(q.sql, "WHERE id =") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE id =")
	}
	if len(q.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(q.params))
	}
	if got := q.params[0]; got != 456 {
		t.Errorf("got %v, want %v", got, 456)
	}
	if got := q.params[1]; got != "admin" {
		t.Errorf("got %v, want %v", got, "admin")
	}
}

// TestOrWhere_NamedParams tests named placeholders in OrWhere
func TestOrWhere_NamedParams(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	query := qb.Select().From("users").
		Where("status = {:status}", Params{"status": 1}).
		OrWhere("role = {:role}", Params{"role": "admin"})

	q := query.Build()
	if !strings.Contains(q.sql, "WHERE") {
		t.Errorf("%q does not contain %q", q.sql, "WHERE")
	}
	if !strings.Contains(q.sql, "OR") {
		t.Errorf("%q does not contain %q", q.sql, "OR")
	}
	if len(q.params) != 2 {
		t.Errorf("expected length %d, got %d", 2, len(q.params))
	}
	if got := q.params[0]; got != 1 {
		t.Errorf("got %v, want %v", got, 1)
	}
	if got := q.params[1]; got != "admin" {
		t.Errorf("got %v, want %v", got, "admin")
	}
}

// TestWhere_Panic tests that invalid Where() arguments store an error
// instead of panicking, and that the error propagates through Build.
func TestWhere_Panic(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	sq := qb.Select().From("users").Where(123) // int is not string or Expression
	q := sq.Build()
	if q.prepErr == nil {
		t.Error("invalid Where() type must store build error")
	}
	if q.prepErr != nil && !strings.Contains(q.prepErr.Error(), "Where()") {
		t.Errorf("expected error containing %q, got %v", "Where()", q.prepErr)
	}

	uq := qb.Update("users").Set(map[string]interface{}{"x": 1}).Where([]string{"bad"})
	q = uq.Build()
	if q.prepErr == nil {
		t.Error("invalid Where() type must store build error on UpdateQuery")
	}
	if q.prepErr != nil && !strings.Contains(q.prepErr.Error(), "Where()") {
		t.Errorf("expected error containing %q, got %v", "Where()", q.prepErr)
	}

	dq := qb.Delete("users").Where(map[string]int{"bad": 1})
	q = dq.Build()
	if q.prepErr == nil {
		t.Error("invalid Where() type must store build error on DeleteQuery")
	}
	if q.prepErr != nil && !strings.Contains(q.prepErr.Error(), "Where()") {
		t.Errorf("expected error containing %q, got %v", "Where()", q.prepErr)
	}
}
