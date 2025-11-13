package analyzer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// MySQLAnalyzer implements query analysis for MySQL databases.
type MySQLAnalyzer struct {
	db *sql.DB
}

// NewMySQLAnalyzer creates a new MySQL query analyzer.
func NewMySQLAnalyzer(db *sql.DB) *MySQLAnalyzer {
	return &MySQLAnalyzer{db: db}
}

// Explain analyzes the query execution plan without executing the query.
func (ma *MySQLAnalyzer) Explain(ctx context.Context, query string, args []interface{}) (*QueryPlan, error) {
	explainQuery := fmt.Sprintf("EXPLAIN FORMAT=JSON %s", query)
	return ma.executeExplain(ctx, explainQuery, args, false)
}

// ExplainAnalyze analyzes the query execution plan AND executes the query.
// MySQL 8.0.18+ supports EXPLAIN ANALYZE for actual execution metrics.
func (ma *MySQLAnalyzer) ExplainAnalyze(ctx context.Context, query string, args []interface{}) (*QueryPlan, error) {
	// Note: EXPLAIN ANALYZE requires MySQL 8.0.18+
	// For older versions, this will return an error from the database
	explainQuery := fmt.Sprintf("EXPLAIN ANALYZE %s", query)
	return ma.executeExplain(ctx, explainQuery, args, true)
}

// executeExplain runs the EXPLAIN query and parses the result.
func (ma *MySQLAnalyzer) executeExplain(ctx context.Context, explainQuery string, args []interface{}, withAnalyze bool) (*QueryPlan, error) {
	var rawJSON string
	err := ma.db.QueryRowContext(ctx, explainQuery, args...).Scan(&rawJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to execute EXPLAIN: %w", err)
	}

	plan, err := parseMySQLExplain(rawJSON, withAnalyze)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EXPLAIN output: %w", err)
	}

	plan.RawOutput = rawJSON
	plan.Database = "mysql"
	return plan, nil
}

// mysqlExplainRoot represents the root structure of MySQL EXPLAIN FORMAT=JSON output.
type mysqlExplainRoot struct {
	QueryBlock mysqlQueryBlock `json:"query_block"`
}

// mysqlQueryBlock represents the query_block node in MySQL EXPLAIN output.
type mysqlQueryBlock struct {
	SelectID   int               `json:"select_id"`
	CostInfo   mysqlCostInfo     `json:"cost_info"`
	Table      *mysqlTableAccess `json:"table"`              // Single table access
	NestedLoop *mysqlNestedLoop  `json:"nested_loop"`        // JOIN operations
	Grouping   *mysqlGrouping    `json:"grouping_operation"` // GROUP BY operations
	Ordering   *mysqlOrdering    `json:"ordering_operation"` // ORDER BY operations
}

// mysqlTableAccess represents a single table access in MySQL EXPLAIN output.
type mysqlTableAccess struct {
	TableName           string        `json:"table_name"`
	AccessType          string        `json:"access_type"` // "ALL", "index", "range", "ref", "eq_ref", "const", "system"
	PossibleKeys        []string      `json:"possible_keys"`
	Key                 string        `json:"key"` // Index name used (empty if none)
	UsedKeyParts        []string      `json:"used_key_parts"`
	KeyLength           string        `json:"key_length"`
	Ref                 []string      `json:"ref"`
	RowsExaminedPerScan int64         `json:"rows_examined_per_scan"`
	RowsProducedPerJoin int64         `json:"rows_produced_per_join"`
	Filtered            float64       `json:"filtered"` // Percentage of rows filtered by WHERE
	CostInfo            mysqlCostInfo `json:"cost_info"`
	AttachedCondition   string        `json:"attached_condition"` // WHERE clause
}

// mysqlNestedLoop represents JOIN operations in MySQL EXPLAIN output.
type mysqlNestedLoop struct {
	Table []*mysqlTableAccess `json:"table"`
}

// mysqlGrouping represents GROUP BY operations in MySQL EXPLAIN output.
type mysqlGrouping struct {
	UsingTemporaryTable bool              `json:"using_temporary_table"`
	UsingFilesort       bool              `json:"using_filesort"`
	Table               *mysqlTableAccess `json:"table"`
	NestedLoop          *mysqlNestedLoop  `json:"nested_loop"`
}

