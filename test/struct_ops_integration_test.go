//go:build integration

package test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// InsertStruct, BatchInsertStruct, UpdateStruct and BatchUpdate on real
// databases, through every receiver (*DB, *DB.Builder(), *Tx, *Tx.Builder()).
// Each case recreates its tables, so generated keys start at 1 on every
// dialect and are asserted exactly.

// SoAudit is embedded in soItem: its fields are flattened into columns.
type SoAudit struct {
	CreatedBy string // untagged → created_by
	UpdatedBy string `db:"updated_by"`
}

// soItem maps to so_item. ID is the primary key by name; it is zero for
// generated keys.
type soItem struct {
	SoAudit
	ID        int    `db:"id"`
	Title     string `db:"title"`
	UnitPrice int    // untagged → unit_price
	UserID    int    // untagged → user_id
	HTTPLog   string // untagged → httplog (default ozzo-dbx snake case)
	Note      string `db:"-"`
}

// soTagged maps to so_tagged: the primary key is a pk tag, not a field named ID.
type soTagged struct {
	Key   int    `db:"item_key,pk"`
	Label string `db:"label"`
}

// soLabel has no primary key at all.
type soLabel struct {
	Label string `db:"label"`
}

type soBadEmbeddedPtr struct {
	*SoAudit
	ID int `db:"id"`
}

type soBadEmbeddedTag struct {
	SoAudit `db:"audit"`
	ID      int `db:"id"`
}

// soOps is the struct-operation surface shared by *DB, *Tx and *QueryBuilder.
type soOps interface {
	InsertStruct(table string, data any) *relica.Query
	BatchInsertStruct(table string, data any) *relica.Query
	UpdateStruct(table string, data any) *relica.UpdateQuery
	BatchUpdate(table, keyColumn string) *relica.BatchUpdateQuery
}

var (
	_ soOps = (*relica.DB)(nil)
	_ soOps = (*relica.Tx)(nil)
	_ soOps = (*relica.QueryBuilder)(nil)
)

// soReceiver runs write with one receiver; a transaction is committed after
// write returns, so the caller verifies through db afterwards.
type soReceiver struct {
	name string
	run  func(t *testing.T, write func(ops soOps))
}

func soReceivers(db *relica.DB) []soReceiver {
	inTx := func(pick func(*relica.Tx) soOps) func(*testing.T, func(soOps)) {
		return func(t *testing.T, write func(soOps)) {
			tx, err := db.Begin(context.Background())
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }() // sql.ErrTxDone after Commit
			write(pick(tx))
			require.NoError(t, tx.Commit())
		}
	}
	return []soReceiver{
		{"DB", func(_ *testing.T, write func(soOps)) { write(db) }},
		{"Builder", func(_ *testing.T, write func(soOps)) { write(db.Builder()) }},
		{"Tx", inTx(func(tx *relica.Tx) soOps { return tx })},
		{"TxBuilder", inTx(func(tx *relica.Tx) soOps { return tx.Builder() })},
	}
}

func soResetTables(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	intType := "INTEGER"
	autoKey := func(col string) string { return col + " INTEGER PRIMARY KEY AUTOINCREMENT" }
	switch dialect {
	case "postgres":
		autoKey = func(col string) string { return col + " SERIAL PRIMARY KEY" }
	case "mysql":
		intType = "INT"
		autoKey = func(col string) string { return col + " INT AUTO_INCREMENT PRIMARY KEY" }
	}
	ctx := context.Background()
	for _, s := range []string{
		"DROP TABLE IF EXISTS so_item",
		"DROP TABLE IF EXISTS so_tagged",
		"CREATE TABLE so_item (" + autoKey("id") + ", title VARCHAR(50) NOT NULL, unit_price " + intType +
			" NOT NULL, user_id " + intType + " NOT NULL, httplog VARCHAR(50) NOT NULL, " +
			"created_by VARCHAR(30) NOT NULL, updated_by VARCHAR(30) NOT NULL)",
		"CREATE TABLE so_tagged (" + autoKey("item_key") + ", label VARCHAR(50) NOT NULL)",
	} {
		_, err := db.ExecContext(ctx, s)
		require.NoError(t, err, s)
	}
}

// soAffected executes q and returns RowsAffected.
func soAffected(t *testing.T, q interface{ Execute() (sql.Result, error) }) int64 {
	t.Helper()
	res, err := q.Execute()
	require.NoError(t, err)
	n, err := res.RowsAffected()
	require.NoError(t, err)
	return n
}

// soRaw reads one row without a struct: the keys prove the column names.
func soRaw(t *testing.T, db *relica.DB, table string, cond relica.Expression) map[string]string {
	t.Helper()
	var raw relica.NullStringMap
	require.NoError(t, db.Select().From(table).Where(cond).One(&raw))
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		require.True(t, v.Valid, "column %s is NULL", k)
		out[k] = v.String
	}
	return out
}

