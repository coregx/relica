//go:build integration

package test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #47: BatchInsert(...).ValuesStructs(rows) upserts a slice of structs in
// one statement; a batch over the database's bind-parameter limit is an error
// sent nowhere, and the limit itself is exact on every database.

type bsNode struct {
	ID            int64  `db:"id,pk"`
	ProjectID     int    `db:"project_id"`
	QualifiedName string `db:"qualified_name"`
	Label         string `db:"label"`
}

func (bsNode) TableName() string { return "bs_node" }

func createBSNode(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	id := map[string]string{
		"postgres": "id SERIAL PRIMARY KEY",
		"mysql":    "id INT AUTO_INCREMENT PRIMARY KEY",
		"sqlite":   "id INTEGER PRIMARY KEY AUTOINCREMENT",
	}[dialect]
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS bs_node",
		"CREATE TABLE bs_node (" + id + ", project_id INT NOT NULL, qualified_name VARCHAR(100) NOT NULL, " +
			"label VARCHAR(100) NOT NULL, UNIQUE (project_id, qualified_name))",
	} {
		_, err := db.ExecContext(context.Background(), stmt)
		require.NoError(t, err, stmt)
	}
}

func resetBSNode(t *testing.T, db *relica.DB) {
	t.Helper()
	_, err := db.Delete("bs_node").AllRows().Execute()
	require.NoError(t, err)
}

func bsLabels(t *testing.T, db *relica.DB) map[string]string {
	t.Helper()
	var nodes []bsNode
	require.NoError(t, db.Select().From("bs_node").All(&nodes))
	out := make(map[string]string, len(nodes))
	for _, n := range nodes {
		out[n.QualifiedName] = n.Label
	}
	return out
}

func bsNodes(n int, label string) []bsNode {
	nodes := make([]bsNode, n)
	for i := range nodes {
		nodes[i] = bsNode{ProjectID: 1, QualifiedName: fmt.Sprintf("q%06d", i), Label: label}
	}
	return nodes
}

func runBatchStructsTests(t *testing.T, db *relica.DB, dialect string) {
	createBSNode(t, db, dialect)
	ctx := context.Background()

	t.Run("insert, then upsert from structs with DoUpdateAll", func(t *testing.T) {
		resetBSNode(t, db)
		_, err := db.BatchInsert("bs_node", nil).ValuesStructs([]bsNode{
			{ProjectID: 1, QualifiedName: "a", Label: "a1"},
			{ProjectID: 1, QualifiedName: "b", Label: "b1"},
		}).Execute()
		require.NoError(t, err)

		_, err = db.BatchInsert("bs_node", nil).ValuesStructs([]*bsNode{
			{ProjectID: 1, QualifiedName: "a", Label: "a2"},
			{ProjectID: 1, QualifiedName: "c", Label: "c1"},
		}).OnConflict("project_id", "qualified_name").DoUpdateAll().Execute()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"a": "a2", "b": "b1", "c": "c1"}, bsLabels(t, db))
	})

	t.Run("explicit columns with DoNothing keep existing rows", func(t *testing.T) {
		resetBSNode(t, db)
		cols := []string{"project_id", "qualified_name", "label"}
		_, err := db.BatchInsert("bs_node", cols).ValuesStructs([]bsNode{{ProjectID: 1, QualifiedName: "a", Label: "a1"}}).Execute()
		require.NoError(t, err)
		_, err = db.BatchInsert("bs_node", cols).ValuesStructs([]bsNode{
			{ProjectID: 1, QualifiedName: "a", Label: "a2"},
			{ProjectID: 1, QualifiedName: "b", Label: "b1"},
		}).OnConflict("project_id", "qualified_name").DoNothing().Execute()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"a": "a1", "b": "b1"}, bsLabels(t, db))
	})

	t.Run("in a transaction through the Executor", func(t *testing.T) {
		resetBSNode(t, db)
		err := db.Transactional(ctx, func(tx *relica.Tx) error {
			var ex relica.Executor = tx
			_, err := ex.BatchInsert("bs_node", nil).ValuesStructs(bsNodes(3, "t")).Execute()
			return err
		})
		require.NoError(t, err)
		assert.Len(t, bsLabels(t, db), 3)
	})

	// The derived columns are label, project_id, qualified_name: 3 parameters a row.
	limit := map[string]int{"postgres": 65535, "mysql": 65535, "sqlite": 32766}[dialect]
	maxRows := limit / 3

	t.Run("a batch at the limit runs", func(t *testing.T) {
		resetBSNode(t, db)
		_, err := db.BatchInsert("bs_node", nil).ValuesStructs(bsNodes(maxRows, "x")).Execute()
		require.NoError(t, err, "%d rows × 3 = %d parameters is within the %s limit", maxRows, maxRows*3, dialect)
		n, err := db.Select().From("bs_node").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(maxRows), n)
	})

	t.Run("a batch over the limit is an error and sends nothing", func(t *testing.T) {
		resetBSNode(t, db)
		_, err := db.BatchInsert("bs_node", nil).ValuesStructs(bsNodes(maxRows+1, "x")).Execute()
		require.ErrorContains(t, err, fmt.Sprintf("limit of %d", limit))
		require.ErrorContains(t, err, fmt.Sprintf("at most %d rows", maxRows))
		n, err := db.Select().From("bs_node").Count()
		require.NoError(t, err)
		assert.Zero(t, n)
	})

	t.Run("split batches inside Transactional are atomic", func(t *testing.T) {
		// The way the limit error names: slices of rows in one transaction.
		resetBSNode(t, db)
		nodes := bsNodes(4, "s")
		nodes[3].QualifiedName = nodes[0].QualifiedName // the second batch hits the unique key
		err := db.Transactional(ctx, func(tx *relica.Tx) error {
			for _, part := range [][]bsNode{nodes[:2], nodes[2:]} {
				if _, err := tx.BatchInsert("bs_node", nil).ValuesStructs(part).Execute(); err != nil {
					return err
				}
			}
			return nil
		})
		require.Error(t, err)
		assert.True(t, relica.IsUniqueViolation(err), "%v", err)
		assert.Empty(t, bsLabels(t, db), "the first batch is rolled back with the second")
	})

	t.Run("rows of two struct types are an error and send nothing", func(t *testing.T) {
		resetBSNode(t, db)
		type other struct {
			ProjectID int `db:"project_id"`
		}
		_, err := db.BatchInsertStruct("bs_node", []any{bsNode{ProjectID: 1, QualifiedName: "a", Label: "a"}, other{ProjectID: 1}}).Execute()
		require.ErrorContains(t, err, "rows must have one type")
		assert.Empty(t, bsLabels(t, db))
	})
}

// Entry points — one per database; CI selects the database with RELICA_TEST_DB.

func TestBatchStructs_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runBatchStructsTests(t, ds.DB, "postgres")
}

func TestBatchStructs_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runBatchStructsTests(t, ds.DB, "mysql")
}

func TestBatchStructs_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "batch.db"), relica.WithMaxOpenConns(1))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runBatchStructsTests(t, db, "sqlite")

	t.Run("over the limit in a transaction is still an error on SQLite", func(t *testing.T) {
		err := db.Transactional(context.Background(), func(tx *relica.Tx) error {
			_, err := tx.BatchInsert("bs_node", nil).ValuesStructs(bsNodes(32766/3+1, "x")).Execute()
			return err
		})
		require.ErrorContains(t, err, "limit of 32766")
	})
}
