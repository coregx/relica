package core

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The rule table of ADR-018, frozen: changing a row is a breaking change.
func TestSnakeCase_RuleTable(t *testing.T) {
	split := []CaseOption{SplitAcronyms()}
	listed := []CaseOption{SplitAcronyms(), Initialisms(GoInitialisms()...)}
	rows := []struct {
		in, def, split, listed string
	}{
		{"UserProfile", "user_profile", "user_profile", "user_profile"},
		{"Address", "address", "address", "address"},
		{"Category", "category", "category", "category"},
		{"ID", "id", "id", "id"},
		{"Id", "id", "id", "id"},
		{"UserID", "user_id", "user_id", "user_id"},
		{"UserIDs", "user_ids", "user_i_ds", "user_ids"},
		{"HTTPLog", "httplog", "http_log", "http_log"},
		{"APIKey", "apikey", "api_key", "api_key"},
		{"APIs", "apis", "ap_is", "apis"},
		{"URLPath", "urlpath", "url_path", "url_path"},
		{"JSONData", "jsondata", "json_data", "json_data"},
		{"MyHTTPServer", "my_httpserver", "my_http_server", "my_http_server"},
		{"HTTPSServer", "httpsserver", "https_server", "https_server"},
		{"XMLHTTPRequest", "xmlhttprequest", "xmlhttp_request", "xmlhttp_request"},
		{"IPv4Address", "ipv4_address", "i_pv4_address", "ipv4_address"},
		{"OAuth2Token", "oauth2_token", "o_auth2_token", "o_auth2_token"},
		{"Order2Item", "order2_item", "order2_item", "order2_item"},
		{"S3Bucket", "s3_bucket", "s3_bucket", "s3_bucket"},
		{"UTF8Text", "utf8_text", "utf8_text", "utf8_text"},
		// Whole tokens only: GORM's substring replace turns HIDE into h_id_e.
		{"HIDE", "hide", "hide", "hide"},
		{"VIDEO", "video", "video", "video"},
		{"GUIDE", "guide", "guide", "guide"},
		// A listed run followed by a lower-case letter stays split when the run
		// before the split is itself listed (UI): documented miss, use a tag.
		{"UIDs", "uids", "ui_ds", "ui_ds"},
		{"ÉcoleName", "école_name", "école_name", "école_name"},
		{"already_snake", "already_snake", "already_snake", "already_snake"},
		{"Foo_Bar", "foo_bar", "foo_bar", "foo_bar"},
		{"x", "x", "x", "x"},
		{"", "", "", ""},
	}
	for _, r := range rows {
		t.Run(r.in, func(t *testing.T) {
			if got := SnakeCase(r.in); got != r.def {
				t.Errorf("default: got %q, want %q", got, r.def)
			}
			if got := SnakeCase(r.in, split...); got != r.split {
				t.Errorf("SplitAcronyms: got %q, want %q", got, r.split)
			}
			if got := SnakeCase(r.in, listed...); got != r.listed {
				t.Errorf("SplitAcronyms+Initialisms: got %q, want %q", got, r.listed)
			}
		})
	}
}

// The default must equal ozzo-dbx's DefaultFieldMapFunc, which is
// regexp `([^A-Z_])([A-Z])` → "${1}_$2" then strings.ToLower (ozzo-dbx struct.go).
func TestSnakeCase_DefaultMatchesOzzoRegex(t *testing.T) {
	for _, in := range []string{
		"aBCdEfGHi", "A1B2c3D", "__AB__cD_", "lowerUPPERlower", "ÄbcDef", "x_Y", "XyZ", "AbcDEFghiJ",
		"ID", "UserID", "HTTPLog", "IPv4Address", "S3Bucket",
	} {
		if got, want := SnakeCase(in), ozzoSnake(in); got != want {
			t.Errorf("%q: got %q, ozzo-dbx %q", in, got, want)
		}
	}
}

var ozzoRe = regexp.MustCompile(`([^A-Z_])([A-Z])`)

func ozzoSnake(s string) string {
	// Reimplements regexp.MustCompile(`([^A-Z_])([A-Z])`).ReplaceAllString(s, "${1}_$2")
	// with the regexp package to cross-check the hand-written loop.
	return strings.ToLower(ozzoRe.ReplaceAllString(s, "${1}_$2"))
}

func TestGoInitialisms_IsACopy(t *testing.T) {
	a := GoInitialisms()
	a[0] = "MUTATED"
	if GoInitialisms()[0] == "MUTATED" {
		t.Fatal("GoInitialisms must return a copy")
	}
	if len(a) != 42 {
		t.Fatalf("GoInitialisms has %d entries, want the 42 of staticcheck v0.8.2 DefaultConfig", len(a))
	}
}

type namingUserProfile struct{}

func TestSnake_TableOptions(t *testing.T) {
	ts := TableSource{Type: reflect.TypeOf(namingUserProfile{})}
	if got := Snake().Table(ts); got != "naming_user_profile" {
		t.Errorf("default table: %q", got)
	}
	plural := func(s string) string { return s + "s" }
	n := Snake(TablePrefix("app."), PluralTables(plural))
	if got := n.Table(ts); got != "app.naming_user_profiles" {
		t.Errorf("prefix+plural: %q", got)
	}
}

type namingCols struct {
	UserID   int
	HTTPLog  string
	Embedded namingInner
}

type namingInner struct{ X int }

func TestSnake_ColumnUsesFieldName(t *testing.T) {
	typ := reflect.TypeOf(namingCols{})
	col := func(n Naming, i int) string {
		f := typ.Field(i)
		return n.Column(ColumnSource{Model: typ, Owner: typ, Field: f, Index: []int{i}})
	}
	if got := col(Snake(), 1); got != "httplog" {
		t.Errorf("default column: %q", got)
	}
	if got := col(Snake(SplitAcronyms()), 1); got != "http_log" {
		t.Errorf("split column: %q", got)
	}
	// TablePrefix and PluralTables never touch columns.
	if got := col(Snake(TablePrefix("x_"), PluralTables(func(s string) string { return s + "s" })), 0); got != "user_id" {
		t.Errorf("table options leaked into column: %q", got)
	}
}

func TestNaming_ZeroValueIsDefault(t *testing.T) {
	var n Naming
	ts := TableSource{Type: reflect.TypeOf(namingUserProfile{})}
	if got := n.table(ts); got != "naming_user_profile" {
		t.Errorf("zero Naming table: %q", got)
	}
	typ := reflect.TypeOf(namingCols{})
	if got := n.column(ColumnSource{Model: typ, Owner: typ, Field: typ.Field(0), Index: []int{0}}); got != "user_id" {
		t.Errorf("zero Naming column: %q", got)
	}
}