func soItems(t *testing.T, db *relica.DB) []soItem {
	t.Helper()
	var got []soItem
	require.NoError(t, db.Select().From("so_item").OrderBy("id").All(&got))
	return got
}

func soCount(t *testing.T, db *relica.DB, table string) int64 {
	t.Helper()
	n, err := db.Select().From(table).Count()
	require.NoError(t, err)
	return n
}

func soQuote(dialect, col string) string {
	if dialect == "mysql" || dialect == "sqlite" {
		return "`" + col + "`"
	}
	return `"` + col + `"`
}

func soItemOf(id int, title string, price int) soItem {
	return soItem{
		SoAudit:   SoAudit{CreatedBy: "ann-" + title, UpdatedBy: "bob-" + title},
		ID:        id,
		Title:     title,
		UnitPrice: price,
		UserID:    price + 1,
		HTTPLog:   "GET /" + title,
	}
}

// soSeed inserts so_item rows with explicit ids through plain Insert, so the
// struct operations under test are not used to prepare their own input.
func soSeed(t *testing.T, db *relica.DB, dialect string, items ...soItem) {
	t.Helper()
	for _, it := range items {
		_, err := db.Insert("so_item", map[string]any{
			"id": it.ID, "title": it.Title, "unit_price": it.UnitPrice, "user_id": it.UserID,
			"httplog": it.HTTPLog, "created_by": it.CreatedBy, "updated_by": it.UpdatedBy,
		}).Execute()
		require.NoError(t, err)
	}
	// An explicit id does not advance a PostgreSQL SERIAL sequence (SQLite and
	// MySQL move their counters past the largest key), so a later generated
	// key would collide with a seeded one.
	if dialect == "postgres" {
		_, err := db.ExecContext(context.Background(),
			"SELECT setval(pg_get_serial_sequence('so_item', 'id'), (SELECT MAX(id) FROM so_item))")
		require.NoError(t, err)
	}
}

func runStructOpsTests(t *testing.T, db *relica.DB, dialect string) {
	for _, r := range soReceivers(db) {
		t.Run(r.name, func(t *testing.T) {
			runStructOpsReceiver(t, db, dialect, r)
		})
	}

	t.Run("Tx_RollbackDiscardsEveryStructOp", func(t *testing.T) {
		soResetTables(t, db, dialect)
		soSeed(t, db, dialect, soItemOf(1, "seed", 10))

		tx, err := db.Begin(context.Background())
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		assert.Equal(t, int64(1), soAffected(t, tx.InsertStruct("so_item", &soItem{Title: "tx-one"})))
		assert.Equal(t, int64(2), soAffected(t, tx.Builder().BatchInsertStruct("so_item",
			[]soItem{{Title: "tx-two"}, {Title: "tx-three"}})))
		upd := soItemOf(1, "tx-upd", 99)
		assert.Equal(t, int64(1), soAffected(t, tx.UpdateStruct("so_item", &upd).Where(relica.Eq("id", 1))))
		assert.Equal(t, int64(2), soAffected(t, tx.BatchUpdate("so_item", "id").
			Set(2, map[string]any{"unit_price": 22}).
			Set(3, map[string]any{"unit_price": 33})))

		// Inside the transaction every change is visible; the listed columns
		// let a partial select scan into the full struct.
		var inside []soItem
		require.NoError(t, tx.Select("id", "title", "unit_price").From("so_item").OrderBy("id").All(&inside))
		require.Len(t, inside, 4)
		assert.Equal(t, []int{1, 2, 3, 4}, []int{inside[0].ID, inside[1].ID, inside[2].ID, inside[3].ID})
		assert.Equal(t, []string{"tx-upd", "tx-one", "tx-two", "tx-three"},
			[]string{inside[0].Title, inside[1].Title, inside[2].Title, inside[3].Title})
		assert.Equal(t, []int{99, 22, 33, 0}, []int{inside[0].UnitPrice, inside[1].UnitPrice, inside[2].UnitPrice, inside[3].UnitPrice})

		require.NoError(t, tx.Rollback())
		assert.Equal(t, []soItem{soItemOf(1, "seed", 10)}, soItems(t, db), "rollback leaves only the seed row, unchanged")
	})
}

