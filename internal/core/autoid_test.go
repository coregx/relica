package core

import (
	"reflect"
	"strings"
	"testing"

	"github.com/coregx/relica/internal/util"
)

// --- populateAutoIDFields ---

func TestPopulateAutoIDFields_GeneratesWhenEmpty(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
		Name     string `db:"name"`
	}

	user := &User{Name: "Alice"}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres")}

	mq.populateAutoIDFields()

	if user.PublicID == "" {
		t.Error("expected non-empty PublicID")
	}
	if !strings.HasPrefix(user.PublicID, "usr_") {
		t.Errorf("expected usr_ prefix, got %s", user.PublicID)
	}
	if len(user.PublicID) != 4+36 { // "usr_" + UUID v7
		t.Errorf("expected length %d, got %d", 4+36, len(user.PublicID))
	}
}

func TestPopulateAutoIDFields_SkipsPreset(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
	}

	user := &User{PublicID: "usr_custom-test-id"}
	mq := &ModelQuery{model: user, db: mockDBFull("postgres")}

	mq.populateAutoIDFields()

	if user.PublicID != "usr_custom-test-id" {
		t.Errorf("got %v, want %v", user.PublicID, "usr_custom-test-id")
	}
}

func TestPopulateAutoIDFields_NoPrefix(t *testing.T) {
	type Event struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid"`
	}

	event := &Event{}
	mq := &ModelQuery{model: event, db: mockDBFull("postgres")}

	mq.populateAutoIDFields()

	if event.PublicID == "" {
		t.Error("expected non-empty PublicID")
	}
	if strings.Contains(event.PublicID[:8], "_") {
		t.Errorf("no prefix expected, got %s", event.PublicID)
	}
	if len(event.PublicID) != 36 { // raw UUID v7
		t.Errorf("expected length %d, got %d", 36, len(event.PublicID))
	}
}

func TestPopulateAutoIDFields_Multiple(t *testing.T) {
	type Order struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:ord"`
		TraceID  string `db:"trace_id,autoid:trc"`
	}

	order := &Order{}
	mq := &ModelQuery{model: order, db: mockDBFull("postgres")}

	mq.populateAutoIDFields()

	if !strings.HasPrefix(order.PublicID, "ord_") {
		t.Errorf("expected ord_ prefix, got %s", order.PublicID)
	}
	if !strings.HasPrefix(order.TraceID, "trc_") {
		t.Errorf("expected trc_ prefix, got %s", order.TraceID)
	}
	if order.PublicID == order.TraceID {
		t.Errorf("expected different, both %v", order.PublicID)
	}
}

func TestPopulateAutoIDFields_NoAutoIDTag(t *testing.T) {
	type Simple struct {
		ID   int64  `db:"id,pk"`
		Name string `db:"name"`
	}

	simple := &Simple{Name: "test"}
	mq := &ModelQuery{model: simple, db: mockDBFull("postgres")}

	mq.populateAutoIDFields()

	if simple.Name != "test" {
		t.Errorf("got %v, want %v", simple.Name, "test")
	}
}

func TestPopulateAutoIDFields_NonStringFieldSkipped(t *testing.T) {
	type Bad struct {
		ID   int64  `db:"id,pk"`
		Code int    `db:"code,autoid:x"`
		Name string `db:"name"`
	}

	bad := &Bad{Name: "test"}
	mq := &ModelQuery{model: bad, db: mockDBFull("postgres")}

	mq.populateAutoIDFields()

	if bad.Code != 0 {
		t.Errorf("got %v, want %v", bad.Code, 0)
	}
}

func TestPopulateAutoIDFields_UniquePerCall(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
	}

	seen := make(map[string]bool, 100)
	for range 100 {
		user := &User{}
		mq := &ModelQuery{model: user, db: mockDBFull("postgres")}
		mq.populateAutoIDFields()
		if seen[user.PublicID] {
			t.Fatalf("duplicate: %s", user.PublicID)
		}
		seen[user.PublicID] = true
	}
}

// --- Insert SQL verification ---

func TestInsert_AutoID_IncludedInSQL(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
		Name     string `db:"name"`
	}

	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	user := User{Name: "Alice"}

	// Simulate what Insert() does: populate autoid, then StructToMap
	v := reflect.ValueOf(&user)
	fields := util.FindAutoIDFields(v)
	if len(fields) != 1 {
		t.Fatalf("expected length %d, got %d", 1, len(fields))
	}

	// Populate
	fv := v.Elem().Field(fields[0].FieldIndex)
	fv.SetString(util.GenerateAutoID(fields[0].Prefix, fields[0].Generator))

	dataMap, err := util.StructToMap(&user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Zero PK removed
	delete(dataMap, "id")

	query := qb.Insert("users", dataMap)
	sql, params := query.ToSQL()

	if !strings.Contains(sql, "public_id") {
		t.Errorf("%q does not contain %q", sql, "public_id")
	}
	if !strings.Contains(sql, "name") {
		t.Errorf("%q does not contain %q", sql, "name")
	}

	// Verify public_id value is in params
	found := false
	for _, p := range params {
		if s, ok := p.(string); ok && strings.HasPrefix(s, "usr_") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("public_id value not in params: %v", params)
	}
}

func TestInsert_AutoID_PresetNotOverwritten_SQL(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
		Name     string `db:"name"`
	}

	db := mockDB("mysql")
	qb := &QueryBuilder{db: db}

	user := User{PublicID: "usr_preset-id", Name: "Bob"}

	// Simulate populate — should NOT overwrite
	v := reflect.ValueOf(&user).Elem()
	fields := util.FindAutoIDFields(v)
	for _, f := range fields {
		fv := v.Field(f.FieldIndex)
		if fv.Kind() == reflect.String && fv.String() != "" {
			continue // skip preset
		}
	}

	dataMap, _ := util.StructToMap(&user)
	delete(dataMap, "id")

	query := qb.Insert("users", dataMap)
	_, params := query.ToSQL()

	// Verify preset value is in params
	found := false
	for _, p := range params {
		if s, ok := p.(string); ok && s == "usr_preset-id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("preset public_id not in params: %v", params)
	}
}

func TestUpsert_AutoID_Populated(t *testing.T) {
	type User struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:usr"`
		Name     string `db:"name"`
	}

	user := &User{ID: 1, Name: "Alice"}
	mq := &ModelQuery{model: user, db: mockDBFull("mysql")}

	mq.populateAutoIDFields()

	if !strings.HasPrefix(user.PublicID, "usr_") {
		t.Errorf("expected usr_ prefix, got %s", user.PublicID)
	}
}

// --- Custom generator ---

func TestPopulateAutoIDFields_CustomGenerator(t *testing.T) {
	util.RegisterIDGenerator("test-seq", func() string { return "seq-001" })
	defer func() {
		util.RegisterIDGenerator("test-seq", nil) // cleanup
	}()

	type Event struct {
		ID       int64  `db:"id,pk"`
		PublicID string `db:"public_id,autoid:evt,gen=test-seq"`
	}

	event := &Event{}
	mq := &ModelQuery{model: event, db: mockDBFull("postgres")}

	mq.populateAutoIDFields()

	if event.PublicID != "evt_seq-001" {
		t.Errorf("got %v, want %v", event.PublicID, "evt_seq-001")
	}
}
