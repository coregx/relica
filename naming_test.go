package relica_test

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/coregx/relica"
)

type namingAccount struct {
	AccountID   int
	DisplayName string
	HTTPStatus  int
	Note        string `db:"memo"`
}

func wrapWith(t *testing.T, opts ...relica.Option) *relica.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", "naming")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return relica.WrapDB(sqlDB, "sqlite3", opts...) // FEAT-013: options on WrapDB
}

func insertSQL(t *testing.T, db *relica.DB, model any) string {
	t.Helper()
	sqlText, _ := db.Builder().InsertStruct("t", model).ToSQL()
	return sqlText
}

func TestNaming_DefaultSnake(t *testing.T) {
	got := insertSQL(t, wrapWith(t), &namingAccount{AccountID: 1})
	want := `INSERT INTO "t" ("account_id", "display_name", "httpstatus", "memo") VALUES (?, ?, ?, ?)`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestNaming_SnakeOptionsAndCustom(t *testing.T) {
	split := wrapWith(t, relica.WithNaming(relica.Snake(relica.Initialisms(relica.GoInitialisms()...))))
	if got := insertSQL(t, split, &namingAccount{AccountID: 1}); !strings.Contains(got, `"http_status"`) {
		t.Fatalf("Initialisms: %s", got)
	}
	// Keep v0.19 column names (the Go field name) while migrating.
	legacy := wrapWith(t, relica.WithNaming(relica.Naming{
		Column: func(c relica.ColumnSource) string { return c.Field.Name },
	}))
	if got := insertSQL(t, legacy, &namingAccount{AccountID: 1}); !strings.Contains(got, `"DisplayName"`) || !strings.Contains(got, `"memo"`) {
		t.Fatalf("custom Naming: %s", got)
	}
	// A refusing Naming turns a missing tag into an error.
	strict := wrapWith(t, relica.WithNaming(relica.Naming{Column: func(relica.ColumnSource) string { return "" }}))
	if _, err := strict.Builder().InsertStruct("t", &namingAccount{}).Execute(); err == nil ||
		!strings.Contains(err.Error(), "add a db tag") {
		t.Fatalf("refusing Naming: err = %v", err)
	}
}

func TestNaming_TableOptions(t *testing.T) {
	n := relica.Snake(relica.TablePrefix("app."), relica.PluralTables(func(s string) string { return s + "s" }))
	if got := n.Table(relica.TableSource{Type: reflect.TypeOf(namingAccount{})}); got != "app.naming_accounts" {
		t.Fatalf("prefix+plural: %q", got)
	}
	if got := relica.SnakeCase("MyHTTPServer", relica.SplitAcronyms()); got != "my_http_server" {
		t.Fatalf("SnakeCase: %s", got)
	}
	if got := n.Table(relica.TableSource{}); got != "" {
		t.Fatalf("anonymous type must be refused, got %q", got)
	}
}
