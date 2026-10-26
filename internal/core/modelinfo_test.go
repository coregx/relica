package core

import (
	"reflect"
	"strings"
	"testing"
)

type miTimestamps struct {
	CreatedAt string
	UpdatedAt string `db:"updated"`
}

type miBase struct {
	ID int
}

type miUser struct {
	miBase
	miTimestamps
	UserName string
	HTTPLog  string `db:"http_log"`
	Skip     string `db:"-"`
	secret   string //nolint:unused // proves unexported fields are skipped
}

func columnsOf(t *testing.T, db *DB, model any) []string {
	t.Helper()
	m, err := db.structValues(model)
	if err != nil {
		t.Fatalf("structValues: %v", err)
	}
	return getKeys(m)
}

func TestModelInfo_UntaggedSnakeAndEmbeddedFlattened(t *testing.T) {
	db := mockDB("postgres")
	got := strings.Join(columnsOf(t, db, &miUser{}), ",")
	// FIX-011: embedded structs were not flattened on write; untagged fields were written verbatim.
	if want := "created_at,http_log,id,updated,user_name"; got != want {
		t.Fatalf("columns = %s, want %s", got, want)
	}
	pk, err := db.primaryKey(reflect.ValueOf(miUser{miBase: miBase{ID: 7}}))
	if err != nil {
		t.Fatal(err)
	}
	if pk.Columns[0] != "id" || pk.Values[0].Int() != 7 {
		t.Fatalf("PK through embedding: %v %v", pk.Columns, pk.Values[0])
	}
}

type miLegacyPK struct {
	UserKey int `db:"pk"`
	Name    string
}

func TestModelInfo_LegacyPKTagUsesDerivedColumn(t *testing.T) {
	// FIX-011: db:"pk" wrote a column named "pk" while the PK code used "userkey".
	db := mockDB("sqlite")
	got := strings.Join(columnsOf(t, db, &miLegacyPK{UserKey: 1}), ",")
	if got != "name,user_key" {
		t.Fatalf("columns = %s", got)
	}
	pk, err := db.primaryKey(reflect.ValueOf(miLegacyPK{}))
	if err != nil || pk.Columns[0] != "user_key" {
		t.Fatalf("pk = %v, %v", pk, err)
	}
}

type miComposite struct {
	Qty       int `db:"qty"`
	OrderID   int `db:"order_id,pk"`
	ProductID int `db:",pk"`
}

func TestModelInfo_CompositePKInDeclarationOrder(t *testing.T) {
	pk, err := mockDB("postgres").primaryKey(reflect.ValueOf(miComposite{}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pk.Columns, ",") != "order_id,product_id" {
		t.Fatalf("pk columns = %v", pk.Columns)
	}
}

type miDup struct {
	UserName string
	Other    string `db:"USER_NAME"`
}

type miEmbedPtr struct {
	*miBase
	Name string
}

type miEmbedTagged struct {
	miBase `db:"base"`
	Name   string
}

type miLongName struct {
	ThisFieldNameIsDeliberatelyFarTooLongForAPostgresIdentifierAndMore int
}

func TestModelInfo_Errors(t *testing.T) {
	db := mockDB("postgres")
	cases := []struct {
		model any
		want  string
	}{
		{&miDup{}, `fields UserName and Other both map to column "user_name"`},
		{&miEmbedPtr{}, "embedded pointer"},
		{&miEmbedTagged{}, "db tag on an embedded struct"},
		{&miLongName{}, "longer than 63 bytes"},
	}
	for _, c := range cases {
		_, err := db.structValues(c.model)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%T: err = %v, want it to contain %q", c.model, err, c.want)
		}
	}
}

