package core

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// QueryBuilder constructs type-safe queries.
// When tx is not nil, all queries execute within that transaction.
type QueryBuilder struct {
	db  *DB
	tx  *sql.Tx         // nil for non-transactional queries
	ctx context.Context // context for all queries built by this builder
}

// WithContext sets the context for all queries built by this builder.
// The context will be used for all subsequent query operations unless overridden.
func (qb *QueryBuilder) WithContext(ctx context.Context) *QueryBuilder {
	qb.ctx = ctx
	return qb
}

// SelectQuery represents a SELECT query being built.
type SelectQuery struct {
	builder *QueryBuilder
	columns []string
	table   string
	where   []string
	params  []interface{}
	ctx     context.Context // context for this specific query
}

// WithContext sets the context for this SELECT query.
// This overrides any context set on the QueryBuilder.
func (sq *SelectQuery) WithContext(ctx context.Context) *SelectQuery {
	sq.ctx = ctx
	return sq
}

// From specifies the table to select from.
func (sq *SelectQuery) From(table string) *SelectQuery {
	sq.table = table
	return sq
}

// Where adds a WHERE condition.
// Accepts either a string with placeholders or an Expression.
//
// String example:
//
//	Where("status = ? AND age > ?", 1, 18)
//
// Expression example:
//
//	Where(relica.And(
//	    relica.Eq("status", 1),
//	    relica.GreaterThan("age", 18),
//	))
func (sq *SelectQuery) Where(condition interface{}, params ...interface{}) *SelectQuery {
	switch cond := condition.(type) {
	case string:
		// Legacy string-based WHERE (backward compatible)
		sq.where = append(sq.where, cond)
		sq.params = append(sq.params, params...)

	case Expression:
		// New Expression-based WHERE
		sqlStr, args := cond.Build(sq.builder.db.dialect)
		if sqlStr != "" {
			sq.where = append(sq.where, sqlStr)
			sq.params = append(sq.params, args...)
		}

	default:
		panic("Where() expects string or Expression")
	}

	return sq
}

// Build constructs the Query object from SelectQuery.
func (sq *SelectQuery) Build() *Query {
	// Build SELECT clause
	cols := "*"
	if len(sq.columns) > 0 {
		cols = strings.Join(sq.columns, ", ")
	}

	// Build WHERE clause
	whereClause := ""
	whereParams := sq.params
	if len(sq.where) > 0 {
		whereClause = " WHERE " + strings.Join(sq.where, " AND ")

		// Renumber WHERE placeholders for PostgreSQL ($1, $2, etc.)
		if sq.builder.db.dialect.Placeholder(1) != "?" {
			for i := range whereParams {
				placeholder := sq.builder.db.dialect.Placeholder(i + 1)
				whereClause = strings.Replace(whereClause, "?", placeholder, 1)
			}
		}
	}

	// Construct SQL
	query := "SELECT " + cols + " FROM " + sq.builder.db.dialect.QuoteIdentifier(sq.table) + whereClause

	// Context priority: query ctx > builder ctx > nil
	ctx := sq.ctx
	if ctx == nil {
		ctx = sq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: whereParams,
		db:     sq.builder.db,
		tx:     sq.builder.tx,
		ctx:    ctx,
	}
}

// One scans a single row into dest.
func (sq *SelectQuery) One(dest interface{}) error {
	return sq.Build().One(dest)
}

// All scans all rows into dest slice.
func (sq *SelectQuery) All(dest interface{}) error {
	return sq.Build().All(dest)
}

// Select starts a SELECT query.
func (qb *QueryBuilder) Select(cols ...string) *SelectQuery {
	return &SelectQuery{
		builder: qb,
		columns: cols,
	}
}

