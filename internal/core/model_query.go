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

// getPrimaryKey returns primary key name and value.
func (mq *ModelQuery) getPrimaryKey() (string, interface{}) {
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	t := v.Type()

	// Search for primary key field.
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Check db tag.
		if tag, ok := field.Tag.Lookup("db"); ok {
			if tag == "id" || strings.HasSuffix(tag, "_id") {
				return tag, v.Field(i).Interface()
			}
		}

		// Fallback: field named "ID".
		if field.Name == "ID" {
			dbName := "id"
			if tag, ok := field.Tag.Lookup("db"); ok {
				dbName = tag
			}
			return dbName, v.Field(i).Interface()
		}
	}

	return "", nil
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

	// TASK-008: Remove zero-value primary key from INSERT (will be auto-populated).
	// This ensures AUTO_INCREMENT works properly.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if pkField, pkValue, err := util.FindPrimaryKeyField(v); err == nil {
		if util.IsPrimaryKeyZero(pkValue) {
			// Get PK column name from db tag or field name.
			pkCol := pkField.Tag.Get("db")
			if pkCol == "" || pkCol == "-" {
				pkCol = strings.ToLower(pkField.Name)
			}
			// Remove zero PK from INSERT.
			delete(filtered, pkCol)
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
	// Errors are silently ignored (backward compatibility) - insert succeeded,
	// ID population failure is acceptable.
	_ = mq.populatePrimaryKey(result)

	return nil
}

// populatePrimaryKey auto-populates the primary key after INSERT.
// It uses LastInsertId() for MySQL/SQLite.
// For PostgreSQL, LastInsertId() is not supported by lib/pq - handled separately.
func (mq *ModelQuery) populatePrimaryKey(result sql.Result) error {
	// 1. Find primary key field.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	_, pkValue, err := util.FindPrimaryKeyField(v)
	if err != nil {
		return nil //nolint:nilerr // Intentionally ignore - no PK or composite PK means skip auto-population.
	}

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
func (mq *ModelQuery) needsPostgresReturning() (bool, string) {
	// Check if database is PostgreSQL (check driver name, not dialect).
	driverName := mq.db.DriverName()
	if driverName != driverPostgres && driverName != driverPgx {
		return false, ""
	}

	// Find primary key field.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	pkField, pkValue, err := util.FindPrimaryKeyField(v)
	if err != nil {
		return false, "" // No PK or composite PK.
	}

	// Check if PK is zero (needs auto-population).
	if !util.IsPrimaryKeyZero(pkValue) {
		return false, "" // PK already set.
	}

	// Check if PK is numeric.
	if !isPKNumeric(pkValue) {
		return false, "" // Non-numeric PK.
	}

	// Get PK column name.
	pkCol := pkField.Tag.Get("db")
	if pkCol == "" || pkCol == "-" {
		pkCol = strings.ToLower(pkField.Name)
	}

	return true, pkCol
}

// insertWithReturning executes INSERT with PostgreSQL RETURNING clause.
// PostgreSQL lib/pq doesn't support LastInsertId(), so we use RETURNING.
func (mq *ModelQuery) insertWithReturning(query *Query, pkCol string) error {
	// Append RETURNING clause to the query.
	returningClause := " RETURNING " + mq.db.dialect.QuoteIdentifier(pkCol)
	query.appendSQL(returningClause)

	// Find primary key field to populate.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	_, pkValue, err := util.FindPrimaryKeyField(v)
	if err != nil {
		return err
	}

	// Execute query and scan returned ID.
	var id int64
	err = query.One(&id)
	if err != nil {
		return err
	}

	// Set ID back to struct.
	return util.SetPrimaryKeyValue(pkValue, id)
}

// Update updates the model in the table.
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

	// Get primary key for WHERE.
	pk, pkValue := mq.getPrimaryKey()
	if pk == "" {
		return errors.New("model: primary key not found")
	}

	// Remove PK from SET clause.
	delete(filtered, pk)

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	// Use existing Update builder.
	_, err = qb.Update(mq.table).
		Set(filtered).
		Where(pk+" = ?", pkValue).
		Execute()

	return err
}

// Delete deletes the model from the table.
func (mq *ModelQuery) Delete() error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	// Get primary key for WHERE.
	pk, pkValue := mq.getPrimaryKey()
	if pk == "" {
		return errors.New("model: primary key not found")
	}

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	// Use existing Delete builder.
	_, err := qb.Delete(mq.table).
		Where(pk+" = ?", pkValue).
		Execute()

	return err
}
