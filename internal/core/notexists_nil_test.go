package core

import "testing"

// TestNotExistsNil_InDeleteAndUpdate_IsError verifies AUDIT-001 N1: NotExists(nil)
// renders no condition, so DELETE/UPDATE with it would hit every row; ADR-020
// rule 2 (a condition that renders empty) makes it a build error.
func TestNotExistsNil_InDeleteAndUpdate_IsError(t *testing.T) {
	qb := &QueryBuilder{db: mockDB("postgres")}
	if q := qb.Delete("t").Where(NotExists(nil)).Build(); q.prepErr == nil {
		t.Errorf("DELETE ... NotExists(nil): sql %q, want a build error", q.sql)
	}
	if q := qb.Update("t").Set(map[string]any{"a": 1}).Where(NotExists(nil)).Build(); q.prepErr == nil {
		t.Errorf("UPDATE ... NotExists(nil): sql %q, want a build error", q.sql)
	}
}
