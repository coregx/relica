package core

// testdb_test.go provides a minimal in-memory SQL driver for unit tests
// that need real *sql.Rows and *sql.Tx objects without an external database.
//
// The driver registers under the name "memdb" and supports:
//   - CREATE TABLE (parsed to extract column names)
//   - INSERT INTO ... VALUES
//   - SELECT ... FROM ... WHERE
//   - UPDATE ... SET ... WHERE
//   - DELETE FROM ... WHERE
//   - COUNT(*) aggregation
//
// Limitations: minimal SQL subset, no joins, no subqueries, no expressions in WHERE
// beyond "col = val" and "col = ?" placeholders. Sufficient for internal/core tests.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ─── Driver registration ──────────────────────────────────────────────────────

func init() {
	sql.Register("memdb", &memDriver{})
}

// unique counter so each Open() call gets an isolated in-memory database.
var memdbCounter atomic.Int64

// ─── Driver ──────────────────────────────────────────────────────────────────

type memDriver struct{}

func (d *memDriver) Open(name string) (driver.Conn, error) {
	id := memdbCounter.Add(1)
	key := fmt.Sprintf("%s-%d", name, id)
	return &memConn{db: getOrCreateDB(key)}, nil
}

// ─── Per-connection in-memory state ──────────────────────────────────────────

var (
	globalDBsMu sync.Mutex
	globalDBs   = map[string]*memDatabase{}
)

func getOrCreateDB(key string) *memDatabase {
	globalDBsMu.Lock()
	defer globalDBsMu.Unlock()
	if db, ok := globalDBs[key]; ok {
		return db
	}
	db := &memDatabase{tables: map[string]*memTable{}}
	globalDBs[key] = db
	return db
}

// memDatabase holds tables for a single connection.
type memDatabase struct {
	mu     sync.Mutex
	tables map[string]*memTable
}

// memTable holds rows for a single table.
type memTable struct {
	columns []string
	rows    []memRow
}

type memRow []driver.Value

// ─── Connection ──────────────────────────────────────────────────────────────

type memConn struct {
	db *memDatabase
	tx *memTx
}

func (c *memConn) Prepare(query string) (driver.Stmt, error) {
	return &memStmt{conn: c, query: strings.TrimSpace(query)}, nil
}

func (c *memConn) Close() error { return nil }

func (c *memConn) Begin() (driver.Tx, error) {
	return c.beginTx()
}

// BeginTx implements driver.ConnBeginTx so that sql.DB can pass isolation levels
// without returning "driver does not support non-default isolation level".
// Our in-memory driver accepts any isolation level (it ignores them).
func (c *memConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	return c.beginTx()
}

func (c *memConn) beginTx() (driver.Tx, error) {
	snap, work := c.db.snapshotAndWork()
	t := &memTx{conn: c, snapshot: snap, work: work}
	c.tx = t
	return t, nil
}

// execDB runs a statement against the appropriate target (tx work copy or live db).
func (c *memConn) execDB() *memDatabase {
	if c.tx != nil {
		return c.tx.work
	}
	return c.db
}

// ─── Transaction ─────────────────────────────────────────────────────────────

type memTx struct {
	conn     *memConn
	work     *memDatabase // mutable working copy; committed to conn.db on Commit
	snapshot *memDatabase // immutable snapshot of state before tx; restored on Rollback
}

func (t *memTx) Commit() error {
	// Replace live database tables with working copy.
	t.conn.db.mu.Lock()
	defer t.conn.db.mu.Unlock()
	t.work.mu.Lock()
	defer t.work.mu.Unlock()

	t.conn.db.tables = cloneTables(t.work.tables)
	t.conn.tx = nil
	return nil
}

func (t *memTx) Rollback() error {
	// Restore live database to pre-transaction snapshot.
	t.conn.db.mu.Lock()
	defer t.conn.db.mu.Unlock()
	t.snapshot.mu.Lock()
	defer t.snapshot.mu.Unlock()

	t.conn.db.tables = cloneTables(t.snapshot.tables)
	t.conn.tx = nil
	return nil
}