// mysqlOrdering represents ORDER BY operations in MySQL EXPLAIN output.
type mysqlOrdering struct {
	UsingFilesort bool              `json:"using_filesort"`
	Table         *mysqlTableAccess `json:"table"`
	NestedLoop    *mysqlNestedLoop  `json:"nested_loop"`
}

// mysqlCostInfo represents cost estimates in MySQL EXPLAIN output.
type mysqlCostInfo struct {
	QueryCost       string `json:"query_cost"`         // Total query cost as string
	ReadCost        string `json:"read_cost"`          // Cost of reading rows
	EvalCost        string `json:"eval_cost"`          // Cost of evaluating conditions
	PrefixCost      string `json:"prefix_cost"`        // Cumulative cost up to this point
	DataReadPerJoin string `json:"data_read_per_join"` // Amount of data read
}

// parseMySQLExplain parses MySQL EXPLAIN JSON output into a QueryPlan.
func parseMySQLExplain(rawJSON string, _ bool) (*QueryPlan, error) {
	var root mysqlExplainRoot
	if err := json.Unmarshal([]byte(rawJSON), &root); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	plan := &QueryPlan{
		Cost:          0,
		EstimatedRows: 0,
		UsesIndex:     false,
		FullScan:      false,
		Database:      "mysql",
	}

	// Extract cost from query_block
	if cost, err := parseFloatOrZero(root.QueryBlock.CostInfo.QueryCost); err == nil {
		plan.Cost = cost
	}

	// Extract metrics from table access patterns
	extractMySQLMetrics(&root.QueryBlock, plan)

	return plan, nil
}

// extractMySQLMetrics recursively extracts metrics from MySQL query plan.
func extractMySQLMetrics(queryBlock *mysqlQueryBlock, plan *QueryPlan) {
	// Process single table access
	if queryBlock.Table != nil {
		updateMySQLTableMetrics(queryBlock.Table, plan)
	}

	// Process nested loop (JOINs)
	if queryBlock.NestedLoop != nil {
		processNestedLoop(queryBlock.NestedLoop, plan)
	}

	// Process grouping operation
	if queryBlock.Grouping != nil {
		processGrouping(queryBlock.Grouping, plan)
	}

	// Process ordering operation
	if queryBlock.Ordering != nil {
		processOrdering(queryBlock.Ordering, plan)
	}
}

// processNestedLoop processes JOIN operations in nested loop format.
func processNestedLoop(nestedLoop *mysqlNestedLoop, plan *QueryPlan) {
	for _, table := range nestedLoop.Table {
		updateMySQLTableMetrics(table, plan)
	}
}

// processGrouping processes GROUP BY operations.
func processGrouping(grouping *mysqlGrouping, plan *QueryPlan) {
	if grouping.Table != nil {
		updateMySQLTableMetrics(grouping.Table, plan)
	}
	if grouping.NestedLoop != nil {
		processNestedLoop(grouping.NestedLoop, plan)
	}
}

// processOrdering processes ORDER BY operations.
func processOrdering(ordering *mysqlOrdering, plan *QueryPlan) {
	if ordering.Table != nil {
		updateMySQLTableMetrics(ordering.Table, plan)
	}
	if ordering.NestedLoop != nil {
		processNestedLoop(ordering.NestedLoop, plan)
	}
}

// updateMySQLTableMetrics updates plan metrics from a single table access.
func updateMySQLTableMetrics(table *mysqlTableAccess, plan *QueryPlan) {
	if table == nil {
		return
	}

	// Detect index usage (key field is not empty)
	if table.Key != "" {
		plan.UsesIndex = true
		if plan.IndexName == "" {
			plan.IndexName = table.Key
		}
	}

	// Detect full table scan (access_type = "ALL")
	if table.AccessType == "ALL" {
		plan.FullScan = true
	}

	// Accumulate row estimates
	if table.RowsExaminedPerScan > 0 {
		plan.EstimatedRows += table.RowsExaminedPerScan
		plan.RowsExamined += table.RowsExaminedPerScan
	}

	if table.RowsProducedPerJoin > 0 {
		plan.RowsProduced += table.RowsProducedPerJoin
	}
}

// parseFloatOrZero parses a string to float64, returns 0 on error.
func parseFloatOrZero(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
