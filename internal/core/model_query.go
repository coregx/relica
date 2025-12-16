// Package core provides the main query builder implementation.
package core

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"

	"github.com/coregx/relica/internal/util"
)

const (
	driverPostgres = "postgres"
	driverPgx      = "pgx"
)

// ModelQuery handles CRUD operations on struct models.
type ModelQuery struct {
	db      *DB
	tx      *sql.Tx // nil for non-transactional queries
	model   interface{}
	table   string
	exclude map[string]bool
}

// Model creates a new ModelQuery for the given struct.
func (db *DB) Model(model interface{}) *ModelQuery {
	return &ModelQuery{
		db:      db,
		tx:      nil,
		model:   model,
		table:   inferTableName(model),
		exclude: make(map[string]bool),
	}
}

// Model creates a ModelQuery within transaction context.
func (tx *Tx) Model(model interface{}) *ModelQuery {
	// Get the DB from the QueryBuilder stored in Tx.
	db := tx.builder.db
	return &ModelQuery{
		db:      db,
		tx:      tx.tx,
		model:   model,
		table:   inferTableName(model),
		exclude: make(map[string]bool),
	}
}

// inferTableName determines table name from struct.
func inferTableName(model interface{}) string {
	// Check for TableName() method.
	if tn, ok := model.(interface{ TableName() string }); ok {
		return tn.TableName()
	}

	// Fallback: struct name lowercased + 's'.
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	name := t.Name()
	// Simple pluralization.
	if !strings.HasSuffix(name, "s") {
		name += "s"
	}

	return strings.ToLower(name)
}

// getPrimaryKeys returns primary key column names and values.
// Supports both single PK and composite PK.
//
// Returns:
//   - columns: slice of column names in struct declaration order
//   - values: slice of values in struct declaration order
//   - error: if no primary key found
func (mq *ModelQuery) getPrimaryKeys() ([]string, []interface{}, error) {
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil {
		return nil, nil, err
	}

	values := make([]interface{}, len(pkInfo.Values))
	for i, val := range pkInfo.Values {
		values[i] = val.Interface()
	}

	return pkInfo.Columns, values, nil
}

// filterFields applies only/exclude filters.
func (mq *ModelQuery) filterFields(data map[string]interface{}, only []string) map[string]interface{} {
	result := make(map[string]interface{})

	// If only specified - take only those.
	if len(only) > 0 {
		for _, field := range only {
			if v, ok := data[field]; ok && !mq.exclude[field] {
				result[field] = v
			}
		}
		return result
	}

	// Otherwise take all except excluded.
	for k, v := range data {
		if !mq.exclude[k] {
			result[k] = v
		}
	}

	return result
}

// Table overrides the table name.
func (mq *ModelQuery) Table(name string) *ModelQuery {
	mq.table = name
	return mq
}

// Exclude excludes fields from the operation.
func (mq *ModelQuery) Exclude(attrs ...string) *ModelQuery {
	for _, attr := range attrs {
		mq.exclude[attr] = true
	}
	return mq
}

// Insert inserts the model into the table.
// If the primary key is zero (auto-increment), it will be auto-populated after insert.
//
// For composite primary keys (CPK), auto-populate is NOT supported.
// All CPK values must be provided by the caller.
func (mq *ModelQuery) Insert(attrs ...string) error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	// Convert struct to map.
	dataMap, err := util.StructToMap(mq.model)
	if err != nil {
		return err
	}

	// Apply filters.
	filtered := mq.filterFields(dataMap, attrs)

	// Get primary key info.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	pkInfo, _ := util.FindPrimaryKeyFields(v)

	// Handle PK removal for auto-increment.
	// For single PK with zero value: remove from INSERT (auto-increment).
	// For composite PK: keep all values (no auto-increment support).
	if pkInfo != nil && pkInfo.IsSingle() {
		if util.IsPrimaryKeyZero(pkInfo.Values[0]) {
			// Remove zero single PK from INSERT.
			delete(filtered, pkInfo.Columns[0])
		}
	}

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	// Build INSERT query.
	query := qb.Insert(mq.table, filtered)

	// Check if we need PostgreSQL RETURNING clause for auto-ID.
	// Only for single PK - composite PKs don't support auto-populate.
	needsReturning, pkCol := mq.needsPostgresReturning()
	if needsReturning {
		// PostgreSQL: Use RETURNING clause (lib/pq doesn't support LastInsertId).
		return mq.insertWithReturning(query, pkCol)
	}

	// MySQL/SQLite: Use standard LastInsertId().
	result, err := query.Execute()
	if err != nil {
		return err
	}

	// Auto-populate primary key (TASK-008).
	// Only for single PK - composite PKs don't support auto-populate.
	// Errors are silently ignored (backward compatibility) - insert succeeded,
	// ID population failure is acceptable.
	_ = mq.populatePrimaryKey(result)

	return nil
}