// cloneTables deep-copies a table map.
func cloneTables(src map[string]*memTable) map[string]*memTable {
	dst := make(map[string]*memTable, len(src))
	for name, tbl := range src {
		dst[name] = cloneTable(tbl)
	}
	return dst
}

// cloneTable deep-copies a single memTable.
func cloneTable(tbl *memTable) *memTable {
	cp := &memTable{
		columns: append([]string{}, tbl.columns...),
		rows:    make([]memRow, len(tbl.rows)),
	}
	for i, r := range tbl.rows {
		cp.rows[i] = append(memRow{}, r...)
	}
	return cp
}

// ─── Snapshot ────────────────────────────────────────────────────────────────

// snapshotAndWork returns two independent deep copies of db:
//   - snap: immutable snapshot used for Rollback
//   - work: mutable working copy that accumulates tx changes
func (db *memDatabase) snapshotAndWork() (snap, work *memDatabase) {
	db.mu.Lock()
	defer db.mu.Unlock()

	snap = &memDatabase{tables: cloneTables(db.tables)}
	work = &memDatabase{tables: cloneTables(db.tables)}
	return snap, work
}

// ─── Statement ───────────────────────────────────────────────────────────────

type memStmt struct {
	conn  *memConn
	query string
}

func (s *memStmt) Close() error { return nil }

func (s *memStmt) NumInput() int { return -1 } // variadic

func (s *memStmt) Exec(args []driver.Value) (driver.Result, error) {
	upper := strings.ToUpper(strings.TrimSpace(s.query))
	db := s.conn.execDB()

	switch {
	case strings.HasPrefix(upper, "CREATE TABLE"):
		return s.execCreate(db)
	case strings.HasPrefix(upper, "INSERT INTO"):
		return s.execInsert(db, args)
	case strings.HasPrefix(upper, "UPDATE"):
		return s.execUpdate(db, args)
	case strings.HasPrefix(upper, "DELETE FROM"):
		return s.execDelete(db, args)
	default:
		return driver.ResultNoRows, nil
	}
}

func (s *memStmt) Query(args []driver.Value) (driver.Rows, error) {
	upper := strings.ToUpper(strings.TrimSpace(s.query))
	db := s.conn.execDB()

	switch {
	case strings.HasPrefix(upper, "SELECT"):
		return s.execSelect(db, args)
	default:
		return &memRows{}, nil
	}
}

// ─── CREATE TABLE ─────────────────────────────────────────────────────────────

func (s *memStmt) execCreate(db *memDatabase) (driver.Result, error) {
	// CREATE TABLE [IF NOT EXISTS] name (col1 TYPE, col2 TYPE, ...)
	q := s.query

	// Strip "IF NOT EXISTS"
	ifNotExists := false
	upper := strings.ToUpper(q)
	if idx := strings.Index(upper, "IF NOT EXISTS"); idx >= 0 {
		ifNotExists = true
		q = q[:idx] + q[idx+len("IF NOT EXISTS"):]
	}

	// Remove "CREATE TABLE " prefix
	idx := strings.Index(strings.ToUpper(q), "TABLE")
	if idx < 0 {
		return nil, errors.New("memdb: invalid CREATE TABLE")
	}
	q = strings.TrimSpace(q[idx+5:])

	// Extract table name
	parenIdx := strings.Index(q, "(")
	if parenIdx < 0 {
		return nil, errors.New("memdb: missing ( in CREATE TABLE")
	}
	tableName := strings.ToLower(unquoteIdent(q[:parenIdx]))

	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.tables[tableName]; exists {
		if ifNotExists {
			return driver.RowsAffected(0), nil
		}
		return nil, fmt.Errorf("memdb: table %q already exists", tableName)
	}

	// Parse column definitions
	rest := q[parenIdx+1:]
	closeIdx := strings.LastIndex(rest, ")")
	if closeIdx < 0 {
		return nil, errors.New("memdb: missing ) in CREATE TABLE")
	}
	colDefs := rest[:closeIdx]

	columns := parseColumnDefs(colDefs)
	db.tables[tableName] = &memTable{columns: columns}
	return driver.RowsAffected(0), nil
}

