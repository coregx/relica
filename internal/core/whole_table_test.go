package core

import "testing"

// ADR-020 rules 1–2: UPDATE/DELETE without Where, or with a condition that
// renders empty, would hit every row; they are build errors. The whole table
// only through an explicit AllRows().

const (
	noWhere        = "without Where"
	emptyCondition = "renders empty"
)

func TestUpdateDelete_NoWhere_IsBuildError(t *testing.T) {
	for _, d := range allDialects {
		qb := &QueryBuilder{db: mockDB(d)}
		t.Run(d, func(t *testing.T) {
			buildErrContains(t, qb.Delete("users").Build(), noWhere)
			buildErrContains(t, qb.Update("users").Set(map[string]any{"a": 1}).Build(), noWhere)
		})
	}
}

func TestUpdateDelete_AllRows(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	buildOK(t, qb.Delete("users").AllRows().Build(), `DELETE FROM "users"`)
	buildOK(t, qb.Update("users").Set(map[string]any{"a": 1}).AllRows().Build(), `UPDATE "users" SET "a" = $1`)
	buildErrContains(t, qb.Delete("users").AllRows().Where(Eq("id", 1)).Build(), "AllRows() together with Where")
	buildErrContains(t, qb.Update("users").Set(map[string]any{"a": 1}).Where(Eq("id", 1)).AllRows().Build(), "AllRows() together with Where")
}

func TestUpdateDelete_EmptyCondition_IsBuildError(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	empties := map[string]Expression{
		"And()":       And(),
		"Or()":        Or(),
		"HashExp{}":   HashExp{},
		"Like no val": Like("name"),
	}
	for name, exp := range empties {
		t.Run(name, func(t *testing.T) {
			buildErrContains(t, qb.Delete("users").Where(exp).Build(), emptyCondition)
			buildErrContains(t, qb.Update("users").Set(map[string]any{"a": 1}).Where(exp).Build(), emptyCondition)
			buildErrContains(t, qb.Delete("users").Where(Eq("id", 1)).OrWhere(exp).Build(), emptyCondition)
			buildErrContains(t, qb.Update("users").Set(map[string]any{"a": 1}).Where(Eq("id", 1)).AndWhere(exp).Build(), emptyCondition)
		})
	}
	buildErrContains(t, qb.Delete("users").Where("").Build(), emptyCondition)
	buildErrContains(t, qb.Delete("users").Where("   ").Build(), emptyCondition)
}

func TestUpdateDelete_WithWhere_Unchanged(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	buildOK(t, qb.Delete("users").Where(Eq("id", 1)).Build(), `DELETE FROM "users" WHERE "id" = $1`)
	buildOK(t, qb.Delete("users").Where("id = ?", 1).Build(), `DELETE FROM "users" WHERE id = $1`)
	buildOK(t, qb.Update("users").Set(map[string]any{"a": 1}).Where(Eq("id", 2)).Build(),
		`UPDATE "users" SET "a" = $1 WHERE "id" = $2`)
}

// SELECT keeps its semantics: an empty condition selects every row.
func TestSelect_EmptyCondition_StillSelectsAll(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	buildOK(t, qb.Select("id").From("users").Where(And()).Build(), `SELECT "id" FROM "users"`)
}
