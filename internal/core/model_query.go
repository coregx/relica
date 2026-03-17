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

// Upsert performs an INSERT ... ON CONFLICT DO UPDATE for the model.
//
// Auto-detects the conflict column from the primary key (same logic as Update/Delete).
// If fields are specified, only those fields are updated on conflict.
// If no fields are specified, all non-PK fields are updated on conflict.
//
// For PostgreSQL, the primary key is auto-populated after upsert (using RETURNING).
// For MySQL/SQLite, LastInsertId() is used.
//
// Composite primary keys are supported for conflict detection but do not
// support auto-population of the generated key.
//
// Example:
//
//	user := User{ID: 1, Name: "Alice", Email: "alice@example.com"}
//	err := db.Model(&user).Upsert()
//	// INSERT INTO users (email, id, name) VALUES (?, ?, ?)
//	// ON CONFLICT (id) DO UPDATE SET email=EXCLUDED.email, name=EXCLUDED.name
//
//	// Selective fields on conflict:
//	err = db.Model(&user).Upsert("name")
//	// ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name
func (mq *ModelQuery) Upsert(fields ...string) error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	// Convert struct to map.
	dataMap, err := util.StructToMap(mq.model)
	if err != nil {
		return err
	}

	// Get primary keys for conflict detection.
	pkCols, _, err := mq.getPrimaryKeys()
	if err != nil {
		return errors.New("model: primary key not found for upsert conflict detection")
	}

	// Build update columns: either specified fields or all non-PK fields.
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, fields)

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	upsertQuery := qb.Upsert(mq.table, dataMap).
		OnConflict(pkCols...).
		DoUpdate(updateCols...)

	q := upsertQuery.Build()

	// PostgreSQL: use RETURNING to auto-populate single PK.
	needsReturning, pkCol := mq.needsPostgresReturning()
	if needsReturning {
		returningClause := " RETURNING " + mq.db.dialect.QuoteIdentifier(pkCol)
		q.appendSQL(returningClause)
		return mq.scanReturningID(q, pkCol)
	}

	// MySQL/SQLite: standard Execute + LastInsertId.
	result, err := q.Execute()
	if err != nil {
		return err
	}

	_ = mq.populatePrimaryKey(result)
	return nil
}

// buildUpsertUpdateCols builds the list of columns to update on conflict.
// If fields are specified, use only those (minus any PKs).
// Otherwise, use all non-PK fields.
func (mq *ModelQuery) buildUpsertUpdateCols(dataMap map[string]interface{}, pkCols, fields []string) []string {
	pkSet := make(map[string]bool, len(pkCols))
	for _, pk := range pkCols {
		pkSet[pk] = true
	}

	if len(fields) > 0 {
		result := make([]string, 0, len(fields))
		for _, f := range fields {
			if !pkSet[f] {
				result = append(result, f)
			}
		}
		return result
	}

	// All non-PK columns.
	result := make([]string, 0, len(dataMap))
	for col := range dataMap {
		if !pkSet[col] {
			result = append(result, col)
		}
	}
	return result
}

// scanReturningID executes a query with RETURNING clause and populates the PK field.
// Used for PostgreSQL upsert with auto-increment PK.
func (mq *ModelQuery) scanReturningID(q *Query, _ string) error {
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil {
		return err
	}

	if pkInfo.IsComposite() {
		return errors.New("model: scanReturningID does not support composite primary keys")
	}

	var id int64
	if err := q.One(&id); err != nil {
		return err
	}

	return util.SetPrimaryKeyValue(pkInfo.Values[0], id)
}

// UpdateChanged updates only the fields that differ between the current model
// and the original snapshot.
//
// It compares the current model against original field by field using reflection.
// Only fields that have changed are included in the UPDATE SET clause.
// Primary key fields are always excluded from the SET clause.
//
// If nothing has changed, no query is executed and nil is returned.
//
// The original parameter must be the same type as the model passed to Model().
// It can be either a pointer or a value of the struct type.
//
// Example:
//
//	var user User
//	db.Select().From("users").Where(relica.Eq("id", 1)).One(&user)
//
//	original := user
//	user.Name = "Alice Updated"
//	user.Status = 2
//
//	err := db.Model(&user).UpdateChanged(&original)
//	// UPDATE users SET name=?, status=? WHERE id=?
func (mq *ModelQuery) UpdateChanged(original interface{}) error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	changed, err := mq.diffFields(original)
	if err != nil {
		return err
	}

	// Nothing changed — skip query.
	if len(changed) == 0 {
		return nil
	}

	// Get primary keys for WHERE clause.
	pkCols, pkValues, err := mq.getPrimaryKeys()
	if err != nil {
		return errors.New("model: primary key not found")
	}

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	updateQuery := qb.Update(mq.table).Set(changed)

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

// diffFields compares the current model with original and returns only the fields
// whose values have changed, excluding primary key fields.
//
//nolint:cyclop // Acceptable complexity for field comparison across all reflect kinds.
func (mq *ModelQuery) diffFields(original interface{}) (map[string]interface{}, error) {
	current := mq.model

	// Dereference pointers.
	currentVal := reflect.ValueOf(current)
	if currentVal.Kind() == reflect.Ptr {
		currentVal = currentVal.Elem()
	}

	origVal := reflect.ValueOf(original)
	if origVal.Kind() == reflect.Ptr {
		origVal = origVal.Elem()
	}

	// Validate both are structs.
	if currentVal.Kind() != reflect.Struct {
		return nil, errors.New("model: UpdateChanged: current model is not a struct")
	}
	if origVal.Kind() != reflect.Struct {
		return nil, errors.New("model: UpdateChanged: original is not a struct")
	}

	// Validate same type.
	if currentVal.Type() != origVal.Type() {
		return nil, errors.New("model: UpdateChanged: original type " + origVal.Type().String() +
			" does not match model type " + currentVal.Type().String())
	}

	// Collect PK columns to skip from SET.
	pkInfo, _ := util.FindPrimaryKeyFields(currentVal)
	pkSet := buildPKSet(pkInfo)

	t := currentVal.Type()
	changed := make(map[string]interface{})

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields.
		if !field.IsExported() {
			continue
		}

		// Determine db column name.
		col, skip := columnFromField(field)
		if skip {
			continue
		}

		// Skip PK columns.
		if pkSet[col] {
			continue
		}

		curField := currentVal.Field(i)
		origField := origVal.Field(i)

		if !reflect.DeepEqual(curField.Interface(), origField.Interface()) {
			changed[col] = curField.Interface()
		}
	}

	return changed, nil
}

// buildPKSet builds a set of primary key column names for fast lookup.
func buildPKSet(pkInfo *util.PrimaryKeyInfo) map[string]bool {
	if pkInfo == nil {
		return map[string]bool{}
	}
	set := make(map[string]bool, len(pkInfo.Columns))
	for _, col := range pkInfo.Columns {
		set[col] = true
	}
	return set
}

// columnFromField extracts the db column name from a struct field.
// Returns the column name and a skip flag (true means the field should be ignored).
func columnFromField(field reflect.StructField) (col string, skip bool) {
	tag, hasTag := field.Tag.Lookup("db")
	if !hasTag {
		// No db tag: use field name as-is (consistent with StructToMap).
		return field.Name, false
	}

	// Parse db tag: "column" or "column,pk" or "-".
	parts := strings.SplitN(tag, ",", 2)
	col = strings.TrimSpace(parts[0])
	if col == "-" {
		return "", true
	}
	return col, false
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