// parseColumnDefs splits column definitions, extracting only column names.
func parseColumnDefs(defs string) []string {
	var cols []string
	for _, part := range splitTopLevel(defs, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		upper := strings.ToUpper(part)
		// Skip table constraints: PRIMARY KEY (...), UNIQUE (...), FOREIGN KEY (...)
		if strings.HasPrefix(upper, "PRIMARY KEY") ||
			strings.HasPrefix(upper, "UNIQUE") ||
			strings.HasPrefix(upper, "FOREIGN KEY") ||
			strings.HasPrefix(upper, "CHECK") {
			continue
		}
		// First token is column name (strip quoting)
		fields := strings.Fields(part)
		if len(fields) > 0 {
			cols = append(cols, strings.ToLower(unquoteIdent(fields[0])))
		}
	}
	return cols
}

// splitTopLevel splits s by sep, ignoring sep inside parentheses.
func splitTopLevel(s string, sep rune) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// ─── INSERT ───────────────────────────────────────────────────────────────────

func (s *memStmt) execInsert(db *memDatabase, args []driver.Value) (driver.Result, error) {
	// INSERT INTO table (col1, col2) VALUES (?, ?)
	q := s.query
	upper := strings.ToUpper(q)

	idx := strings.Index(upper, "INTO")
	if idx < 0 {
		return nil, errors.New("memdb: invalid INSERT")
	}
	q = strings.TrimSpace(q[idx+4:])

	// Table name (strip SQL identifier quoting)
	var tableName string
	if i := strings.IndexAny(q, " \t("); i >= 0 {
		tableName = strings.ToLower(unquoteIdent(q[:i]))
		q = strings.TrimSpace(q[i:])
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tbl, ok := db.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("memdb: table %q not found", tableName)
	}

	// Parse column list and values
	var insertCols []string
	var vals []driver.Value

	if strings.HasPrefix(q, "(") {
		// Has explicit column list
		closeIdx := strings.Index(q, ")")
		if closeIdx < 0 {
			return nil, errors.New("memdb: missing ) in INSERT column list")
		}
		colList := q[1:closeIdx]
		for _, c := range strings.Split(colList, ",") {
			insertCols = append(insertCols, strings.ToLower(unquoteIdent(c)))
		}
		q = strings.TrimSpace(q[closeIdx+1:])
	}

	upper = strings.ToUpper(q)
	valIdx := strings.Index(upper, "VALUES")
	if valIdx < 0 {
		return nil, errors.New("memdb: missing VALUES in INSERT")
	}
	q = strings.TrimSpace(q[valIdx+6:])

	// Parse VALUES (...)
	if strings.HasPrefix(q, "(") {
		closeIdx := strings.LastIndex(q, ")")
		q = q[1:closeIdx]
	}

	valParts := strings.Split(q, ",")
	argIdx := 0
	for _, vp := range valParts {
		vp = strings.TrimSpace(vp)
		if vp == "?" {
			if argIdx >= len(args) {
				return nil, errors.New("memdb: not enough args for INSERT")
			}
			vals = append(vals, args[argIdx])
			argIdx++
		} else {
			vals = append(vals, parseValue(vp))
		}
	}

	// Build a row matching table column order
	row := make(memRow, len(tbl.columns))
	if len(insertCols) > 0 {
		// Map provided columns to table column positions
		colIndex := map[string]int{}
		for i, c := range tbl.columns {
			colIndex[c] = i
		}
		for i, col := range insertCols {
			if i < len(vals) {
				if pos, ok := colIndex[col]; ok {
					row[pos] = vals[i]
				}
			}
		}
		// Auto-assign INTEGER PRIMARY KEY (AUTOINCREMENT simulation)
		// If "id" column is nil and is an integer-like field, assign next id.
		if pos, ok := colIndex["id"]; ok && row[pos] == nil {
			row[pos] = driver.Value(nextID(tbl))
		}
	} else {
		for i := range row {
			if i < len(vals) {
				row[i] = vals[i]
			}
		}
	}

	tbl.rows = append(tbl.rows, row)
	return driver.RowsAffected(1), nil
}

