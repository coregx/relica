package optimizer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/coregx/relica/internal/analyzer"
)

// Database dialect constants used across the optimizer package.
const (
	dialectPostgres = "postgres"
	dialectMySQL    = "mysql"
	dialectSQLite   = "sqlite"
)

// Index type constants.
const indexTypeBTree = "btree"

// Index recommendation reason constants.
const (
	reasonCompositeIndex = "Composite index for multiple AND conditions"
	reasonJoinForeignKey = "JOIN condition - index on foreign key"
)

// Operator constants for normalized WHERE clause operators.
const operatorNotIn = "NOT_IN"

// NewOptimizerForDB creates a BasicOptimizer for the given database connection.
// It automatically detects the database driver and creates the appropriate analyzer.
func NewOptimizerForDB(db *sql.DB, driverName string, threshold time.Duration) (*BasicOptimizer, error) {
	var queryAnalyzer analyzer.Analyzer

	switch driverName {
	case dialectPostgres, "postgresql":
		queryAnalyzer = analyzer.NewPostgresAnalyzer(db)
	case dialectMySQL:
		queryAnalyzer = analyzer.NewMySQLAnalyzer(db)
	case dialectSQLite, "sqlite3":
		queryAnalyzer = analyzer.NewSQLiteAnalyzer(db)
	default:
		return nil, fmt.Errorf("optimizer not supported for driver: %s", driverName)
	}

	return NewBasicOptimizer(queryAnalyzer, threshold), nil
}

// Adapter wraps Optimizer for use in core package (avoids import cycles).
type Adapter struct {
	optimizer Optimizer
}

// NewOptimizerAdapter creates a new adapter.
func NewOptimizerAdapter(optimizer Optimizer) *Adapter {
	return &Adapter{optimizer: optimizer}
}

// Analyze implements the core.Optimizer interface.
func (a *Adapter) Analyze(ctx context.Context, query string, args []interface{}, executionTime time.Duration) (interface{}, error) {
	return a.optimizer.Analyze(ctx, query, args, executionTime)
}

// Suggest implements the core.Optimizer interface.
func (a *Adapter) Suggest(analysis interface{}) []interface{} {
	// Type assertion to *Analysis
	if analysisResult, ok := analysis.(*Analysis); ok {
		suggestions := a.optimizer.Suggest(analysisResult)
		// Convert to []interface{}
		result := make([]interface{}, len(suggestions))
		for i, s := range suggestions {
			result[i] = s
		}
		return result
	}
	return nil
}
