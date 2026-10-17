package analyzer

import "testing"

// mysql8ExplainJSON is EXPLAIN FORMAT=JSON as MySQL 8.0 prints it: "filtered"
// and every cost are JSON strings, row counts are JSON numbers. The fixtures in
// mysql_test.go used a numeric "filtered", so the parser failed on every real
// MySQL server ("cannot unmarshal string into ... filtered of type float64")
// and SelectQuery.Explain never worked on MySQL. Found by the v0.19.0
// integration suite.
const mysql8ExplainJSON = `{
  "query_block": {
    "select_id": 1,
    "cost_info": {"query_cost": "0.55"},
    "table": {
      "table_name": "pt_item",
      "access_type": "ALL",
      "rows_examined_per_scan": 3,
      "rows_produced_per_join": 1,
      "filtered": "33.33",
      "cost_info": {
        "read_cost": "0.45",
        "eval_cost": "0.10",
        "prefix_cost": "0.55",
        "data_read_per_join": "1K"
      },
      "used_columns": ["id", "name", "state"],
      "attached_condition": "(` + "`test`.`pt_item`.`state` = 1" + `)"
    }
  }
}`

func TestParseMySQLExplain_RealServerOutput(t *testing.T) {
	plan, err := parseMySQLExplain(mysql8ExplainJSON, false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if plan.Cost != 0.55 {
		t.Errorf("Cost = %v, want 0.55", plan.Cost)
	}
	if plan.EstimatedRows != 3 {
		t.Errorf("EstimatedRows = %d, want 3", plan.EstimatedRows)
	}
	if !plan.FullScan || plan.UsesIndex {
		t.Errorf("FullScan = %v, UsesIndex = %v; want full scan without index", plan.FullScan, plan.UsesIndex)
	}
	if plan.Database != dbMySQL {
		t.Errorf("Database = %q", plan.Database)
	}
}

// MySQL and MariaDB versions differ in whether numbers are quoted; both forms
// must parse to the same value.
func TestMySQLNumber_QuotedAndUnquoted(t *testing.T) {
	for _, in := range []string{
		`{"query_block":{"table":{"rows_examined_per_scan":"7","rows_produced_per_join":"2","filtered":"50.00","access_type":"ALL"}}}`,
		`{"query_block":{"table":{"rows_examined_per_scan":7,"rows_produced_per_join":2,"filtered":50.00,"access_type":"ALL"}}}`,
	} {
		plan, err := parseMySQLExplain(in, false)
		if err != nil {
			t.Fatalf("parse %s: %v", in, err)
		}
		if plan.EstimatedRows != 7 {
			t.Errorf("%s: EstimatedRows = %d, want 7", in, plan.EstimatedRows)
		}
	}
	if _, err := parseMySQLExplain(`{"query_block":{"table":{"filtered":"abc"}}}`, false); err == nil {
		t.Error("a non-numeric string must be a parse error, not zero")
	}
}

func TestMySQLNumber_EdgeCases(t *testing.T) {
	var f mysqlFloat
	var n mysqlInt
	for _, tc := range []struct {
		in    string
		wantF float64
		wantN int64
	}{
		{`null`, 0, 0},
		{`""`, 0, 0},
		{`"7.0"`, 7, 7},
		{` 12 `, 12, 12},
	} {
		if err := f.UnmarshalJSON([]byte(tc.in)); err != nil || float64(f) != tc.wantF {
			t.Errorf("mysqlFloat(%s) = %v, %v; want %v", tc.in, f, err, tc.wantF)
		}
		if err := n.UnmarshalJSON([]byte(tc.in)); err != nil || int64(n) != tc.wantN {
			t.Errorf("mysqlInt(%s) = %v, %v; want %v", tc.in, n, err, tc.wantN)
		}
	}
	if err := n.UnmarshalJSON([]byte(`"x"`)); err == nil {
		t.Error(`mysqlInt("x") must fail`)
	}
}