// nextID returns a simple auto-increment id for AUTOINCREMENT columns.
func nextID(tbl *memTable) int64 {
	var maxID int64
	colIndex := colIndexMap(tbl)
	if pos, ok := colIndex["id"]; ok {
		for _, r := range tbl.rows {
			if pos < len(r) {
				switch v := r[pos].(type) {
				case int64:
					if v > maxID {
						maxID = v
					}
				}
			}
		}
	}
	return maxID + 1
}

func colIndexMap(tbl *memTable) map[string]int {
	m := map[string]int{}
	for i, c := range tbl.columns {
		m[c] = i
	}
	return m
}

// ─── UPDATE ───────────────────────────────────────────────────────────────────

func (s *memStmt) execUpdate(db *memDatabase, args []driver.Value) (driver.Result, error) {
	// UPDATE table SET col=?, col=? WHERE col=?
	q := s.query
	upper := strings.ToUpper(q)

	// Table name: after UPDATE
	rest := strings.TrimSpace(q[len("UPDATE"):])
	setIdx := strings.Index(strings.ToUpper(rest), " SET ")
	if setIdx < 0 {
		return nil, errors.New("memdb: missing SET in UPDATE")
	}
	tableName := strings.ToLower(unquoteIdent(rest[:setIdx]))
	rest = rest[setIdx+5:]

	db.mu.Lock()
	defer db.mu.Unlock()

	tbl, ok := db.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("memdb: table %q not found", tableName)
	}
	colIdx := colIndexMap(tbl)

	// Split SET from WHERE
	whereIdx := strings.Index(strings.ToUpper(rest), " WHERE ")
	var setClause, whereClause string
	if whereIdx >= 0 {
		setClause = rest[:whereIdx]
		whereClause = rest[whereIdx+7:]
	} else {
		setClause = rest
	}

	argIdx := 0
	// Parse SET assignments
	type assignment struct {
		col string
		val driver.Value
	}
	var assignments []assignment
	_ = upper
	for _, part := range strings.Split(setClause, ",") {
		part = strings.TrimSpace(part)
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			continue
		}
		col := strings.ToLower(unquoteIdent(part[:eqIdx]))
		valStr := strings.TrimSpace(part[eqIdx+1:])
		var val driver.Value
		if valStr == "?" {
			if argIdx >= len(args) {
				return nil, errors.New("memdb: not enough args for UPDATE SET")
			}
			val = args[argIdx]
			argIdx++
		} else {
			val = parseValue(valStr)
		}
		assignments = append(assignments, assignment{col, val})
	}

	// Parse WHERE condition (argIdx tracks position after SET args)
	var whereConds []whereCond
	if whereClause != "" {
		var err error
		whereConds, err = parseWhere(whereClause, args, &argIdx)
		if err != nil {
			return nil, err
		}
	}

	var affected int64
	for i, row := range tbl.rows {
		if matchesAll(row, colIdx, whereConds) {
			for _, a := range assignments {
				if pos, ok := colIdx[a.col]; ok {
					tbl.rows[i][pos] = a.val
				}
			}
			affected++
		}
	}

	return driver.RowsAffected(affected), nil
}

// ─── DELETE ───────────────────────────────────────────────────────────────────

