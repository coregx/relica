// Package core provides the main query builder implementation.
package core

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"

	"github.com/coregx/relica/internal/util"
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

	// Create builder with transaction context if applicable.
	qb := &QueryBuilder{
		db: mq.db,
		tx: mq.tx,
	}

	// Use existing Insert builder.
	_, err = qb.Insert(mq.table, filtered).Execute()
	return err
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
