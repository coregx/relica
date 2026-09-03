// Package core provides the main query builder implementation.
package core

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"

	"github.com/coregx/relica/internal/util"
)

const (
	driverPostgres = "postgres"
	driverPgx      = "pgx"
	driverSQLite   = "sqlite"
	driverSQLite3  = "sqlite3"
)

// ModelQuery handles CRUD operations on struct models.
type ModelQuery struct {
	db      *DB
	tx      *sql.Tx // nil for non-transactional queries
	model   any
	table   string
	exclude map[string]bool
	ctx     context.Context // nil means use background context
}

// WithContext returns a new ModelQuery with the given context.
func (mq *ModelQuery) WithContext(ctx context.Context) *ModelQuery {
	newMQ := *mq
	newMQ.ctx = ctx
	newMQ.exclude = make(map[string]bool, len(mq.exclude))
	for k, v := range mq.exclude {
		newMQ.exclude[k] = v
	}
	return &newMQ
}

// Model creates a new ModelQuery for the given struct.
func (db *DB) Model(model any) *ModelQuery {
	return &ModelQuery{
		db:      db,
		tx:      nil,
		model:   model,
		table:   inferTableName(model),
		exclude: make(map[string]bool),
	}
}

// Model creates a ModelQuery within transaction context.
func (tx *Tx) Model(model any) *ModelQuery {
	db := tx.builder.db
	return &ModelQuery{
		db:      db,
		tx:      tx.tx,
		model:   model,
		table:   inferTableName(model),
		exclude: make(map[string]bool),
		ctx:     tx.ctx,
	}
}