func (s *memStmt) execDelete(db *memDatabase, args []driver.Value) (driver.Result, error) {
	// DELETE FROM table WHERE col=?
	q := s.query
	upper := strings.ToUpper(q)

	fromIdx := strings.Index(upper, "FROM")
	if fromIdx < 0 {
		return nil, errors.New("memdb: invalid DELETE")
	}
	rest := strings.TrimSpace(q[fromIdx+4:])

	whereIdx := strings.Index(strings.ToUpper(rest), " WHERE ")
	var tableName, whereClause string
	if whereIdx >= 0 {
		tableName = strings.ToLower(unquoteIdent(rest[:whereIdx]))
		whereClause = rest[whereIdx+7:]
	} else {
		tableName = strings.ToLower(unquoteIdent(rest))
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tbl, ok := db.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("memdb: table %q not found", tableName)
	}
	colIdx := colIndexMap(tbl)

	var whereConds []whereCond
	if whereClause != "" {
		var err error
		whereArgIdx := 0
		whereConds, err = parseWhere(whereClause, args, &whereArgIdx)
		if err != nil {
			return nil, err
		}
	}

	var remaining []memRow
	var affected int64
	for _, row := range tbl.rows {
		if matchesAll(row, colIdx, whereConds) {
			affected++
		} else {
			remaining = append(remaining, row)
		}
	}
	tbl.rows = remaining
	return driver.RowsAffected(affected), nil
}

// ─── SELECT ───────────────────────────────────────────────────────────────────

func (s *memStmt) execSelect(db *memDatabase, args []driver.Value) (driver.Rows, error) {
	q := s.query
	upper := strings.ToUpper(q)

	// Find FROM
	fromIdx := strings.Index(upper, " FROM ")
	if fromIdx < 0 {
		return &memRows{}, nil
	}
	selectPart := strings.TrimSpace(q[7:fromIdx]) // after SELECT
	rest := strings.TrimSpace(q[fromIdx+6:])

	// Find WHERE
	whereIdx := strings.Index(strings.ToUpper(rest), " WHERE ")
	var tableName, whereClause string
	if whereIdx >= 0 {
		tableName = strings.ToLower(unquoteIdent(rest[:whereIdx]))
		whereClause = rest[whereIdx+7:]
	} else {
		tableName = strings.ToLower(unquoteIdent(rest))
	}
	// Strip ORDER BY if present
	if i := strings.Index(strings.ToUpper(tableName), " ORDER BY"); i >= 0 {
		tableName = unquoteIdent(tableName[:i])
	}
	if i := strings.Index(strings.ToUpper(whereClause), " ORDER BY"); i >= 0 {
		whereClause = whereClause[:i]
	}
	tableName = strings.ToLower(strings.TrimSpace(tableName))

	db.mu.Lock()
	defer db.mu.Unlock()

	// COUNT(*) special case
	upperSelect := strings.ToUpper(strings.TrimSpace(selectPart))
	if strings.Contains(upperSelect, "COUNT(") {
		tbl, ok := db.tables[tableName]
		var count int64
		if ok {
			tbl2 := tbl
			colIdx := colIndexMap(tbl2)
			var whereConds []whereCond
			if whereClause != "" {
				var err error
				whereArgIdx := 0
				whereConds, err = parseWhere(whereClause, args, &whereArgIdx)
				if err != nil {
					return nil, err
				}
			}
			for _, row := range tbl2.rows {
				if matchesAll(row, colIdx, whereConds) {
					count++
				}
			}
		}
		return &memRows{
			columns: []string{"count"},
			rows:    []memRow{{count}},
		}, nil
	}

	tbl, ok := db.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("memdb: table %q not found", tableName)
	}
	colIdx := colIndexMap(tbl)

	var whereConds []whereCond
	if whereClause != "" {
		var err error
		whereArgIdx := 0
		whereConds, err = parseWhere(whereClause, args, &whereArgIdx)
		if err != nil {
			return nil, err
		}
	}

	// Determine output columns
	var outCols []string
	var outPositions []int
	if strings.TrimSpace(selectPart) == "*" || selectPart == "" {
		outCols = tbl.columns
		for i := range tbl.columns {
			outPositions = append(outPositions, i)
		}
	} else {
		for _, c := range strings.Split(selectPart, ",") {
			c = strings.ToLower(unquoteIdent(c))
			outCols = append(outCols, c)
			if pos, ok := colIdx[c]; ok {
				outPositions = append(outPositions, pos)
			} else {
				outPositions = append(outPositions, -1)
			}
		}
	}

	var resultRows []memRow
	for _, row := range tbl.rows {
		if matchesAll(row, colIdx, whereConds) {
			out := make(memRow, len(outPositions))
			for i, pos := range outPositions {
				if pos >= 0 && pos < len(row) {
					out[i] = row[pos]
				}
			}
			resultRows = append(resultRows, out)
		}
	}

	return &memRows{columns: outCols, rows: resultRows}, nil
}

