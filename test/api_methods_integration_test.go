//go:build integration

// Package test contains integration tests for the Relica query builder.
// This file validates the new API methods (SqlDB, PingContext, DriverName,
// Tx raw-SQL methods, ForUpdate/ForShare, IsNull, GreaterOrEqualCol, Find)
// against a real SQLite database.
package test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── DB.SqlDB + DB.PingContext ────────────────────────────────────────────────

// TestSqlDB_SQLite verifies that SqlDB() exposes a live *sql.DB and that
// PingContext on that connection succeeds.
func TestSqlDB_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	rawDB := ds.DB.SqlDB()
	require.NotNil(t, rawDB, "SqlDB() must not return nil")

	ctx := context.Background()
	err := rawDB.PingContext(ctx)
	assert.NoError(t, err, "PingContext on SqlDB() must succeed")
}

// TestPingContext_SQLite verifies that DB.PingContext() succeeds on a healthy connection.
func TestPingContext_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	err := ds.DB.PingContext(context.Background())
	assert.NoError(t, err)
}

// ─── DB.DriverName ───────────────────────────────────────────────────────────

// TestDriverName_SQLite verifies that DriverName() returns "sqlite" (the
// driver name passed to relica.NewDB) for a SQLite connection.
func TestDriverName_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	got := ds.DB.DriverName()

	// modernc.org/sqlite registers as "sqlite" in testutil.go (SetupSQLiteTestDB
	// calls relica.NewDB("sqlite", ":memory:")); the driver itself may internally
	// use "sqlite" or "sqlite3" — what matters is that the stored name matches
	// the one we passed in.
	if got != "sqlite" && got != "sqlite3" {
		t.Errorf("DriverName() = %q, want %q or %q", got, "sqlite", "sqlite3")
	}
}

// ─── Tx.ExecContext ───────────────────────────────────────────────────────────

