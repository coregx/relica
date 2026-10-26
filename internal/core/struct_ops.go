package core

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/coregx/relica/internal/util"
)

// InsertStruct builds an INSERT of every column of data (a struct or *struct)
// under the DB's Naming, embedded structs flattened. A zero single-column
// primary key is left out so the database generates it.
func (qb *QueryBuilder) InsertStruct(table string, data any) *Query {
	values, err := qb.db.structValues(data)
	if err != nil {
		return qb.errQuery(err)
	}
	v, _ := structValue(data)
	if pk, err := qb.db.primaryKey(v); err == nil && pk.IsSingle() && util.IsPrimaryKeyZero(pk.Values[0]) {
		delete(values, pk.Columns[0])
	}
	return qb.Insert(table, values)
}

// BatchInsertStruct builds one multi-row INSERT from a slice of structs or
// struct pointers. The primary key column is left out only when it is a single
// column and zero in every row; a mix of zero and non-zero keys is an error.
func (qb *QueryBuilder) BatchInsertStruct(table string, data any) *Query {
	rows := reflect.ValueOf(data)
	if rows.Kind() != reflect.Slice {
		return qb.errQuery(fmt.Errorf("relica: BatchInsertStruct expects a slice, got %T", data))
	}
	if rows.Len() == 0 {
		return qb.errQuery(errors.New("relica: BatchInsertStruct: empty slice"))
	}

	maps, skipPK, err := qb.batchRows(rows)
	if err != nil {
		return qb.errQuery(err)
	}

	columns := make([]string, 0, len(maps[0]))
	for col := range maps[0] {
		if col != skipPK {
			columns = append(columns, col)
		}
	}
	sort.Strings(columns)

	biq := qb.BatchInsert(table, columns)
	for _, m := range maps {
		values := make([]any, len(columns))
		for j, col := range columns {
			values[j] = m[col]
		}
		biq = biq.Values(values...)
	}
	return biq.Build()
}

// batchRows reads every row's columns and returns the single-column primary
// key to leave out ("" = keep it): only when it is zero in every row.
func (qb *QueryBuilder) batchRows(rows reflect.Value) ([]map[string]any, string, error) {
	maps := make([]map[string]any, rows.Len())
	zeroPK := 0
	var pkColumn string
	for i := range maps {
		elem := rows.Index(i).Interface()
		m, err := qb.db.structValues(elem)
		if err != nil {
			return nil, "", fmt.Errorf("relica: BatchInsertStruct row %d: %w", i, err)
		}
		maps[i] = m
		v, _ := structValue(elem)
		if pk, err := qb.db.primaryKey(v); err == nil && pk.IsSingle() {
			pkColumn = pk.Columns[0]
			if util.IsPrimaryKeyZero(pk.Values[0]) {
				zeroPK++
			}
		}
	}
	switch {
	case zeroPK == 0:
		return maps, "", nil
	case zeroPK < len(maps):
		return nil, "", fmt.Errorf("relica: BatchInsertStruct: %d of %d rows have a zero primary key %q; "+
			"insert generated and explicit keys in separate batches", zeroPK, len(maps), pkColumn)
	default:
		return maps, pkColumn, nil
	}
}

// UpdateStruct builds an UPDATE that sets every column of data except the
// primary key. Add Where.
func (qb *QueryBuilder) UpdateStruct(table string, data any) *UpdateQuery {
	uq := qb.Update(table)
	values, err := qb.db.structValuesExceptPK(data)
	if err != nil {
		uq.buildErr = err
		return uq
	}
	return uq.Set(values)
}

func (qb *QueryBuilder) errQuery(err error) *Query {
	return &Query{prepErr: err, db: qb.db, tx: qb.tx, ctx: qb.ctx}
}