// ─── WHERE parsing ────────────────────────────────────────────────────────────

type whereCond struct {
	col string
	val driver.Value
}

// parseWhere parses a WHERE clause into conditions. argIdx is advanced for each ? placeholder.
func parseWhere(where string, args []driver.Value, argIdx *int) ([]whereCond, error) {
	var conds []whereCond

	// Split by AND (simple implementation, no OR support)
	parts := splitAnd(where)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			continue
		}
		col := strings.ToLower(unquoteIdent(part[:eqIdx]))
		valStr := strings.TrimSpace(part[eqIdx+1:])
		var val driver.Value
		if valStr == "?" {
			if *argIdx >= len(args) {
				return nil, errors.New("memdb: not enough args for WHERE")
			}
			val = args[*argIdx]
			*argIdx++
		} else {
			val = parseValue(valStr)
		}
		conds = append(conds, whereCond{col, val})
	}

	return conds, nil
}

// splitAnd splits a WHERE clause by AND, ignoring case.
func splitAnd(s string) []string {
	var parts []string
	upper := strings.ToUpper(s)
	start := 0
	for {
		idx := strings.Index(upper[start:], " AND ")
		if idx < 0 {
			parts = append(parts, s[start:])
			break
		}
		parts = append(parts, s[start:start+idx])
		start += idx + 5
	}
	return parts
}

func matchesAll(row memRow, colIdx map[string]int, conds []whereCond) bool {
	for _, c := range conds {
		pos, ok := colIdx[c.col]
		if !ok {
			return false
		}
		if !valuesEqual(row[pos], c.val) {
			return false
		}
	}
	return true
}

func valuesEqual(a, b driver.Value) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Normalize numeric types for comparison
	ai := toInt64(a)
	bi := toInt64(b)
	if ai != nil && bi != nil {
		return *ai == *bi
	}
	as := toString(a)
	bs := toString(b)
	if as != nil && bs != nil {
		return *as == *bs
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toInt64(v driver.Value) *int64 {
	switch x := v.(type) {
	case int64:
		return &x
	case int:
		i := int64(x)
		return &i
	}
	return nil
}

func toString(v driver.Value) *string {
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

// ─── Identifier / value helpers ──────────────────────────────────────────────

// unquoteIdent strips SQL identifier quoting (", `, []) from a name.
func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		switch {
		case s[0] == '"' && s[len(s)-1] == '"':
			return s[1 : len(s)-1]
		case s[0] == '`' && s[len(s)-1] == '`':
			return s[1 : len(s)-1]
		case s[0] == '[' && s[len(s)-1] == ']':
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseValue parses a SQL literal into a driver.Value.
func parseValue(s string) driver.Value {
	s = strings.TrimSpace(s)
	// Single-quoted string
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	// Double-quoted string (non-identifier context, treat as string value)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	// NULL
	if strings.ToUpper(s) == "NULL" {
		return nil
	}
	// Integer
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	// Float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// ─── Rows ────────────────────────────────────────────────────────────────────

type memRows struct {
	columns []string
	rows    []memRow
	pos     int
}

func (r *memRows) Columns() []string { return r.columns }

func (r *memRows) Close() error { return nil }

func (r *memRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.pos]
	r.pos++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		} else {
			dest[i] = nil
		}
	}
	return nil
}