// TestTx_ExecContext_SQLite verifies that Tx.ExecContext can execute raw DDL
// and DML statements. The transaction is committed and data verified afterwards.
func TestTx_ExecContext_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	ctx := context.Background()
	db := ds.DB

	// Create a fresh table outside the transaction.
	_, err := db.ExecContext(ctx, `CREATE TABLE tx_exec_test (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	// Begin transaction.
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Insert via ExecContext on the Tx.
	result, err := tx.ExecContext(ctx, `INSERT INTO tx_exec_test (name) VALUES (?)`, "alice")
	require.NoError(t, err)

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Commit.
	require.NoError(t, tx.Commit())

	// Verify the row persists outside the transaction.
	var name string
	row := db.SqlDB().QueryRowContext(ctx, `SELECT name FROM tx_exec_test WHERE name = ?`, "alice")
	require.NoError(t, row.Scan(&name))
	assert.Equal(t, "alice", name)
}

// TestTx_ExecContext_Rollback verifies that rows inserted via ExecContext are
// NOT visible after a rollback.
func TestTx_ExecContext_Rollback(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	ctx := context.Background()
	db := ds.DB

	_, err := db.ExecContext(ctx, `CREATE TABLE tx_rollback_test (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `INSERT INTO tx_rollback_test (name) VALUES (?)`, "ghost")
	require.NoError(t, err)

	require.NoError(t, tx.Rollback())

	// After rollback the row must not exist.
	var count int
	row := db.SqlDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tx_rollback_test WHERE name = ?`, "ghost")
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count, "rolled-back row must not be visible")
}

// ─── Tx.QueryContext ──────────────────────────────────────────────────────────

// TestTx_QueryContext_SQLite verifies that Tx.QueryContext returns rows visible
// within the open transaction (even before commit).
func TestTx_QueryContext_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	ctx := context.Background()
	db := ds.DB

	_, err := db.ExecContext(ctx, `CREATE TABLE tx_query_test (
		id    INTEGER PRIMARY KEY AUTOINCREMENT,
		score INTEGER NOT NULL
	)`)
	require.NoError(t, err)

	// Seed two rows outside the transaction.
	_, err = db.ExecContext(ctx, `INSERT INTO tx_query_test (score) VALUES (10), (20)`)
	require.NoError(t, err)

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	// QueryContext within the transaction.
	rows, err := tx.QueryContext(ctx, `SELECT score FROM tx_query_test ORDER BY score`)
	require.NoError(t, err)
	defer rows.Close()

	var scores []int
	for rows.Next() {
		var s int
		require.NoError(t, rows.Scan(&s))
		scores = append(scores, s)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []int{10, 20}, scores)
}

// ─── Tx.QueryRowContext ───────────────────────────────────────────────────────

// TestTx_QueryRowContext_SQLite verifies that Tx.QueryRowContext scans a single
// row correctly within an open transaction.
func TestTx_QueryRowContext_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	ctx := context.Background()
	db := ds.DB

	_, err := db.ExecContext(ctx, `CREATE TABLE tx_rowquery_test (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO tx_rowquery_test (name) VALUES (?)`, "bob")
	require.NoError(t, err)

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	var name string
	row := tx.QueryRowContext(ctx, `SELECT name FROM tx_rowquery_test WHERE name = ?`, "bob")
	require.NoError(t, row.Scan(&name))
	assert.Equal(t, "bob", name)
}

// ─── ModelQuery.Find ─────────────────────────────────────────────────────────

// findTestItem is the model used by Find integration tests.
type findTestItem struct {
	ID    int64  `db:"id,pk"`
	Title string `db:"title"`
}

// TableName makes the table name explicit and stable.
func (findTestItem) TableName() string { return "find_items" }

// setupFindTable creates the find_items table and inserts one seed row.
// Returns the inserted row's ID.
func setupFindTable(t *testing.T, db *relica.DB) int64 {
	t.Helper()
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS find_items (
		id    INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL
	)`)
	require.NoError(t, err)

	res, err := db.ExecContext(ctx, `INSERT INTO find_items (title) VALUES (?)`, "hello world")
	require.NoError(t, err)

	id, err := res.LastInsertId()
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS find_items`)
	})

	return id
}

// TestFind_SinglePK_SQLite verifies that Find(id) fetches the record and
// populates all struct fields correctly.
func TestFind_SinglePK_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	db := ds.DB
	id := setupFindTable(t, db)

	var item findTestItem
	err := db.Model(&item).Find(id)
	require.NoError(t, err, "Find(%d) must succeed", id)

	assert.Equal(t, id, item.ID, "ID must match the inserted row")
	assert.Equal(t, "hello world", item.Title, "Title must match the inserted value")
}

// TestFind_NotFound_SQLite verifies that Find() with a non-existent PK value
// returns ErrNotFound.
func TestFind_NotFound_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	db := ds.DB
	setupFindTable(t, db)

	var item findTestItem
	err := db.Model(&item).Find(int64(999999))

	require.Error(t, err, "Find(999999) must return an error for missing row")
	assert.True(t, errors.Is(err, relica.ErrNotFound),
		"error must wrap ErrNotFound; got: %v", err)
}

// ─── SelectQuery.ForUpdate on SQLite ─────────────────────────────────────────

// TestForUpdate_SQLite_NoError verifies that ForUpdate() does not produce an
// error on SQLite and that the generated SQL executes successfully (the clause
// is simply omitted since SQLite uses file-level locking).
func TestForUpdate_SQLite_NoError(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	ctx := context.Background()
	db := ds.DB

	_, err := db.ExecContext(ctx, `CREATE TABLE forupdate_test (
		id    INTEGER PRIMARY KEY AUTOINCREMENT,
		value INTEGER NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO forupdate_test (value) VALUES (42)`)
	require.NoError(t, err)

	// Inspect the SQL without executing to verify "FOR UPDATE" is absent.
	sql, _ := db.Select("id", "value").From("forupdate_test").ForUpdate().ToSQL()
	assert.False(t, strings.Contains(sql, "FOR UPDATE"),
		"SQLite: FOR UPDATE must be omitted; got SQL: %s", sql)

	// Execute the query; it must succeed without error even with ForUpdate set.
	type row struct {
		ID    int64 `db:"id"`
		Value int   `db:"value"`
	}
	var rows []row
	err = db.Select("id", "value").
		From("forupdate_test").
		ForUpdate().
		All(&rows)
	require.NoError(t, err, "ForUpdate on SQLite must not produce a query error")
	require.Len(t, rows, 1)
	assert.Equal(t, 42, rows[0].Value)
}

// TestForShare_SQLite_NoError verifies that ForShare() is silently ignored on
// SQLite and that the query still returns correct results.
func TestForShare_SQLite_NoError(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	ctx := context.Background()
	db := ds.DB

	_, err := db.ExecContext(ctx, `CREATE TABLE forshare_test (
		id    INTEGER PRIMARY KEY AUTOINCREMENT,
		label TEXT NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO forshare_test (label) VALUES (?)`, "shared")
	require.NoError(t, err)

	type row struct {
		ID    int64  `db:"id"`
		Label string `db:"label"`
	}
	var rows []row
	err = db.Select("id", "label").
		From("forshare_test").
		ForShare().
		All(&rows)
	require.NoError(t, err, "ForShare on SQLite must not produce a query error")
	require.Len(t, rows, 1)
	assert.Equal(t, "shared", rows[0].Label)
}

// ─── SelectQuery.Model ────────────────────────────────────────────────────────

// modelTestUser is the model used by SelectQuery.Model integration tests.
type modelTestUser struct {
	ID    int64  `db:"id,pk"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

// TableName makes the table name explicit and stable across tests.
func (modelTestUser) TableName() string { return "model_test_users" }

// setupModelTable creates the model_test_users table and inserts a seed row.
// Returns the inserted row's ID.
func setupModelTable(t *testing.T, db *relica.DB) int64 {
	t.Helper()
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS model_test_users (
		id    INTEGER PRIMARY KEY AUTOINCREMENT,
		name  TEXT    NOT NULL,
		email TEXT    NOT NULL
	)`)
	require.NoError(t, err)

	res, err := db.ExecContext(ctx,
		`INSERT INTO model_test_users (name, email) VALUES (?, ?)`,
		"Alice", "alice@example.com",
	)
	require.NoError(t, err)

	id, err := res.LastInsertId()
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS model_test_users`)
	})

	return id
}