// Insert builds an INSERT query.
func (qb *QueryBuilder) Insert(table string, values map[string]interface{}) *Query {
	// Get sorted keys for deterministic SQL generation (prevents cache misses)
	keys := getKeys(values)

	placeholders := make([]string, 0, len(keys))
	params := make([]interface{}, 0, len(keys))

	for i, col := range keys {
		placeholders = append(placeholders, qb.db.dialect.Placeholder(i+1))
		params = append(params, values[col])
	}

	query := `INSERT INTO ` + qb.db.dialect.QuoteIdentifier(table) +
		` (` + strings.Join(keys, ", ") + `) ` +
		`VALUES (` + strings.Join(placeholders, ", ") + `)`

	return &Query{
		sql:    query,
		params: params,
		db:     qb.db,
		tx:     qb.tx,
		ctx:    qb.ctx,
	}
}

// getKeys returns sorted map keys for deterministic SQL generation.
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// UpsertQuery represents an UPSERT (INSERT ... ON CONFLICT/DUPLICATE) query.
type UpsertQuery struct {
	builder         *QueryBuilder
	table           string
	values          map[string]interface{}
	conflictColumns []string
	updateColumns   []string
	doNothing       bool
	ctx             context.Context // context for this specific query
}

// WithContext sets the context for this UPSERT query.
// This overrides any context set on the QueryBuilder.
func (uq *UpsertQuery) WithContext(ctx context.Context) *UpsertQuery {
	uq.ctx = ctx
	return uq
}

// Upsert creates an UPSERT query for the given table and values.
// UPSERT is INSERT with conflict resolution (UPDATE or IGNORE).
func (qb *QueryBuilder) Upsert(table string, values map[string]interface{}) *UpsertQuery {
	return &UpsertQuery{
		builder: qb,
		table:   table,
		values:  values,
	}
}

// OnConflict specifies the columns that determine a conflict.
// For PostgreSQL/SQLite: columns in UNIQUE constraint or PRIMARY KEY.
// For MySQL: this is optional (uses PRIMARY KEY or UNIQUE keys automatically).
func (uq *UpsertQuery) OnConflict(columns ...string) *UpsertQuery {
	uq.conflictColumns = columns
	return uq
}

// DoUpdate specifies which columns to update on conflict.
// If not called, defaults to updating all columns except conflict columns.
func (uq *UpsertQuery) DoUpdate(columns ...string) *UpsertQuery {
	uq.updateColumns = columns
	uq.doNothing = false
	return uq
}

// DoNothing specifies to ignore conflicts (do not update).
// This is equivalent to INSERT IGNORE in MySQL or ON CONFLICT DO NOTHING in PostgreSQL.
func (uq *UpsertQuery) DoNothing() *UpsertQuery {
	uq.doNothing = true
	uq.updateColumns = nil
	return uq
}