// populatePrimaryKey auto-populates the primary key after INSERT.
// It uses LastInsertId() for MySQL/SQLite.
// For PostgreSQL, LastInsertId() is not supported by lib/pq - handled separately.
//
// Only works for single PK. Composite PKs do not support auto-populate.
func (mq *ModelQuery) populatePrimaryKey(result sql.Result) error {
	// 1. Find primary key fields.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil {
		return nil //nolint:nilerr // Intentionally ignore - no PK means skip auto-population.
	}

	// Skip composite PKs - auto-populate not supported.
	if pkInfo.IsComposite() {
		return nil
	}

	pkValue := pkInfo.Values[0]

	// 2. Check if PK is zero (needs population).
	if !util.IsPrimaryKeyZero(pkValue) {
		return nil // PK already set - skip.
	}

	// 3. Check if PK is numeric (auto-increment).
	if !isPKNumeric(pkValue) {
		return nil // Non-numeric PK (string, UUID) - skip.
	}

	// 4. Get LastInsertId from result.
	id, err := result.LastInsertId()
	if err != nil {
		// PostgreSQL lib/pq doesn't support LastInsertId() - return nil for now
		// (PostgreSQL support will be added in Phase 3 if needed).
		// Note: SQLite and MySQL should support this.
		return nil //nolint:nilerr // Intentionally ignore - DB doesn't support LastInsertId (e.g., PostgreSQL).
	}

	// 5. Set ID back to struct.
	return util.SetPrimaryKeyValue(pkValue, id)
}

// isPKNumeric checks if primary key is numeric type (int/uint).
func isPKNumeric(pkValue reflect.Value) bool {
	kind := pkValue.Kind()
	if kind == reflect.Ptr {
		if pkValue.IsNil() {
			kind = pkValue.Type().Elem().Kind()
		} else {
			kind = pkValue.Elem().Kind()
		}
	}

	return kind >= reflect.Int && kind <= reflect.Int64 ||
		kind >= reflect.Uint && kind <= reflect.Uint64
}

// needsPostgresReturning checks if we need PostgreSQL RETURNING clause.
// Returns (needsReturning bool, pkColumnName string).
//
// Only returns true for single PK. Composite PKs do not support auto-populate.
func (mq *ModelQuery) needsPostgresReturning() (bool, string) {
	// Check if database is PostgreSQL (check driver name, not dialect).
	driverName := mq.db.DriverName()
	if driverName != driverPostgres && driverName != driverPgx {
		return false, ""
	}

	// Find primary key fields.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil {
		return false, "" // No PK.
	}

	// Skip composite PKs - auto-populate not supported.
	if pkInfo.IsComposite() {
		return false, ""
	}

	pkValue := pkInfo.Values[0]

	// Check if PK is zero (needs auto-population).
	if !util.IsPrimaryKeyZero(pkValue) {
		return false, "" // PK already set.
	}

	// Check if PK is numeric.
	if !isPKNumeric(pkValue) {
		return false, "" // Non-numeric PK.
	}

	return true, pkInfo.Columns[0]
}

// insertWithReturning executes INSERT with PostgreSQL RETURNING clause.
// PostgreSQL lib/pq doesn't support LastInsertId(), so we use RETURNING.
//
// Only called for single PK (composite PKs don't reach here).
func (mq *ModelQuery) insertWithReturning(query *Query, pkCol string) error {
	// Append RETURNING clause to the query.
	returningClause := " RETURNING " + mq.db.dialect.QuoteIdentifier(pkCol)
	query.appendSQL(returningClause)

	// Find primary key field to populate.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil {
		return err
	}

	// This function is only called for single PK, but check anyway.
	if pkInfo.IsComposite() {
		return errors.New("model: insertWithReturning does not support composite primary keys")
	}

	// Execute query and scan returned ID.
	var id int64
	err = query.One(&id)
	if err != nil {
		return err
	}

	// Set ID back to struct.
	return util.SetPrimaryKeyValue(pkInfo.Values[0], id)
}

// Update updates the model in the table.
// Supports both single PK and composite PK for WHERE clause.
func (mq *ModelQuery) Update(attrs ...string) error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	// Convert struct to map.
	dataMap, err := util.StructToMap(mq.model)
	if err != nil {
		return err
	}

	// Apply filters.
	filtered := mq.filterFields(dataMap, attrs)

	// Get primary keys for WHERE.
	pkCols, pkValues, err := mq.getPrimaryKeys()
	if err != nil {
		return errors.New("model: primary key not found")
	}

	// Remove all PK columns from SET clause.
	for _, col := range pkCols {
		delete(filtered, col)
	}

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	// Build UPDATE query with WHERE clause for all PK columns.
	updateQuery := qb.Update(mq.table).Set(filtered)

	// Add WHERE conditions for each PK column.
	// First condition uses Where(), subsequent use AndWhere().
	for i, col := range pkCols {
		if i == 0 {
			updateQuery = updateQuery.Where(col+" = ?", pkValues[i])
		} else {
			updateQuery = updateQuery.AndWhere(col+" = ?", pkValues[i])
		}
	}

	_, err = updateQuery.Execute()
	return err
}

// Delete deletes the model from the table.
// Supports both single PK and composite PK for WHERE clause.
func (mq *ModelQuery) Delete() error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	// Get primary keys for WHERE.
	pkCols, pkValues, err := mq.getPrimaryKeys()
	if err != nil {
		return errors.New("model: primary key not found")
	}

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	// Build DELETE query with WHERE clause for all PK columns.
	deleteQuery := qb.Delete(mq.table)

	// Add WHERE conditions for each PK column.
	// First condition uses Where(), subsequent use AndWhere().
	for i, col := range pkCols {
		if i == 0 {
			deleteQuery = deleteQuery.Where(col+" = ?", pkValues[i])
		} else {
			deleteQuery = deleteQuery.AndWhere(col+" = ?", pkValues[i])
		}
	}

	_, err = deleteQuery.Execute()
	return err
}
