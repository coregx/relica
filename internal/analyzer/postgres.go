package analyzer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PostgresAnalyzer implements query analysis for PostgreSQL databases.
type PostgresAnalyzer struct {
	db *sql.DB
}

// NewPostgresAnalyzer creates a new PostgreSQL query analyzer.
func NewPostgresAnalyzer(db *sql.DB) *PostgresAnalyzer {
	return &PostgresAnalyzer{db: db}
}

// Explain analyzes the query execution plan without executing the query.
func (pa *PostgresAnalyzer) Explain(ctx context.Context, query string, args []interface{}) (*QueryPlan, error) {
	explainQuery := fmt.Sprintf("EXPLAIN (FORMAT JSON) %s", query)
	return pa.executeExplain(ctx, explainQuery, args, false)
}

// ExplainAnalyze analyzes the query execution plan AND executes the query.
func (pa *PostgresAnalyzer) ExplainAnalyze(ctx context.Context, query string, args []interface{}) (*QueryPlan, error) {
	explainQuery := fmt.Sprintf("EXPLAIN (ANALYZE, FORMAT JSON, BUFFERS) %s", query)
	return pa.executeExplain(ctx, explainQuery, args, true)
}

// executeExplain runs the EXPLAIN query and parses the result.
func (pa *PostgresAnalyzer) executeExplain(ctx context.Context, explainQuery string, args []interface{}, withAnalyze bool) (*QueryPlan, error) {
	var rawJSON string
	err := pa.db.QueryRowContext(ctx, explainQuery, args...).Scan(&rawJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to execute EXPLAIN: %w", err)
	}

	plan, err := parsePostgresExplain(rawJSON, withAnalyze)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EXPLAIN output: %w", err)
	}

	plan.RawOutput = rawJSON
	plan.Database = "postgres"
	return plan, nil
}

// postgresExplainRoot represents the root structure of PostgreSQL EXPLAIN JSON output.
type postgresExplainRoot struct {
	Plan          postgresExplainPlan `json:"Plan"`
	PlanningTime  float64             `json:"Planning Time"`  // milliseconds
	ExecutionTime float64             `json:"Execution Time"` // milliseconds (EXPLAIN ANALYZE only)
	Triggers      []json.RawMessage   `json:"Triggers"`       // optional
	JITTime       float64             `json:"JIT Time"`       // optional
}

// postgresExplainPlan represents a plan node in PostgreSQL EXPLAIN output.
type postgresExplainPlan struct {
	NodeType         string                `json:"Node Type"`           // "Seq Scan", "Index Scan", etc.
	RelationName     string                `json:"Relation Name"`       // table name
	Alias            string                `json:"Alias"`               // table alias
	ParentRelation   string                `json:"Parent Relationship"` // optional
	JoinType         string                `json:"Join Type"`           // optional
	IndexName        string                `json:"Index Name"`          // optional
	IndexCond        string                `json:"Index Cond"`          // optional
	Filter           string                `json:"Filter"`              // optional
	TotalCost        float64               `json:"Total Cost"`          // estimated total cost
	StartupCost      float64               `json:"Startup Cost"`        // estimated startup cost
	PlanRows         int64                 `json:"Plan Rows"`           // estimated rows
	PlanWidth        int                   `json:"Plan Width"`          // estimated row width in bytes
	ActualRows       int64                 `json:"Actual Rows"`         // actual rows (EXPLAIN ANALYZE only)
	ActualLoops      int                   `json:"Actual Loops"`        // number of times executed
	ActualTotalTime  float64               `json:"Actual Total Time"`   // milliseconds (EXPLAIN ANALYZE only)
	SharedHitBlocks  int64                 `json:"Shared Hit Blocks"`   // buffer cache hits
	SharedReadBlocks int64                 `json:"Shared Read Blocks"`  // buffer cache misses
	Plans            []postgresExplainPlan `json:"Plans"`               // child plans
}

// parsePostgresExplain parses PostgreSQL EXPLAIN JSON output into a QueryPlan.
func parsePostgresExplain(rawJSON string, withAnalyze bool) (*QueryPlan, error) {
	// PostgreSQL returns an array with single element
	var explainArray []postgresExplainRoot
	if err := json.Unmarshal([]byte(rawJSON), &explainArray); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	if len(explainArray) == 0 {
		return nil, fmt.Errorf("empty EXPLAIN output")
	}

	root := explainArray[0]
	plan := &QueryPlan{
		Cost:          root.Plan.TotalCost,
		EstimatedRows: root.Plan.PlanRows,
		UsesIndex:     false,
		FullScan:      false,
		Database:      "postgres",
	}

	// Extract metrics from plan tree
	extractPostgresMetrics(&root.Plan, plan, withAnalyze)

	// Set actual execution time (EXPLAIN ANALYZE only)
	if withAnalyze && root.ExecutionTime > 0 {
		plan.ActualTime = time.Duration(root.ExecutionTime * float64(time.Millisecond))
	}

	return plan, nil
}

// extractPostgresMetrics recursively extracts metrics from PostgreSQL plan tree.
func extractPostgresMetrics(node *postgresExplainPlan, plan *QueryPlan, withAnalyze bool) {
	// Check node type for index usage and full scans
	updateIndexInfo(node, plan)

	// Accumulate buffer statistics (EXPLAIN ANALYZE with BUFFERS)
	if withAnalyze {
		updateActualMetrics(node, plan)
	}

	// Recursively process child plans
	for i := range node.Plans {
		extractPostgresMetrics(&node.Plans[i], plan, withAnalyze)
	}
}

// updateIndexInfo updates plan with index usage information from node.
func updateIndexInfo(node *postgresExplainPlan, plan *QueryPlan) {
	// Check for index scan types
	isIndexScan := strings.Contains(node.NodeType, "Index Scan") ||
		strings.Contains(node.NodeType, "Index Only Scan") ||
		strings.Contains(node.NodeType, "Bitmap Index Scan")

	if isIndexScan {
		plan.UsesIndex = true
		if plan.IndexName == "" && node.IndexName != "" {
			plan.IndexName = node.IndexName
		}
	}

	// Check for sequential scan
	if node.NodeType == "Seq Scan" {
		plan.FullScan = true
	}
}

// updateActualMetrics updates plan with actual execution metrics from node.
func updateActualMetrics(node *postgresExplainPlan, plan *QueryPlan) {
	if node.ActualRows > 0 {
		plan.ActualRows += node.ActualRows * int64(node.ActualLoops)
	}
	plan.BuffersHit += node.SharedHitBlocks
	plan.BuffersMiss += node.SharedReadBlocks
}
