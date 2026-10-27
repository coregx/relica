package core

import (
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
	return qb.BatchInsert(table, nil).valuesStructs(data, "BatchInsertStruct").Build()
}

// ValuesStructs adds one row per element of rows, a slice of structs or struct
// pointers of one type, read under the DB's Naming like BatchInsertStruct.
//
// Without columns in BatchInsert, the columns come from the struct (a single
// primary key that is zero in every row is left out, as in BatchInsertStruct)
// and ValuesStructs must be the only source of rows. With columns, exactly
// those are read from each struct; a listed primary key must not be zero.
// An empty autoid field is an error: a batch does not generate IDs.
func (biq *BatchInsertQuery) ValuesStructs(rows any) *BatchInsertQuery {
	return biq.valuesStructs(rows, "BatchInsert.ValuesStructs")
}

func (biq *BatchInsertQuery) valuesStructs(data any, op string) *BatchInsertQuery {
	if biq.buildErr != nil {
		return biq
	}
	rows := reflect.ValueOf(data)
	if rows.Kind() != reflect.Slice {
		biq.buildErr = fmt.Errorf("relica: %s expects a slice, got %T", op, data)
		return biq
	}
	if rows.Len() == 0 {
		biq.buildErr = fmt.Errorf("relica: %s: empty slice", op)
		return biq
	}
	if biq.derived || (len(biq.columns) == 0 && len(biq.rows) > 0) {
		biq.buildErr = fmt.Errorf("relica: %s without columns must be the only source of rows: "+
			"pass every row in one call, or name the columns in BatchInsert", op)
		return biq
	}
	sr, err := biq.builder.structRows(rows, op)
	if err != nil {
		biq.buildErr = err
		return biq
	}
	if len(biq.columns) == 0 {
		err = biq.addDerivedRows(sr, op)
	} else {
		err = biq.addListedRows(sr, op)
	}
	if err != nil {
		biq.buildErr = err
	}
	return biq
}

// structRowSet is a slice of structs read into column maps.
type structRowSet struct {
	maps     []map[string]any
	pkColumn string // single-column primary key, "" if none
	zeroPK   []bool // per row: the single-column primary key is zero
	autoID   []string
}

// structRows reads every row's columns. Rows must share one struct type.
func (qb *QueryBuilder) structRows(rows reflect.Value, op string) (structRowSet, error) {
	sr := structRowSet{maps: make([]map[string]any, rows.Len()), zeroPK: make([]bool, rows.Len())}
	var first reflect.Type
	for i := range sr.maps {
		elem := rows.Index(i).Interface()
		m, err := qb.db.structValues(elem)
		if err != nil {
			return sr, fmt.Errorf("relica: %s row %d: %w", op, i, err)
		}
		v, _ := structValue(elem)
		if i == 0 {
			first = v.Type()
			fields, err := qb.db.autoIDFields(first)
			if err != nil {
				return sr, fmt.Errorf("relica: %s row 0: %w", op, err)
			}
			for _, f := range fields {
				sr.autoID = append(sr.autoID, f.column)
			}
		} else if v.Type() != first {
			return sr, fmt.Errorf("relica: %s row %d is %s, row 0 is %s: rows must have one type", op, i, v.Type(), first)
		}
		sr.maps[i] = m
		if pk, err := qb.db.primaryKey(v); err == nil && pk.IsSingle() {
			sr.pkColumn = pk.Columns[0]
			sr.zeroPK[i] = util.IsPrimaryKeyZero(pk.Values[0])
		}
	}
	return sr, nil
}

// checkAutoID reports an autoid column that is written empty: Model().Insert()
// generates the ID, a batch would insert "".
func (sr structRowSet) checkAutoID(written func(string) bool, op string) error {
	for _, col := range sr.autoID {
		if !written(col) {
			continue
		}
		for i, m := range sr.maps {
			if s, ok := m[col].(string); ok && s == "" {
				return fmt.Errorf("relica: %s row %d: autoid field %q is empty; a batch does not generate IDs: "+
					"set it, or insert the row with Model().Insert()", op, i, col)
			}
		}
	}
	return nil
}

// addDerivedRows takes the columns from the struct: every column, without a
// single primary key that is zero in every row.
func (biq *BatchInsertQuery) addDerivedRows(sr structRowSet, op string) error {
	zero := 0
	for _, z := range sr.zeroPK {
		if z {
			zero++
		}
	}
	skipPK := ""
	switch {
	case zero == len(sr.maps):
		skipPK = sr.pkColumn
	case zero > 0:
		return fmt.Errorf("relica: %s: %d of %d rows have a zero primary key %q; "+
			"insert generated and explicit keys in separate batches", op, zero, len(sr.maps), sr.pkColumn)
	}
	columns := make([]string, 0, len(sr.maps[0]))
	for col := range sr.maps[0] {
		if col != skipPK {
			columns = append(columns, col)
		}
	}
	sort.Strings(columns)
	if err := sr.checkAutoID(func(col string) bool { return col != skipPK }, op); err != nil {
		return err
	}
	biq.columns = columns
	biq.derived = true
	return biq.appendMaps(sr.maps)
}

// addListedRows reads exactly the BatchInsert columns from each struct.
func (biq *BatchInsertQuery) addListedRows(sr structRowSet, op string) error {
	listed := make(map[string]bool, len(biq.columns))
	for _, col := range biq.columns {
		if _, ok := sr.maps[0][col]; !ok {
			return fmt.Errorf("relica: %s: column %q is not a field of the struct", op, col)
		}
		listed[col] = true
	}
	if listed[sr.pkColumn] {
		for i, z := range sr.zeroPK {
			if z {
				return fmt.Errorf("relica: %s row %d: zero primary key %q in the column list; "+
					"leave the column out so the database generates it", op, i, sr.pkColumn)
			}
		}
	}
	if err := sr.checkAutoID(func(col string) bool { return listed[col] }, op); err != nil {
		return err
	}
	return biq.appendMaps(sr.maps)
}

func (biq *BatchInsertQuery) appendMaps(maps []map[string]any) error {
	for _, m := range maps {
		row := make([]any, len(biq.columns))
		for j, col := range biq.columns {
			row[j] = m[col]
		}
		biq.rows = append(biq.rows, row)
	}
	return nil
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
