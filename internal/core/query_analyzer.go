package core

import (
	"context"
	"fmt"
	"os"
	"time"
)

// analyzeQuery performs query optimization analysis asynchronously.
// This method is called in a goroutine to avoid blocking query execution.
// The ctx parameter is intentionally unused as we create a new timeout context.
func (q *Query) analyzeQuery(_ context.Context, executionTime time.Duration) {
	// Use a timeout context to prevent hanging
	analyzeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call optimizer.Analyze
	analysis, err := q.db.optimizer.Analyze(analyzeCtx, q.sql, q.params, executionTime)
	if err != nil {
		// Log error but don't fail the query
		fmt.Fprintf(os.Stderr, "[RELICA OPTIMIZER] Failed to analyze query: %v\n", err)
		return
	}

	// Get suggestions
	suggestions := q.db.optimizer.Suggest(analysis)
	if len(suggestions) == 0 {
		return
	}

	// Output suggestions (in production, this would use a proper logger)
	// We use reflection-free type switches to extract suggestion fields
	for _, s := range suggestions {
		// Extract fields using struct field accessors
		// Format: [RELICA OPTIMIZER] severity: message
		msg := fmt.Sprintf("%v", s)
		fmt.Fprintf(os.Stderr, "[RELICA OPTIMIZER] %s\n", msg)
	}
}