func runStructOpsReceiver(t *testing.T, db *relica.DB, dialect string, r soReceiver) {
	t.Run("InsertStruct", func(t *testing.T) {
		soResetTables(t, db, dialect)
		first := soItemOf(0, "apple", 150)
		first.Note = "not stored"
		second := soItemOf(0, "pear", 70)
		explicit := soItemOf(100, "plum", 30)
		r.run(t, func(ops soOps) {
			assert.Equal(t, int64(1), soAffected(t, ops.InsertStruct("so_item", &first)), "pointer")
			assert.Equal(t, int64(1), soAffected(t, ops.InsertStruct("so_item", second)), "value")
			assert.Equal(t, int64(1), soAffected(t, ops.InsertStruct("so_item", &explicit)), "explicit key")
			assert.Equal(t, int64(1), soAffected(t, ops.InsertStruct("so_tagged", &soTagged{Label: "tagged"})))
		})

		assert.Equal(t, map[string]string{
			"id": "1", "title": "apple", "unit_price": "150", "user_id": "151", "httplog": "GET /apple",
			"created_by": "ann-apple", "updated_by": "bob-apple",
		}, soRaw(t, db, "so_item", relica.Eq("id", 1)), "zero ID left out and generated; untagged and embedded columns named by snake case")

		want := []soItem{soItemOf(1, "apple", 150), soItemOf(2, "pear", 70), soItemOf(100, "plum", 30)}
		assert.Equal(t, want, soItems(t, db), "db:\"-\" field is neither written nor read")

		assert.Equal(t, map[string]string{"item_key": "1", "label": "tagged"},
			soRaw(t, db, "so_tagged", relica.Eq("label", "tagged")), "zero pk-tagged key left out and generated")
	})

	t.Run("BatchInsertStruct", func(t *testing.T) {
		soResetTables(t, db, dialect)
		generated := []soItem{soItemOf(0, "a", 1), soItemOf(0, "b", 2), soItemOf(0, "c", 3)}
		explicit := []*soItem{ptrItem(soItemOf(10, "x", 10)), ptrItem(soItemOf(11, "y", 11))}
		tagged := []soTagged{{Label: "t1"}, {Label: "t2"}}

		sqlGen, _ := db.BatchInsertStruct("so_item", generated).ToSQL()
		assert.NotContains(t, sqlGen, soQuote(dialect, "id"), "all-zero keys: id column left out")
		assert.Contains(t, sqlGen, soQuote(dialect, "unit_price"))
		sqlExp, _ := db.BatchInsertStruct("so_item", explicit).ToSQL()
		assert.Contains(t, sqlExp, soQuote(dialect, "id"), "non-zero keys: id column kept")

		r.run(t, func(ops soOps) {
			assert.Equal(t, int64(3), soAffected(t, ops.BatchInsertStruct("so_item", generated)), "values, generated keys")
			assert.Equal(t, int64(2), soAffected(t, ops.BatchInsertStruct("so_item", explicit)), "pointers, explicit keys")
			assert.Equal(t, int64(2), soAffected(t, ops.BatchInsertStruct("so_tagged", tagged)))
		})

		assert.Equal(t, map[string]string{
			"id": "2", "title": "b", "unit_price": "2", "user_id": "3", "httplog": "GET /b",
			"created_by": "ann-b", "updated_by": "bob-b",
		}, soRaw(t, db, "so_item", relica.Eq("id", 2)))
		assert.Equal(t, []soItem{
			soItemOf(1, "a", 1), soItemOf(2, "b", 2), soItemOf(3, "c", 3),
			soItemOf(10, "x", 10), soItemOf(11, "y", 11),
		}, soItems(t, db))

		var tags []soTagged
		require.NoError(t, db.Select().From("so_tagged").OrderBy("item_key").All(&tags))
		assert.Equal(t, []soTagged{{Key: 1, Label: "t1"}, {Key: 2, Label: "t2"}}, tags)
	})

	t.Run("UpdateStruct", func(t *testing.T) {
		soResetTables(t, db, dialect)
		soSeed(t, db, dialect, soItemOf(1, "old", 10), soItemOf(2, "keep", 20))
		_, err := db.Insert("so_tagged", map[string]any{"item_key": 1, "label": "a"}).Execute()
		require.NoError(t, err)
		_, err = db.Insert("so_tagged", map[string]any{"item_key": 2, "label": "b"}).Execute()
		require.NoError(t, err)

		upd := soItemOf(1, "new", 0) // UnitPrice 0: every column is set, zero values too
		upd.CreatedBy = "carol"
		r.run(t, func(ops soOps) {
			assert.Equal(t, int64(1), soAffected(t, ops.UpdateStruct("so_item", &upd).Where(relica.Eq("id", 1))))
			assert.Equal(t, int64(2), soAffected(t, ops.UpdateStruct("so_tagged", soLabel{Label: "all"}).AllRows()),
				"AllRows updates the whole table on purpose")
		})

		assert.Equal(t, map[string]string{
			"id": "1", "title": "new", "unit_price": "0", "user_id": "1", "httplog": "GET /new",
			"created_by": "carol", "updated_by": "bob-new",
		}, soRaw(t, db, "so_item", relica.Eq("id", 1)))
		assert.Equal(t, []soItem{upd, soItemOf(2, "keep", 20)}, soItems(t, db), "row outside Where unchanged")

		var labels []string
		require.NoError(t, db.Select("label").From("so_tagged").OrderBy("item_key").Column(&labels))
		assert.Equal(t, []string{"all", "all"}, labels)

		// The primary key is never written, as in Model().Update(): a struct
		// with a zero key updates the row Where selects and keeps its key.
		renamed := soItemOf(0, "renamed", 7)
		r.run(t, func(ops soOps) {
			assert.Equal(t, int64(1), soAffected(t, ops.UpdateStruct("so_item", &renamed).Where(relica.Eq("title", "keep"))))
			assert.Equal(t, int64(1), soAffected(t, ops.UpdateStruct("so_tagged", &soTagged{Label: "two"}).
				Where(relica.Eq("item_key", 2))))
		})
		assert.Equal(t, "2", soRaw(t, db, "so_item", relica.Eq("title", "renamed"))["id"], "id kept")
		assert.Equal(t, map[string]string{"item_key": "2", "label": "two"}, soRaw(t, db, "so_tagged", relica.Eq("label", "two")),
			"pk-tagged key kept")
	})

	t.Run("BatchUpdate", func(t *testing.T) {
		soResetTables(t, db, dialect)
		soSeed(t, db, dialect, soItemOf(1, "one", 10), soItemOf(2, "two", 20), soItemOf(3, "three", 30))
		r.run(t, func(ops soOps) {
			assert.Equal(t, int64(2), soAffected(t, ops.BatchUpdate("so_item", "id").
				Set(1, map[string]any{"title": "uno", "unit_price": 11}).
				Set(2, map[string]any{"title": "dos"})))
		})

		assert.Equal(t, map[string]string{
			"id": "1", "title": "uno", "unit_price": "11", "user_id": "11", "httplog": "GET /one",
			"created_by": "ann-one", "updated_by": "bob-one",
		}, soRaw(t, db, "so_item", relica.Eq("id", 1)))

		two := soItemOf(2, "two", 20)
		two.Title = "dos" // unit_price not in its Set: the ELSE branch keeps 20
		one := soItemOf(1, "one", 10)
		one.Title, one.UnitPrice = "uno", 11
		assert.Equal(t, []soItem{one, two, soItemOf(3, "three", 30)}, soItems(t, db), "row 3 outside the batch unchanged")
	})

	t.Run("Errors", func(t *testing.T) {
		soResetTables(t, db, dialect)
		soSeed(t, db, dialect, soItemOf(1, "seed", 10))
		var nilItem *soItem
		r.run(t, func(ops soOps) {
			upd := soItemOf(1, "changed", 99)
			_, err := ops.UpdateStruct("so_item", &upd).Execute()
			require.ErrorContains(t, err, "without Where would affect every row")

			_, err = ops.BatchInsertStruct("so_item", []soItem{soItemOf(0, "gen", 1), soItemOf(5, "fixed", 2)}).Execute()
			require.ErrorContains(t, err, "1 of 2 rows have a zero primary key")

			_, err = ops.BatchInsertStruct("so_item", soItemOf(0, "single", 1)).Execute()
			require.ErrorContains(t, err, "BatchInsertStruct expects a slice")

			_, err = ops.BatchInsertStruct("so_item", []soItem{}).Execute()
			require.ErrorContains(t, err, "BatchInsertStruct: empty slice")

			_, err = ops.BatchUpdate("so_item", "id").Execute()
			require.ErrorContains(t, err, "no updates to apply")

			_, err = ops.InsertStruct("so_item", 42).Execute()
			require.ErrorContains(t, err, "expected a struct or *struct")

			_, err = ops.InsertStruct("so_item", nilItem).Execute()
			require.ErrorContains(t, err, "nil pointer")

			_, err = ops.InsertStruct("so_item", &soBadEmbeddedPtr{SoAudit: &SoAudit{}, ID: 7}).Execute()
			require.ErrorContains(t, err, "embedded pointers are not supported")

			_, err = ops.UpdateStruct("so_item", &soBadEmbeddedTag{ID: 7}).Where(relica.Eq("id", 1)).Execute()
			require.ErrorContains(t, err, "db tag on an embedded struct")
		})
		assert.Equal(t, []soItem{soItemOf(1, "seed", 10)}, soItems(t, db), "no failed operation changed or added a row")
		assert.Equal(t, int64(0), soCount(t, db, "so_tagged"))
	})
}

func ptrItem(it soItem) *soItem { return &it }

func TestStructOps_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runStructOpsTests(t, ds.DB, "postgres")
}

func TestStructOps_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runStructOpsTests(t, ds.DB, "mysql")
}

func TestStructOps_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "so.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runStructOpsTests(t, db, "sqlite")
}
