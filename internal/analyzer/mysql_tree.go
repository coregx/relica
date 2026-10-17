package analyzer

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MySQL 8.0.18+ returns EXPLAIN ANALYZE as a TREE-format text plan, one node
// per line, children indented:
//
//	-> Filter: (t.state = 1)  (cost=0.55 rows=1) (actual time=0.0383..0.0446 rows=1 loops=1)
//	    -> Table scan on t  (cost=0.55 rows=3) (actual time=0.0341..0.0404 rows=3 loops=1)
//
// Numbers are matched as \d+(\.\d+)? so that ranges like "cost=0..0" split
// correctly; row estimates may use exponent notation (rows=1e+6).
var (
	treeCostRe   = regexp.MustCompile(`\(cost=(\d+(?:\.\d+)?)(?:\.\.(\d+(?:\.\d+)?))?\s+rows=([0-9.eE+]+)\)`)
	treeActualRe = regexp.MustCompile(`\(actual time=(\d+(?:\.\d+)?)\.\.(\d+(?:\.\d+)?)\s+rows=([0-9.eE+]+)\s+loops=(\d+)\)`)
	treeIndexRe  = regexp.MustCompile(`(?i)\bindex (?:lookup|scan|range scan|skip scan)\b.*?\busing (\S+)|covering index (?:lookup|scan|range scan) on \S+ using (\S+)`)
)

var errNotATreePlan = errors.New("not a MySQL TREE-format plan")

// parseMySQLExplainTree converts EXPLAIN ANALYZE TREE output into a QueryPlan.
// Cost, estimated rows, actual rows and actual time come from the root node
// (actual time is the root's per-loop end time, as MySQL reports it).
// RowsExamined sums actual rows × loops of table and index access nodes.
func parseMySQLExplainTree(raw string) (*QueryPlan, error) {
	plan := &QueryPlan{Database: dbMySQL}
	rootFound := false

	for _, line := range strings.Split(raw, "\n") {
		node := strings.TrimSpace(line)
		if !strings.HasPrefix(node, "->") {
			continue
		}
		cost := treeCostRe.FindStringSubmatch(node)
		actual := treeActualRe.FindStringSubmatch(node)
		if !rootFound && (cost != nil || actual != nil) {
			rootFound = true
			applyTreeRoot(plan, cost, actual)
		}
		applyTreeAccess(plan, node, actual)
	}

	if !rootFound {
		return nil, errNotATreePlan
	}
	return plan, nil
}

// applyTreeRoot copies the root node's estimates and measurements.
func applyTreeRoot(plan *QueryPlan, cost, actual []string) {
	if cost != nil {
		plan.Cost = parseTreeFloat(lastNonEmpty(cost[1], cost[2]))
		plan.EstimatedRows = int64(parseTreeFloat(cost[3]))
	}
	if actual != nil {
		ms := parseTreeFloat(actual[2])
		plan.ActualTime = time.Duration(ms * float64(time.Millisecond))
		plan.ActualRows = int64(parseTreeFloat(actual[3]))
	}
}

// applyTreeAccess records full scans, index use and examined rows of a node.
func applyTreeAccess(plan *QueryPlan, node string, actual []string) {
	isTableScan := strings.Contains(strings.ToLower(node), "table scan on")
	if isTableScan {
		plan.FullScan = true
	}
	idx := treeIndexRe.FindStringSubmatch(node)
	if idx != nil {
		plan.UsesIndex = true
		if plan.IndexName == "" {
			plan.IndexName = lastNonEmpty(idx[1], idx[2])
		}
	}
	if (isTableScan || idx != nil) && actual != nil {
		loops, _ := strconv.ParseInt(actual[4], 10, 64)
		plan.RowsExamined += int64(parseTreeFloat(actual[3])) * loops
	}
}

func parseTreeFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// lastNonEmpty returns b when it is set, otherwise a. For "cost=0.1..2.5" the
// second number is the total cost.
func lastNonEmpty(a, b string) string {
	if b != "" {
		return b
	}
	return a
}