// Build constructs the Query object from UpsertQuery.
func (uq *UpsertQuery) Build() *Query {
	keys := getKeys(uq.values)
	placeholders := make([]string, 0, len(keys))
	params := make([]interface{}, 0, len(keys))

	for i, col := range keys {
		placeholders = append(placeholders, uq.builder.db.dialect.Placeholder(i+1))
		params = append(params, uq.values[col])
	}

	// Build base INSERT statement
	query := `INSERT INTO ` + uq.builder.db.dialect.QuoteIdentifier(uq.table) +
		` (` + strings.Join(keys, ", ") + `) ` +
		`VALUES (` + strings.Join(placeholders, ", ") + `)`

	// Add conflict resolution if specified
	if uq.doNothing {
		// PostgreSQL/SQLite: ON CONFLICT DO NOTHING
		// MySQL: needs special handling in dialect
		query += uq.builder.db.dialect.UpsertSQL(uq.table, uq.conflictColumns, nil)
	} else if len(uq.conflictColumns) > 0 || len(uq.updateColumns) > 0 {
		// If no update columns specified, update all except conflict columns
		updateCols := uq.updateColumns
		if len(updateCols) == 0 {
			updateCols = filterKeys(keys, uq.conflictColumns)
		}
		query += uq.builder.db.dialect.UpsertSQL(uq.table, uq.conflictColumns, updateCols)
	}

	// Context priority: query ctx > builder ctx > nil
	ctx := uq.ctx
	if ctx == nil {
		ctx = uq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: params,
		db:     uq.builder.db,
		tx:     uq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the UPSERT query and returns the result.
func (uq *UpsertQuery) Execute() (interface{}, error) {
	return uq.Build().Execute()
}

// filterKeys returns keys that are not in the exclude list.
func filterKeys(keys, exclude []string) []string {
	excludeMap := make(map[string]bool)
	for _, e := range exclude {
		excludeMap[e] = true
	}

	filtered := make([]string, 0, len(keys))
	for _, k := range keys {
		if !excludeMap[k] {
			filtered = append(filtered, k)
		}
	}
	return filtered
}

// UpdateQuery represents an UPDATE query being built.
type UpdateQuery struct {
	builder *QueryBuilder
	table   string
	values  map[string]interface{}
	where   []string
	params  []interface{}
	ctx     context.Context // context for this specific query
}

// WithContext sets the context for this UPDATE query.
// This overrides any context set on the QueryBuilder.
func (uq *UpdateQuery) WithContext(ctx context.Context) *UpdateQuery {
	uq.ctx = ctx
	return uq
}

// Update creates an UPDATE query for the specified table.
func (qb *QueryBuilder) Update(table string) *UpdateQuery {
	return &UpdateQuery{
		builder: qb,
		table:   table,
	}
}

// Set specifies the columns and values to update.
// Values should be a map of column names to new values.
func (uq *UpdateQuery) Set(values map[string]interface{}) *UpdateQuery {
	uq.values = values
	return uq
}

// Where adds a WHERE condition to the UPDATE query.
// Accepts either a string with placeholders or an Expression.
// Multiple Where calls are combined with AND.
//
// String example:
//
//	Where("status = ?", 1)
//
// Expression example:
//
//	Where(relica.Eq("status", 1))
func (uq *UpdateQuery) Where(condition interface{}, params ...interface{}) *UpdateQuery {
	switch cond := condition.(type) {
	case string:
		// Legacy string-based WHERE (backward compatible)
		uq.where = append(uq.where, cond)
		uq.params = append(uq.params, params...)

	case Expression:
		// New Expression-based WHERE
		sqlStr, args := cond.Build(uq.builder.db.dialect)
		if sqlStr != "" {
			uq.where = append(uq.where, sqlStr)
			uq.params = append(uq.params, args...)
		}

	default:
		panic("Where() expects string or Expression")
	}

	return uq
}

// Build constructs the Query object from UpdateQuery.
func (uq *UpdateQuery) Build() *Query {
	// Get sorted keys for deterministic SQL generation
	keys := getKeys(uq.values)

	// Build SET clause with placeholders
	setClauses := make([]string, 0, len(keys))
	setParams := make([]interface{}, 0, len(keys))

	for i, col := range keys {
		setClauses = append(setClauses, col+" = "+uq.builder.db.dialect.Placeholder(i+1))
		setParams = append(setParams, uq.values[col])
	}

	// Build WHERE clause
	whereClause := ""
	whereParams := uq.params
	if len(uq.where) > 0 {
		whereClause = " WHERE " + strings.Join(uq.where, " AND ")

		// Renumber WHERE placeholders for PostgreSQL ($1, $2, etc.)
		if uq.builder.db.dialect.Placeholder(1) != "?" {
			startIndex := len(setParams) + 1
			for i := range whereParams {
				placeholder := uq.builder.db.dialect.Placeholder(startIndex + i)
				whereClause = strings.Replace(whereClause, "?", placeholder, 1)
			}
		}
	}

	// Construct SQL
	query := "UPDATE " + uq.builder.db.dialect.QuoteIdentifier(uq.table) +
		" SET " + strings.Join(setClauses, ", ") + whereClause

	// Combine SET and WHERE parameters
	setParams = append(setParams, whereParams...)

	// Context priority: query ctx > builder ctx > nil
	ctx := uq.ctx
	if ctx == nil {
		ctx = uq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: setParams,
		db:     uq.builder.db,
		tx:     uq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the UPDATE query and returns the result.
func (uq *UpdateQuery) Execute() (interface{}, error) {
	return uq.Build().Execute()
}

// DeleteQuery represents a DELETE query being built.
type DeleteQuery struct {
	builder *QueryBuilder
	table   string
	where   []string
	params  []interface{}
	ctx     context.Context // context for this specific query
}

// WithContext sets the context for this DELETE query.
// This overrides any context set on the QueryBuilder.
func (dq *DeleteQuery) WithContext(ctx context.Context) *DeleteQuery {
	dq.ctx = ctx
	return dq
}

// Delete creates a DELETE query for the specified table.
func (qb *QueryBuilder) Delete(table string) *DeleteQuery {
	return &DeleteQuery{
		builder: qb,
		table:   table,
	}
}

// Where adds a WHERE condition to the DELETE query.
// Accepts either a string with placeholders or an Expression.
// Multiple Where calls are combined with AND.
//
// String example:
//
//	Where("id = ?", 123)
//
// Expression example:
//
//	Where(relica.Eq("id", 123))
func (dq *DeleteQuery) Where(condition interface{}, params ...interface{}) *DeleteQuery {
	switch cond := condition.(type) {
	case string:
		// Legacy string-based WHERE (backward compatible)
		dq.where = append(dq.where, cond)
		dq.params = append(dq.params, params...)

	case Expression:
		// New Expression-based WHERE
		sqlStr, args := cond.Build(dq.builder.db.dialect)
		if sqlStr != "" {
			dq.where = append(dq.where, sqlStr)
			dq.params = append(dq.params, args...)
		}

	default:
		panic("Where() expects string or Expression")
	}

	return dq
}

// Build constructs the Query object from DeleteQuery.
func (dq *DeleteQuery) Build() *Query {
	// Build WHERE clause
	whereClause := ""
	whereParams := dq.params
	if len(dq.where) > 0 {
		whereClause = " WHERE " + strings.Join(dq.where, " AND ")

		// Renumber WHERE placeholders for PostgreSQL ($1, $2, etc.)
		if dq.builder.db.dialect.Placeholder(1) != "?" {
			for i := range whereParams {
				placeholder := dq.builder.db.dialect.Placeholder(i + 1)
				whereClause = strings.Replace(whereClause, "?", placeholder, 1)
			}
		}
	}

	// Construct SQL
	query := "DELETE FROM " + dq.builder.db.dialect.QuoteIdentifier(dq.table) + whereClause

	// Context priority: query ctx > builder ctx > nil
	ctx := dq.ctx
	if ctx == nil {
		ctx = dq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: whereParams,
		db:     dq.builder.db,
		tx:     dq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the DELETE query and returns the result.
func (dq *DeleteQuery) Execute() (interface{}, error) {
	return dq.Build().Execute()
}

// BatchInsertQuery represents a batch INSERT query being built.
// It allows inserting multiple rows with a single SQL statement for performance.
type BatchInsertQuery struct {
	builder *QueryBuilder
	table   string
	columns []string
	rows    [][]interface{}
	ctx     context.Context // context for this specific query
}

// WithContext sets the context for this batch INSERT query.
// This overrides any context set on the QueryBuilder.
func (biq *BatchInsertQuery) WithContext(ctx context.Context) *BatchInsertQuery {
	biq.ctx = ctx
	return biq
}

// BatchInsert creates a batch INSERT query for the specified table and columns.
// This is optimized for inserting multiple rows in a single statement.
// Example:
//
//	db.Builder().BatchInsert("users", []string{"name", "email"}).
//	    Values("Alice", "alice@example.com").
//	    Values("Bob", "bob@example.com").
//	    Execute()
func (qb *QueryBuilder) BatchInsert(table string, columns []string) *BatchInsertQuery {
	return &BatchInsertQuery{
		builder: qb,
		table:   table,
		columns: columns,
		rows:    make([][]interface{}, 0),
	}
}

// Values adds a row of values to the batch insert.
// The number of values must match the number of columns specified in BatchInsert.
// Panics if the value count doesn't match the column count (fail fast).
func (biq *BatchInsertQuery) Values(values ...interface{}) *BatchInsertQuery {
	if len(values) != len(biq.columns) {
		panic(fmt.Sprintf("BatchInsert: expected %d values, got %d", len(biq.columns), len(values)))
	}
	biq.rows = append(biq.rows, values)
	return biq
}

// ValuesMap adds a row from a map of column names to values.
// Values are extracted in the order of columns specified in BatchInsert.
// Missing columns will have nil values.
func (biq *BatchInsertQuery) ValuesMap(values map[string]interface{}) *BatchInsertQuery {
	row := make([]interface{}, len(biq.columns))
	for i, col := range biq.columns {
		row[i] = values[col]
	}
	return biq.Values(row...)
}

// Build constructs the Query object from BatchInsertQuery.
// Generates SQL in the form: INSERT INTO table (cols) VALUES (?, ?), (?, ?), ...
// Panics if no rows have been added (fail fast).
func (biq *BatchInsertQuery) Build() *Query {
	if len(biq.rows) == 0 {
		panic("BatchInsert: no rows to insert")
	}

	// Build column list with proper quoting
	quotedColumns := make([]string, len(biq.columns))
	for i, col := range biq.columns {
		quotedColumns[i] = biq.builder.db.dialect.QuoteIdentifier(col)
	}

	// Build VALUES clause with placeholders for all rows
	valueClauses := make([]string, len(biq.rows))
	params := make([]interface{}, 0, len(biq.rows)*len(biq.columns))

	paramIndex := 1
	for i, row := range biq.rows {
		placeholders := make([]string, len(biq.columns))
		for j := 0; j < len(biq.columns); j++ {
			placeholders[j] = biq.builder.db.dialect.Placeholder(paramIndex)
			params = append(params, row[j])
			paramIndex++
		}
		valueClauses[i] = "(" + strings.Join(placeholders, ", ") + ")"
	}

	query := "INSERT INTO " + biq.builder.db.dialect.QuoteIdentifier(biq.table) +
		" (" + strings.Join(quotedColumns, ", ") + ") VALUES " +
		strings.Join(valueClauses, ", ")

	// Context priority: query ctx > builder ctx > nil
	ctx := biq.ctx
	if ctx == nil {
		ctx = biq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: params,
		db:     biq.builder.db,
		tx:     biq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the batch INSERT query and returns the result.
func (biq *BatchInsertQuery) Execute() (interface{}, error) {
	return biq.Build().Execute()
}

// BatchUpdateQuery represents a batch UPDATE query using CASE-WHEN logic.
// It updates multiple rows with different values in a single SQL statement.
type BatchUpdateQuery struct {
	builder       *QueryBuilder
	table         string
	keyColumn     string
	updates       []batchUpdateRow
	updateColumns []string        // Cached list of columns to update
	ctx           context.Context // context for this specific query
}

// WithContext sets the context for this batch UPDATE query.
// This overrides any context set on the QueryBuilder.
func (buq *BatchUpdateQuery) WithContext(ctx context.Context) *BatchUpdateQuery {
	buq.ctx = ctx
	return buq
}

// batchUpdateRow represents a single row update in a batch.
type batchUpdateRow struct {
	keyValue interface{}
	values   map[string]interface{}
}

// BatchUpdate creates a batch UPDATE query for the specified table.
// The keyColumn is used to identify which rows to update (typically the primary key).
// Example:
//
//	db.Builder().BatchUpdate("users", "id").
//	    Set(1, map[string]interface{}{"name": "Alice", "status": "active"}).
//	    Set(2, map[string]interface{}{"name": "Bob", "status": "inactive"}).
//	    Execute()
func (qb *QueryBuilder) BatchUpdate(table, keyColumn string) *BatchUpdateQuery {
	return &BatchUpdateQuery{
		builder:   qb,
		table:     table,
		keyColumn: keyColumn,
		updates:   make([]batchUpdateRow, 0),
	}
}

// Set adds a row update to the batch.
// keyValue is the value of the key column for this row.
// values contains the columns and their new values for this row.
func (buq *BatchUpdateQuery) Set(keyValue interface{}, values map[string]interface{}) *BatchUpdateQuery {
	buq.updates = append(buq.updates, batchUpdateRow{
		keyValue: keyValue,
		values:   values,
	})

	// Update the list of columns to update (union of all columns across all rows)
	if buq.updateColumns == nil {
		buq.updateColumns = getKeys(values)
	} else {
		// Add any new columns from this row
		for col := range values {
			found := false
			for _, existing := range buq.updateColumns {
				if existing == col {
					found = true
					break
				}
			}
			if !found {
				buq.updateColumns = append(buq.updateColumns, col)
			}
		}
		sort.Strings(buq.updateColumns) // Keep sorted for consistency
	}

	return buq
}

// Build constructs the Query object from BatchUpdateQuery.
// Generates SQL using CASE-WHEN for each column:
//
//	UPDATE table SET
//	  col1 = CASE key WHEN ? THEN ? WHEN ? THEN ? END,
//	  col2 = CASE key WHEN ? THEN ? WHEN ? THEN ? END
//	WHERE key IN (?, ?)
//
// Panics if no updates have been added (fail fast).
func (buq *BatchUpdateQuery) Build() *Query {
	if len(buq.updates) == 0 {
		panic("BatchUpdate: no updates to apply")
	}

	// Collect all key values for WHERE IN clause
	keyValues := make([]interface{}, len(buq.updates))
	for i, update := range buq.updates {
		keyValues[i] = update.keyValue
	}

	// Build CASE-WHEN for each column
	setClauses := make([]string, 0, len(buq.updateColumns))
	params := make([]interface{}, 0)
	paramIndex := 1

	for _, col := range buq.updateColumns {
		// Build: col = CASE key_column WHEN ? THEN ? WHEN ? THEN ? ELSE col END
		// The ELSE clause preserves the existing value for rows not being updated for this column
		quotedCol := buq.builder.db.dialect.QuoteIdentifier(col)
		caseWhen := quotedCol + " = CASE " + buq.builder.db.dialect.QuoteIdentifier(buq.keyColumn)

		for _, update := range buq.updates {
			// Only add WHEN clause if this row has this column
			if val, exists := update.values[col]; exists {
				caseWhen += " WHEN " + buq.builder.db.dialect.Placeholder(paramIndex) +
					" THEN " + buq.builder.db.dialect.Placeholder(paramIndex+1)
				params = append(params, update.keyValue, val)
				paramIndex += 2
			}
		}

		// ELSE clause preserves existing value for rows not updating this column
		caseWhen += " ELSE " + quotedCol + " END"
		setClauses = append(setClauses, caseWhen)
	}

	// Build WHERE IN clause
	whereInPlaceholders := make([]string, len(keyValues))
	for i := range keyValues {
		whereInPlaceholders[i] = buq.builder.db.dialect.Placeholder(paramIndex)
		params = append(params, keyValues[i])
		paramIndex++
	}

	query := "UPDATE " + buq.builder.db.dialect.QuoteIdentifier(buq.table) +
		" SET " + strings.Join(setClauses, ", ") +
		" WHERE " + buq.builder.db.dialect.QuoteIdentifier(buq.keyColumn) +
		" IN (" + strings.Join(whereInPlaceholders, ", ") + ")"

	// Context priority: query ctx > builder ctx > nil
	ctx := buq.ctx
	if ctx == nil {
		ctx = buq.builder.ctx
	}

	return &Query{
		sql:    query,
		params: params,
		db:     buq.builder.db,
		tx:     buq.builder.tx,
		ctx:    ctx,
	}
}

// Execute executes the batch UPDATE query and returns the result.
func (buq *BatchUpdateQuery) Execute() (interface{}, error) {
	return buq.Build().Execute()
}