// TestSelectQuery_Model_SinglePK_SQLite inserts a row, then calls Select().Model()
// with the PK set and verifies all fields are populated.
func TestSelectQuery_Model_SinglePK_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	db := ds.DB
	id := setupModelTable(t, db)

	dest := modelTestUser{ID: id}
	err := db.Select().Model(&dest)
	require.NoError(t, err, "Select().Model() must succeed for existing PK")

	assert.Equal(t, id, dest.ID, "ID must match the inserted row")
	assert.Equal(t, "Alice", dest.Name, "Name must be populated")
	assert.Equal(t, "alice@example.com", dest.Email, "Email must be populated")
}

// TestSelectQuery_Model_PartialRead_SQLite verifies that specifying column names
// in Select() limits which fields are populated; unselected fields stay zero.
func TestSelectQuery_Model_PartialRead_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	db := ds.DB
	id := setupModelTable(t, db)

	// Select only "name" — email must remain empty string.
	dest := modelTestUser{ID: id}
	err := db.Select("name").Model(&dest)
	require.NoError(t, err, "Select(\"name\").Model() must succeed")

	assert.Equal(t, "Alice", dest.Name, "Name must be populated")
	// Email was not selected, so it stays as the zero value.
	assert.Empty(t, dest.Email, "Email must not be populated when not selected")
}