// inferTableName determines table name from struct.
// Returns an empty string if model is nil; callers that need the table name
// must handle the empty-string case (ModelQuery operations return an error).
func inferTableName(model any) string {
	if model == nil {
		return ""
	}

	// Check for TableName() method.
	if tn, ok := model.(interface{ TableName() string }); ok {
		return tn.TableName()
	}

	// Fallback: struct name lowercased + 's'.
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Pointer {
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
func (mq *ModelQuery) getPrimaryKeys() ([]string, []any, error) {
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil {
		return nil, nil, err
	}

	values := make([]any, len(pkInfo.Values))
	for i, val := range pkInfo.Values {
		values[i] = val.Interface()
	}

	return pkInfo.Columns, values, nil
}

// filterFields applies only/exclude filters.
func (mq *ModelQuery) filterFields(data map[string]any, only []string) map[string]any {
	result := make(map[string]any)

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

// BeforeInserter is implemented by models that need pre-insert logic
// (timestamps, validation, custom ID generation).
// Called before autoid generation — if BeforeInsert sets an autoid field, it won't be overwritten.
type BeforeInserter interface {
	BeforeInsert() error
}

// callBeforeInsert calls BeforeInsert() if the model implements BeforeInserter.
func (mq *ModelQuery) callBeforeInsert() error {
	if hook, ok := mq.model.(BeforeInserter); ok {
		return hook.BeforeInsert()
	}
	return nil
}

// populateAutoIDFields generates IDs for autoid-tagged fields that are empty.
// Must be called before StructToMap so generated values are included in the INSERT.
func (mq *ModelQuery) populateAutoIDFields() {
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	fields := util.FindAutoIDFields(v)
	if len(fields) == 0 {
		return
	}

	for _, f := range fields {
		fv := v.Field(f.FieldIndex)
		if fv.Kind() != reflect.String || fv.String() != "" {
			continue
		}
		fv.SetString(util.GenerateAutoID(f.Prefix, f.Generator))
	}
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

	if err := mq.callBeforeInsert(); err != nil {
		return err
	}

	mq.populateAutoIDFields()

	// Convert struct to map.
	dataMap, err := util.StructToMap(mq.model)
	if err != nil {
		return err
	}

	// Apply filters.
	filtered := mq.filterFields(dataMap, attrs)

	// Get primary key info.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Pointer {
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

	// Create builder with transaction/query context if applicable.
	qb := &QueryBuilder{
		db:  mq.db,
		tx:  mq.tx,
		ctx: mq.ctx,
	}

	// Build INSERT query.
	query := qb.Insert(mq.table, filtered)

	// Check if we need PostgreSQL RETURNING clause for auto-ID.
	// Only for single PK - composite PKs don't support auto-populate.
	needsReturning, pkCol := mq.needsReturning()
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
// Non-numeric PKs (string/UUID) with autoincrement tag are skipped here:
// MySQL LastInsertId() returns int64 only, so server-generated string PKs
// (e.g. UUID via DEFAULT gen_random_uuid()) cannot be populated via this path.
func (mq *ModelQuery) populatePrimaryKey(result sql.Result) error {
	// 1. Find primary key fields.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Pointer {
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

	// 3. Check if PK is numeric (auto-increment by convention).
	// Non-numeric PKs with autoincrement tag are not supported via LastInsertId()
	// since MySQL/SQLite LastInsertId() returns int64 only.
	if !isPKNumeric(pkValue) {
		return nil // Non-numeric PK (string, UUID) - skip for MySQL/SQLite.
	}

	// 4. Get LastInsertId from result.
	id, err := result.LastInsertId()
	if err != nil {
		// PostgreSQL lib/pq doesn't support LastInsertId() - return nil.
		// Note: SQLite and MySQL should support this.
		return nil //nolint:nilerr // Intentionally ignore - DB doesn't support LastInsertId (e.g., PostgreSQL).
	}

	// 5. Set ID back to struct.
	return util.SetPrimaryKeyValue(pkValue, id)
}

// isPKNumeric checks if primary key is numeric type (int/uint).
func isPKNumeric(pkValue reflect.Value) bool {
	kind := pkValue.Kind()
	if kind == reflect.Pointer {
		if pkValue.IsNil() {
			kind = pkValue.Type().Elem().Kind()
		} else {
			kind = pkValue.Elem().Kind()
		}
	}

	return kind >= reflect.Int && kind <= reflect.Int64 ||
		kind >= reflect.Uint && kind <= reflect.Uint64
}

// needsReturning checks if we need the RETURNING clause for auto-ID population.
// PostgreSQL: always (lib/pq doesn't support LastInsertId).
// SQLite: for upsert and insert (last_insert_rowid() not updated on ON CONFLICT DO UPDATE).
//
// Returns true for single PK when:
//   - PK is numeric (auto-increment by convention), OR
//   - PK is non-numeric (string/UUID) with explicit `autoincrement` tag.
//
// Composite PKs do not support auto-populate.
func (mq *ModelQuery) needsReturning() (bool, string) {
	driverName := mq.db.DriverName()
	switch driverName {
	case driverPostgres, driverPgx:
		// PostgreSQL: always use RETURNING (lib/pq doesn't support LastInsertId).
	case driverSQLite, driverSQLite3:
		// SQLite 3.35+: use RETURNING (last_insert_rowid() not updated on upsert UPDATE path).
	default:
		return false, "" // MySQL: uses LastInsertId (works for upsert).
	}

	// Find primary key fields.
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Pointer {
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

	// Numeric PKs use RETURNING by convention (auto-increment).
	if isPKNumeric(pkValue) {
		return true, pkInfo.Columns[0]
	}

	// Non-numeric PKs (string, UUID) require explicit autoincrement tag
	// to opt into server-side ID generation via RETURNING.
	if pkInfo.AutoIncrement {
		return true, pkInfo.Columns[0]
	}

	return false, ""
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
	if v.Kind() == reflect.Pointer {
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

	pkField := pkInfo.Values[0]
	return scanReturningIntoField(query, pkField)
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

	// Create builder with transaction/query context if applicable.
	qb := &QueryBuilder{
		db:  mq.db,
		tx:  mq.tx,
		ctx: mq.ctx,
	}

	// Build UPDATE query with WHERE clause for all PK columns.
	updateQuery := qb.Update(mq.table).Set(filtered)

	for i, col := range pkCols {
		if i == 0 {
			updateQuery = updateQuery.Where(Eq(col, pkValues[i]))
		} else {
			updateQuery = updateQuery.AndWhere(Eq(col, pkValues[i]))
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

	if err := mq.callBeforeInsert(); err != nil {
		return err
	}

	mq.populateAutoIDFields()

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

	// Remove zero single PK from INSERT (let database auto-increment).
	mq.removeZeroPK(dataMap)

	// Build update columns: either specified fields or all non-PK fields.
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, fields)

	// Create builder with transaction/query context if applicable.
	qb := &QueryBuilder{
		db:  mq.db,
		tx:  mq.tx,
		ctx: mq.ctx,
	}

	q := qb.Upsert(mq.table, dataMap).
		OnConflict(pkCols...).
		DoUpdate(updateCols...).
		Build()

	return mq.executeUpsertQuery(q)
}

// UpsertOn performs INSERT ... ON CONFLICT with custom conflict columns.
// Use when the conflict target is a UNIQUE constraint, not the primary key.
//
// Example:
//
//	// Schema: UNIQUE(project_id, qualified_name)
//	db.Model(&node).UpsertOn([]string{"project_id", "qualified_name"}, "name", "label")
//	// ON CONFLICT ("project_id", "qualified_name") DO UPDATE SET name=EXCLUDED.name, label=EXCLUDED.label
func (mq *ModelQuery) UpsertOn(conflictColumns []string, fields ...string) error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	if len(conflictColumns) == 0 {
		return errors.New("model: UpsertOn requires at least one conflict column")
	}

	if err := mq.callBeforeInsert(); err != nil {
		return err
	}

	mq.populateAutoIDFields()

	dataMap, err := util.StructToMap(mq.model)
	if err != nil {
		return err
	}

	// Remove zero single PK from INSERT (let database auto-increment).
	mq.removeZeroPK(dataMap)

	updateCols := mq.buildUpsertUpdateCols(dataMap, conflictColumns, fields)

	qb := &QueryBuilder{
		db:  mq.db,
		tx:  mq.tx,
		ctx: mq.ctx,
	}

	q := qb.Upsert(mq.table, dataMap).
		OnConflict(conflictColumns...).
		DoUpdate(updateCols...).
		Build()

	return mq.executeUpsertQuery(q)
}

// removeZeroPK removes zero single PK from dataMap so database auto-increments.
func (mq *ModelQuery) removeZeroPK(dataMap map[string]any) {
	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil || pkInfo == nil || !pkInfo.IsSingle() {
		return
	}
	if util.IsPrimaryKeyZero(pkInfo.Values[0]) {
		delete(dataMap, pkInfo.Columns[0])
	}
}

// executeUpsertQuery handles RETURNING (PostgreSQL/SQLite) or LastInsertId (MySQL).
func (mq *ModelQuery) executeUpsertQuery(q *Query) error {
	needsReturning, pkCol := mq.needsReturning()
	if needsReturning {
		q.appendSQL(" RETURNING " + mq.db.dialect.QuoteIdentifier(pkCol))
		return mq.scanReturningID(q, pkCol)
	}

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
func (mq *ModelQuery) buildUpsertUpdateCols(dataMap map[string]any, pkCols, fields []string) []string {
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
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	pkInfo, err := util.FindPrimaryKeyFields(v)
	if err != nil {
		return err
	}

	if pkInfo.IsComposite() {
		return errors.New("model: scanReturningID does not support composite primary keys")
	}

	return scanReturningIntoField(q, pkInfo.Values[0])
}

// scanReturningIntoField executes q.Row() and writes the returned value into pkField.
// Supports int/uint families (via SetPrimaryKeyValue) and string (direct scan).
func scanReturningIntoField(q *Query, pkField reflect.Value) error {
	kind := pkField.Kind()
	if kind == reflect.Pointer {
		if pkField.IsNil() {
			pkField.Set(reflect.New(pkField.Type().Elem()))
		}
		return scanReturningIntoField(q, pkField.Elem())
	}

	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var v int64
		if err := q.Row(&v); err != nil {
			return err
		}
		return util.SetPrimaryKeyValue(pkField, v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var v int64
		if err := q.Row(&v); err != nil {
			return err
		}
		return util.SetPrimaryKeyValue(pkField, v)
	case reflect.String:
		var v string
		if err := q.Row(&v); err != nil {
			return err
		}
		pkField.SetString(v)
		return nil
	default:
		return errors.New("model: unsupported PK type for RETURNING: " + kind.String())
	}
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
func (mq *ModelQuery) UpdateChanged(original any) error {
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

	// Create builder with transaction/query context if applicable.
	qb := &QueryBuilder{
		db:  mq.db,
		tx:  mq.tx,
		ctx: mq.ctx,
	}

	updateQuery := qb.Update(mq.table).Set(changed)

	for i, col := range pkCols {
		if i == 0 {
			updateQuery = updateQuery.Where(Eq(col, pkValues[i]))
		} else {
			updateQuery = updateQuery.AndWhere(Eq(col, pkValues[i]))
		}
	}

	_, err = updateQuery.Execute()
	return err
}

// diffFields compares the current model with original and returns only the fields
// whose values have changed, excluding primary key fields.
//
//nolint:cyclop // Acceptable complexity for field comparison across all reflect kinds.
func (mq *ModelQuery) diffFields(original any) (map[string]any, error) {
	current := mq.model

	// Dereference pointers.
	currentVal := reflect.ValueOf(current)
	if currentVal.Kind() == reflect.Pointer {
		currentVal = currentVal.Elem()
	}

	origVal := reflect.ValueOf(original)
	if origVal.Kind() == reflect.Pointer {
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
	changed := make(map[string]any)

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

	// Create builder with transaction/query context if applicable.
	qb := &QueryBuilder{
		db:  mq.db,
		tx:  mq.tx,
		ctx: mq.ctx,
	}

	// Build DELETE query with WHERE clause for all PK columns.
	deleteQuery := qb.Delete(mq.table)

	for i, col := range pkCols {
		if i == 0 {
			deleteQuery = deleteQuery.Where(Eq(col, pkValues[i]))
		} else {
			deleteQuery = deleteQuery.AndWhere(Eq(col, pkValues[i]))
		}
	}

	_, err = deleteQuery.Execute()
	return err
}

// FindByPublicID finds a record by its autoid-tagged field value.
// Validates the prefix if the autoid tag specifies one.
// Scans the result into the model struct (same as One).
func (mq *ModelQuery) FindByPublicID(publicID string) error {
	if mq.table == "" {
		return errors.New("model: table name not specified")
	}

	if publicID == "" {
		return errors.New("model: public ID is empty")
	}

	v := reflect.ValueOf(mq.model)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	fields := util.FindAutoIDFields(v)
	if len(fields) == 0 {
		return errors.New("model: no autoid field found")
	}

	f := fields[0]

	if err := util.ValidateAutoIDPrefix(publicID, f.Prefix); err != nil {
		return err
	}

	qb := &QueryBuilder{
		db:  mq.db,
		tx:  mq.tx,
		ctx: mq.ctx,
	}

	return qb.Select().
		From(mq.table).
		Where(Eq(f.Column, publicID)).
		Limit(1).
		One(mq.model)
}