func TestModelInfo_NamingRefusesAndCustom(t *testing.T) {
	strict := mockDB("postgres")
	WithNaming(Naming{Column: func(ColumnSource) string { return "" }})(strict)
	_, err := strict.structValues(&miUser{})
	if err == nil || !strings.Contains(err.Error(), `Naming.Column returned "" for field miUser.ID`) {
		t.Fatalf("refused column: err = %v", err)
	}

	upper := mockDB("postgres")
	WithNaming(Naming{Column: func(cs ColumnSource) string { return strings.ToUpper(cs.Field.Name) }})(upper)
	if got := strings.Join(columnsOf(t, upper, &miLegacyPK{}), ","); got != "NAME,USERKEY" {
		t.Fatalf("custom naming columns = %s", got)
	}
	// Another DB keeps its own naming for the same type (per-DB cache).
	if got := strings.Join(columnsOf(t, mockDB("postgres"), &miLegacyPK{}), ","); got != "name,user_key" {
		t.Fatalf("default DB after custom DB = %s", got)
	}
}

func TestModelInfo_ColumnSourceForEmbeddedField(t *testing.T) {
	db := mockDB("postgres")
	var seen []ColumnSource
	WithNaming(Naming{Column: func(cs ColumnSource) string {
		seen = append(seen, cs)
		return SnakeCase(cs.Field.Name)
	}})(db)
	if _, err := db.structValues(&miUser{}); err != nil {
		t.Fatal(err)
	}
	for _, cs := range seen {
		if cs.Field.Name != "CreatedAt" {
			continue
		}
		model := reflect.TypeOf(miUser{})
		if cs.Model != model || cs.Owner != reflect.TypeOf(miTimestamps{}) {
			t.Fatalf("Model/Owner = %v/%v", cs.Model, cs.Owner)
		}
		if model.FieldByIndex(cs.Index).Name != "CreatedAt" {
			t.Fatalf("Index %v does not lead from Model to the field", cs.Index)
		}
		return
	}
	t.Fatal("CreatedAt not passed to Naming.Column")
}

// --- table names ---------------------------------------------------------

type miPtrRecv struct{ ID int }

func (*miPtrRecv) TableName() string { return "custom_ptr" }

type miValRecv struct{ ID int }

func (miValRecv) TableName() string { return "custom_val" }

type miInstance struct {
	ID     int
	Tenant string
}

func (m *miInstance) TableName() string { return m.Tenant + "_items" }

type miNoTable struct{ ID int }

type miUserProfile struct{ ID int }

func TestTableName_Rules(t *testing.T) {
	db := mockDB("postgres")
	cases := []struct {
		model any
		want  string
	}{
		{&miPtrRecv{}, "custom_ptr"},
		{miPtrRecv{}, "custom_ptr"}, // FIX-011: pointer-receiver TableName was ignored on a value model
		{miValRecv{}, "custom_val"},
		{&miValRecv{}, "custom_val"},
		{&miInstance{Tenant: "acme"}, "acme_items"}, // instance-dependent on a non-nil *T
		{&miUserProfile{}, "mi_user_profile"},       // was "miuserprofiles"
		{[]miUserProfile{}, "mi_user_profile"},
		{&[]*miPtrRecv{}, "custom_ptr"},
	}
	for _, c := range cases {
		got, err := db.tableNameOf(c.model)
		if err != nil || got != c.want {
			t.Errorf("%T: got %q, %v; want %q", c.model, got, err, c.want)
		}
	}
}

func TestTableName_NamingRefusesOrTooLong(t *testing.T) {
	db := mockDB("postgres")
	WithNaming(Naming{Table: func(TableSource) string { return "" }})(db)
	if _, err := db.tableNameOf(&miNoTable{}); err == nil || !strings.Contains(err.Error(), "miNoTable") {
		t.Fatalf("refused table: %v", err)
	}
	// TableName() still wins over a refusing Naming.
	if got, err := db.tableNameOf(&miPtrRecv{}); err != nil || got != "custom_ptr" {
		t.Fatalf("TableName() with refusing Naming: %q, %v", got, err)
	}

	long := mockDB("postgres")
	WithNaming(Naming{Table: func(TableSource) string { return strings.Repeat("t", 64) }})(long)
	if _, err := long.tableNameOf(&miNoTable{}); err == nil || !strings.Contains(err.Error(), "longer than 63 bytes") {
		t.Fatalf("long table: %v", err)
	}
	if _, err := db.tableNameOf(nil); err == nil {
		t.Fatal("nil model must be an error")
	}
}
