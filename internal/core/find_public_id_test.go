package core

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/coregx/relica/internal/util"
)

// --- FindByPublicID unit tests ---

func TestFindByPublicID_NoTable(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
	}

	user := &User{}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres"), table: ""}

	err := mq.FindByPublicID("usr_019078fa")
	if err == nil {
		t.Error("expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "table name not specified") {
		t.Errorf("%q does not contain %q", err.Error(), "table name not specified")
	}
}

func TestFindByPublicID_EmptyID(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
	}

	user := &User{}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres"), table: "users"}

	err := mq.FindByPublicID("")
	if err == nil {
		t.Error("expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "empty") {
		t.Errorf("%q does not contain %q", err.Error(), "empty")
	}
}

func TestFindByPublicID_NoAutoIDField(t *testing.T) {
	type Simple struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	s := &Simple{}
	mq := &ModelQuery{model: s, db: mockDBFull("postgres"), table: "simples"}

	err := mq.FindByPublicID("some_id")
	if err == nil {
		t.Error("expected error")
	}
	if err != nil && !strings.Contains(err.Error(), "no autoid field") {
		t.Errorf("%q does not contain %q", err.Error(), "no autoid field")
	}
}

func TestFindByPublicID_PrefixMismatch(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
	}

	user := &User{}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres"), table: "users"}

	err := mq.FindByPublicID("ord_019078fa")
	if !errors.Is(err, util.ErrAutoIDPrefixMismatch) {
		t.Errorf("expected error %v, got %v", util.ErrAutoIDPrefixMismatch, err)
	}
}

func TestFindByPublicID_PrefixMissing(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
	}

	user := &User{}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres"), table: "users"}

	err := mq.FindByPublicID("019078fa-no-prefix")
	if !errors.Is(err, util.ErrAutoIDPrefixMismatch) {
		t.Errorf("expected error %v, got %v", util.ErrAutoIDPrefixMismatch, err)
	}
}

func TestFindByPublicID_NoPrefixTag_PassesValidation(t *testing.T) {
	type Event struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid"`
	}

	event := &Event{}

	// No prefix in tag → ValidateAutoIDPrefix with empty expected = always passes
	err := util.ValidateAutoIDPrefix("any-id-format", "")
	if err != nil {
		t.Errorf("no-prefix autoid should accept any ID: unexpected error: %v", err)
	}

	// The struct-info builder finds the autoid field with no prefix
	fields, err := mockDB("postgres").autoIDFields(reflect.TypeOf(event).Elem())
	if err != nil || len(fields) != 1 {
		t.Fatalf("autoIDFields = %v, %v", fields, err)
	}
	if fields[0].prefix != "" || fields[0].column != "public_id" {
		t.Errorf("got prefix %q column %q", fields[0].prefix, fields[0].column)
	}
}

func TestFindByPublicID_PrefixMatch_PassesValidation(t *testing.T) {
	// Correct prefix → passes validation
	err := util.ValidateAutoIDPrefix("usr_019078fa-valid", "usr")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Wrong prefix → fails
	err = util.ValidateAutoIDPrefix("ord_019078fa", "usr")
	if !errors.Is(err, util.ErrAutoIDPrefixMismatch) {
		t.Errorf("expected error %v, got %v", util.ErrAutoIDPrefixMismatch, err)
	}
}

func TestFindByPublicID_UsesFirstAutoIDField(t *testing.T) {
	type Multi struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
		TraceID  string `db:"trace_id,autoid:trc"`
	}

	m := &Multi{}
	mq := &ModelQuery{model: m, db: mockDBFull("postgres"), table: "multis"}

	// Should validate against first autoid field prefix (usr)
	err := mq.FindByPublicID("trc_019078fa")
	if !errors.Is(err, util.ErrAutoIDPrefixMismatch) {
		t.Errorf("should validate against first autoid field (usr): expected error %v, got %v", util.ErrAutoIDPrefixMismatch, err)
	}
}