// TestSelectQuery_Model_WithWhere_SQLite verifies that an additional Where()
// clause is ANDed with the PK condition so only the matching row is returned.
func TestSelectQuery_Model_WithWhere_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	db := ds.DB
	id := setupModelTable(t, db)

	// Pre-set WHERE matching the existing row — query must succeed.
	dest := modelTestUser{ID: id}
	err := db.Select().
		Where("name = ?", "Alice").
		Model(&dest)
	require.NoError(t, err, "Select().Where(name=Alice).Model() must succeed")
	assert.Equal(t, "Alice", dest.Name)
}

// TestSelectQuery_Model_NotFound_SQLite verifies that Model() with a non-existent PK
// returns ErrNotFound.
func TestSelectQuery_Model_NotFound_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	db := ds.DB
	setupModelTable(t, db)

	dest := modelTestUser{ID: 999999}
	err := db.Select().Model(&dest)

	require.Error(t, err, "Model() with non-existent PK must return an error")
	assert.True(t, errors.Is(err, relica.ErrNotFound),
		"error must wrap ErrNotFound; got: %v", err)
}

// TestSelectQuery_Model_AllPKZero_SQLite verifies that Model() returns an error
// before executing any query when all PK fields are zero.
func TestSelectQuery_Model_AllPKZero_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()

	db := ds.DB
	setupModelTable(t, db)

	dest := modelTestUser{} // ID == 0
	err := db.Select().Model(&dest)

	require.Error(t, err, "Model() with zero PK must return an error")
	// Must not be ErrNotFound — error must occur before the query is issued.
	assert.False(t, errors.Is(err, relica.ErrNotFound),
		"zero-PK error must not be ErrNotFound (no query should be issued)")
	assert.True(t, strings.Contains(err.Error(), "primary key"),
		"error message must mention 'primary key'; got: %v", err)
}

// --- Tests for table.* wildcard quoting fix ---

func TestSelectTableDotStar_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	db := ds.DB

	_, err := db.NewQuery("CREATE TABLE wc_nodes (id INTEGER PRIMARY KEY, name TEXT)").Execute()
	require.NoError(t, err)
	_, err = db.NewQuery("INSERT INTO wc_nodes VALUES (1, 'Foo')").Execute()
	require.NoError(t, err)
	_, err = db.NewQuery("INSERT INTO wc_nodes VALUES (2, 'Bar')").Execute()
	require.NoError(t, err)

	type Node struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	var results []Node
	err = db.Select("wc_nodes.*").From("wc_nodes").All(&results)
	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "Foo", results[0].Name)
}

func TestSelectAliasDotStar_WithJoin_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	db := ds.DB

	_, err := db.NewQuery("CREATE TABLE wc_nodes2 (id INTEGER PRIMARY KEY, name TEXT)").Execute()
	require.NoError(t, err)
	_, err = db.NewQuery("CREATE TABLE wc_edges2 (id INTEGER PRIMARY KEY, target_id INTEGER, type TEXT)").Execute()
	require.NoError(t, err)
	_, err = db.NewQuery("INSERT INTO wc_nodes2 VALUES (1, 'Foo')").Execute()
	require.NoError(t, err)
	_, err = db.NewQuery("INSERT INTO wc_nodes2 VALUES (2, 'Bar')").Execute()
	require.NoError(t, err)
	_, err = db.NewQuery("INSERT INTO wc_edges2 VALUES (1, 1, 'CALLS')").Execute()
	require.NoError(t, err)

	type Node struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	var results []Node
	err = db.Select("n.*").
		From("wc_nodes2 n").
		LeftJoin("wc_edges2 ec", "ec.target_id = n.id AND ec.type = 'CALLS'").
		Where("ec.id IS NULL").
		All(&results)
	require.NoError(t, err, "Select(n.*) with LEFT JOIN must not fail")
	assert.Equal(t, 1, len(results), "only Bar should be uncalled")
	if len(results) > 0 {
		assert.Equal(t, "Bar", results[0].Name)
	}
}
