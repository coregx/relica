// Package relica provides a lightweight, type-safe database query builder for Go with
// support for PostgreSQL, MySQL, and SQLite. It offers reflection-based struct scanning,
// prepared statement caching, and OpenTelemetry tracing out of the box.
package relica

import (
	"github.com/coregx/relica/internal/core"
)

type (
	// DB represents the main database connection with caching and tracing capabilities.
	DB = core.DB
	// Option is a functional option for configuring DB.
	Option = core.Option
	// Query represents a database query with tracing.
	Query = core.Query
	// QueryBuilder constructs type-safe queries.
	QueryBuilder = core.QueryBuilder
	// SelectQuery represents a SELECT query being built.
	SelectQuery = core.SelectQuery
	// Tx represents a database transaction.
	Tx = core.Tx
)

// Re-export core functions.
var (
	Open             = core.Open
	NewDB            = core.NewDB
	WithMaxOpenConns = core.WithMaxOpenConns
	WithMaxIdleConns = core.WithMaxIdleConns
)
