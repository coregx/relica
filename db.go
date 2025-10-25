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
	// TxOptions represents transaction options including isolation level.
	TxOptions = core.TxOptions

	// Expression represents a database expression for building complex WHERE clauses.
	Expression = core.Expression
	// HashExp represents a hash-based expression using column-value pairs.
	HashExp = core.HashExp
	// LikeExp represents a LIKE expression with automatic escaping.
	LikeExp = core.LikeExp
)

// Re-export core functions.
var (
	Open             = core.Open
	NewDB            = core.NewDB
	WrapDB           = core.WrapDB
	WithMaxOpenConns = core.WithMaxOpenConns
	WithMaxIdleConns = core.WithMaxIdleConns

	// Expression builders
	NewExp         = core.NewExp
	Eq             = core.Eq
	NotEq          = core.NotEq
	GreaterThan    = core.GreaterThan
	LessThan       = core.LessThan
	GreaterOrEqual = core.GreaterOrEqual
	LessOrEqual    = core.LessOrEqual
	In             = core.In
	NotIn          = core.NotIn
	Between        = core.Between
	NotBetween     = core.NotBetween
	Like           = core.Like
	NotLike        = core.NotLike
	OrLike         = core.OrLike
	OrNotLike      = core.OrNotLike
	And            = core.And
	Or             = core.Or
	Not            = core.Not
	Exists         = core.Exists
	NotExists      = core.NotExists
)
