package analyzer

import (
	"testing"
	"time"
)

// MySQL 8.0.18+ prints EXPLAIN ANALYZE only in TREE format (text). The
// analyzer used to feed it to the JSON parser, so ExplainAnalyze failed on
// every MySQL server.

const mysqlTreeFullScan = `-> Filter: (pt_item.state = 1)  (cost=0.55 rows=1) (actual time=0.0383..0.0446 rows=1 loops=1)
    -> Table scan on pt_item  (cost=0.55 rows=3) (actual time=0.0341..0.0404 rows=3 loops=1)
`

const mysqlTreeIndex = `-> Index lookup on o using idx_user (user_id=u.id)  (cost=0.35 rows=2) (actual time=0.012..0.015 rows=2 loops=3)
`

const mysqlTreeNestedLoop = `-> Nested loop inner join  (cost=2.20 rows=6) (actual time=0.0611..0.0925 rows=6 loops=1)
    -> Table scan on u  (cost=0.55 rows=3) (actual time=0.0322..0.0387 rows=3 loops=1)
    -> Index lookup on o using idx_user (user_id=u.id)  (cost=0.35 rows=2) (actual time=0.012..0.015 rows=2 loops=3)
`

func TestParseMySQLExplainTree(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		cost       float64
		estRows    int64
		actualRows int64
		actualTime time.Duration
		fullScan   bool
		usesIndex  bool
		indexName  string
	}{
		{"full scan with filter", mysqlTreeFullScan, 0.55, 1, 1, 44600 * time.Nanosecond, true, false, ""},
		{"index lookup", mysqlTreeIndex, 0.35, 2, 2, 15 * time.Microsecond, false, true, "idx_user"},
		{"nested loop", mysqlTreeNestedLoop, 2.20, 6, 6, 92500 * time.Nanosecond, true, true, "idx_user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parseMySQLExplainTree(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if p.Cost != tt.cost || p.EstimatedRows != tt.estRows || p.ActualRows != tt.actualRows {
				t.Errorf("cost/est/actual = %v/%d/%d, want %v/%d/%d",
					p.Cost, p.EstimatedRows, p.ActualRows, tt.cost, tt.estRows, tt.actualRows)
			}
			if p.ActualTime != tt.actualTime {
				t.Errorf("ActualTime = %v, want %v", p.ActualTime, tt.actualTime)
			}
			if p.FullScan != tt.fullScan || p.UsesIndex != tt.usesIndex || p.IndexName != tt.indexName {
				t.Errorf("fullScan/usesIndex/index = %v/%v/%q, want %v/%v/%q",
					p.FullScan, p.UsesIndex, p.IndexName, tt.fullScan, tt.usesIndex, tt.indexName)
			}
			if p.Database != dbMySQL {
				t.Errorf("Database = %q", p.Database)
			}
		})
	}
}

func TestParseMySQLExplainTree_Errors(t *testing.T) {
	for _, in := range []string{"", "not a plan", `{"query_block":{}}`} {
		if _, err := parseMySQLExplainTree(in); err == nil {
			t.Errorf("parseMySQLExplainTree(%q) = nil error", in)
		}
	}
}

// A node without measurements (e.g. "Rows fetched before execution") must not
// break the parse; metrics come from the first node that has them.
func TestParseMySQLExplainTree_CostRange(t *testing.T) {
	p, err := parseMySQLExplainTree("-> Rows fetched before execution  (cost=0..0 rows=1) (actual time=0..0 rows=1 loops=1)\n")
	if err != nil {
		t.Fatal(err)
	}
	if p.Cost != 0 || p.EstimatedRows != 1 || p.ActualRows != 1 {
		t.Errorf("%+v", p)
	}
}
