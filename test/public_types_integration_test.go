//go:build integration
// +build integration

package test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v0.19.0 (ADR-016): the 20 public types that were aliases to internal
// packages are now declared in package relica. This suite drives every one of
// them against PostgreSQL, MySQL and SQLite, plus the bugs found while doing
// it: CASE with Expression operands, the invalid-LIKE filter that was silently
// dropped (an unconditional DELETE), and named parameters in HAVING.

// ptItem is the fixture row. Singular table name with TableName().
type ptItem struct {
	ID       int            `db:"id"`
	Name     string         `db:"name"`
	Nickname sql.NullString `db:"nickname"`
	Price    int            `db:"price"`
	MinPrice int            `db:"min_price"`
	State    int            `db:"state"`
	Note     sql.NullString `db:"note"`
}

func (ptItem) TableName() string { return "pt_item" }

func createPtItemTable(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()
	id := "id INTEGER PRIMARY KEY AUTOINCREMENT"
	switch dialect {
	case "postgres":
		id = "id SERIAL PRIMARY KEY"
	case "mysql":
		id = "id INT AUTO_INCREMENT PRIMARY KEY"
	}
	_, err := db.ExecContext(context.Background(), "CREATE TABLE IF NOT EXISTS pt_item ("+id+`,
		name VARCHAR(100) NOT NULL,
		nickname VARCHAR(100) NULL,
		price INT NOT NULL,
		min_price INT NOT NULL,
		state INT NOT NULL,
		note VARCHAR(100) NULL)`)
	require.NoError(t, err)
}

// seedPtItems resets the table to three rows:
//
//	alpha: nickname al,  price 100, min 50, state 1, note NULL
//	beta:  nickname NULL, price 30, min 40, state 2, note "50% off"
//	gamma: nickname ga,  price 70,  min 70, state 3, note "x_y"
func seedPtItems(t *testing.T, db *relica.DB) {
	t.Helper()
	_, err := db.Delete("pt_item").Execute()
	require.NoError(t, err)
	rows := []ptItem{
		{Name: "alpha", Nickname: sql.NullString{String: "al", Valid: true}, Price: 100, MinPrice: 50, State: 1},
		{Name: "beta", Price: 30, MinPrice: 40, State: 2, Note: sql.NullString{String: "50% off", Valid: true}},
		{Name: "gamma", Nickname: sql.NullString{String: "ga", Valid: true}, Price: 70, MinPrice: 70, State: 3,
			Note: sql.NullString{String: "x_y", Valid: true}},
	}
	for i := range rows {
		require.NoError(t, db.Model(&rows[i]).Insert())
	}
}

func ptNames(t *testing.T, q *relica.SelectQuery) []string {
	t.Helper()
	var names []string
	require.NoError(t, q.OrderBy("name").Column(&names))
	return names
}

func ptCount(t *testing.T, db *relica.DB) int64 {
	t.Helper()
	n, err := db.Select().From("pt_item").Count()
	require.NoError(t, err)
	return n
}

// reopen opens a second connection to the same database with options.
func reopen(t *testing.T, ds *DatabaseSetup, opts ...relica.Option) *relica.DB {
	t.Helper()
	require.NotEmpty(t, ds.DSN, "DatabaseSetup.DSN is required to reopen with options")
	db, err := relica.Open(ds.Dialect, ds.DSN, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func runPublicTypesTests(t *testing.T, ds *DatabaseSetup) {
	db := ds.DB
	createPtItemTable(t, db, ds.Dialect)
	ctx := context.Background()

	// ── HashExp ──────────────────────────────────────────────────────────

	t.Run("HashExp_InAndIsNull", func(t *testing.T) {
		seedPtItems(t, db)
		got := ptNames(t, db.Select("name").From("pt_item").
			Where(relica.HashExp{"state": []any{1, 2}, "note": nil}))
		assert.Equal(t, []string{"alpha"}, got)
	})

	t.Run("HashExp_EmptyListMatchesNothing", func(t *testing.T) {
		seedPtItems(t, db)
		// Negative: an empty IN list must never turn into "no condition".
		res, err := db.Delete("pt_item").Where(relica.HashExp{"state": []any{}}).Execute()
		require.NoError(t, err)
		n, _ := res.RowsAffected()
		assert.Zero(t, n)
		assert.Equal(t, int64(3), ptCount(t, db))
	})

	t.Run("HashExp_InOrAndJoinOn", func(t *testing.T) {
		seedPtItems(t, db)
		var got []string
		err := db.Select("p.name").From("pt_item p").
			InnerJoin("pt_item q", relica.And(relica.EqCol("q.id", "p.id"), relica.HashExp{"q.state": []any{1, 3}})).
			Where(relica.HashExp{"p.name": "gamma"}).
			OrWhere(relica.HashExp{"p.name": "alpha"}).
			OrderBy("p.name").Column(&got)
		require.NoError(t, err)
		assert.Equal(t, []string{"alpha", "gamma"}, got)
	})

	// ── Params ───────────────────────────────────────────────────────────

	t.Run("Params_WhereAndHaving", func(t *testing.T) {
		seedPtItems(t, db)
		got := ptNames(t, db.Select("name").From("pt_item").
			Where("price >= {:min} AND price <= {:max}", relica.Params{"min": 30, "max": 70}))
		assert.Equal(t, []string{"beta", "gamma"}, got)

		var states []int
		err := db.Select("state").From("pt_item").GroupBy("state").
			Having("MAX(price) > {:p}", relica.Params{"p": 60}).
			OrderBy("state").Column(&states)
		require.NoError(t, err)
		assert.Equal(t, []int{1, 3}, states)
	})

	t.Run("Params_BindParams", func(t *testing.T) {
		seedPtItems(t, db)
		var name string
		err := db.NewQuery("SELECT name FROM pt_item WHERE state = {:s} AND price > {:p}").
			BindParams(relica.Params{"s": 1, "p": 10}).Row(&name)
		require.NoError(t, err)
		assert.Equal(t, "alpha", name)
	})

	t.Run("Params_MissingName_DeletesNothing", func(t *testing.T) {
		seedPtItems(t, db)
		_, err := db.Delete("pt_item").Where("state = {:state}", relica.Params{"other": 1}).Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "state")
		assert.Equal(t, int64(3), ptCount(t, db))
	})

	// ── NullStringMap ────────────────────────────────────────────────────

	t.Run("NullStringMap_OneAllGeneric", func(t *testing.T) {
		seedPtItems(t, db)
		var row relica.NullStringMap
		require.NoError(t, db.Select("name", "nickname").From("pt_item").Where(relica.Eq("name", "beta")).One(&row))
		assert.Equal(t, "beta", row.String("name"))
		assert.True(t, row.IsNull("nickname"))
		assert.Equal(t, []string{"name", "nickname"}, row.Keys())

		var rows []relica.NullStringMap
		require.NoError(t, db.Select("name").From("pt_item").OrderBy("name").All(&rows))
		require.Len(t, rows, 3)
		assert.Equal(t, "gamma", rows[2].String("name"))

		g, err := relica.One[relica.NullStringMap](db.Select("nickname").From("pt_item").Where(relica.Eq("name", "alpha")))
		require.NoError(t, err)
		assert.Equal(t, "al", g.String("nickname"))
	})

	t.Run("NullStringMap_NotFound", func(t *testing.T) {
		seedPtItems(t, db)
		var row relica.NullStringMap
		err := db.Select().From("pt_item").Where(relica.Eq("name", "nobody")).One(&row)
		assert.ErrorIs(t, err, relica.ErrNotFound)
	})

	// ── Functional expressions ───────────────────────────────────────────

	t.Run("Case_SearchedWithExpressionsInSelect", func(t *testing.T) {
		seedPtItems(t, db)
		type tierRow struct {
			Name  string `db:"name"`
			Tier  string `db:"tier"`
			Label string `db:"label"`
		}
		var rows []tierRow
		err := db.Select("name").
			SelectSub(relica.CaseWhen().When(relica.GreaterThan("price", 60), "high").Else("low"), "tier").
			SelectSub(relica.Case("state").
				When(1, relica.Coalesce("nickname", "name")).
				Else(relica.Concat("name", "'!'")), "label").
			From("pt_item").OrderBy("name").All(&rows)
		require.NoError(t, err)
		assert.Equal(t, []tierRow{
			{Name: "alpha", Tier: "high", Label: "al"},
			{Name: "beta", Tier: "low", Label: "beta!"},
			{Name: "gamma", Tier: "high", Label: "gamma!"},
		}, rows)
	})

	// Integer CASE results must sort as numbers: as text, 10 sorts before 2.
	t.Run("Case_IntegerResultsSortAsNumbers", func(t *testing.T) {
		seedPtItems(t, db)
		var got []string
		require.NoError(t, db.Select("name").From("pt_item").
			OrderBySub(relica.CaseWhen().
				When(relica.Eq("name", "alpha"), 10).
				When(relica.Eq("name", "beta"), 2).
				Else(1)).
			Column(&got))
		assert.Equal(t, []string{"gamma", "beta", "alpha"}, got)
	})

	t.Run("Case_ExpressionConditionInUpdateSet", func(t *testing.T) {
		seedPtItems(t, db)
		_, err := db.Update("pt_item").Set(map[string]any{
			"state": relica.CaseWhen().When(relica.GreaterThan("price", 60), 9).Else(0),
		}).Execute()
		require.NoError(t, err)
		got := ptNames(t, db.Select("name").From("pt_item").Where(relica.Eq("state", 9)))
		assert.Equal(t, []string{"alpha", "gamma"}, got)
	})

	t.Run("GreatestLeastNullIf", func(t *testing.T) {
		seedPtItems(t, db)
		type row struct {
			Name string        `db:"name"`
			Hi   int           `db:"hi"`
			Lo   int           `db:"lo"`
			NI   sql.NullInt64 `db:"ni"`
		}
		var rows []row
		err := db.Select("name").
			SelectSub(relica.Greatest("price", "min_price"), "hi").
			SelectSub(relica.Least("price", "min_price"), "lo").
			SelectSub(relica.NullIf("price", "min_price"), "ni").
			From("pt_item").OrderBy("name").All(&rows)
		require.NoError(t, err)
		require.Len(t, rows, 3)
		assert.Equal(t, row{Name: "alpha", Hi: 100, Lo: 50, NI: sql.NullInt64{Int64: 100, Valid: true}}, rows[0])
		assert.Equal(t, row{Name: "beta", Hi: 40, Lo: 30, NI: sql.NullInt64{Int64: 30, Valid: true}}, rows[1])
		assert.Equal(t, row{Name: "gamma", Hi: 70, Lo: 70}, rows[2])
	})

	t.Run("Coalesce_InUpdateSet", func(t *testing.T) {
		seedPtItems(t, db)
		_, err := db.Update("pt_item").Set(map[string]any{"nickname": relica.Coalesce("nickname", "name")}).Execute()
		require.NoError(t, err)
		var nick string
		require.NoError(t, db.Select("nickname").From("pt_item").Where(relica.Eq("name", "beta")).Row(&nick))
		assert.Equal(t, "beta", nick)
	})

	// ── LikeExp ──────────────────────────────────────────────────────────

	t.Run("Like_EscapesWildcards", func(t *testing.T) {
		seedPtItems(t, db)
		assert.Equal(t, []string{"beta"}, ptNames(t, db.Select("name").From("pt_item").Where(relica.Like("note", "50%"))))
		assert.Equal(t, []string{"gamma"}, ptNames(t, db.Select("name").From("pt_item").Where(relica.Like("note", "x_y"))))
		// Negative: escaped wildcards must not match as wildcards.
		assert.Empty(t, ptNames(t, db.Select("name").From("pt_item").Where(relica.Like("note", "x%y"))))
		assert.Empty(t, ptNames(t, db.Select("name").From("pt_item").Where(relica.Like("note", "5_%"))))
		// Prefix match only.
		assert.Equal(t, []string{"beta"}, ptNames(t, db.Select("name").From("pt_item").
			Where(relica.Like("note", "50").Match(false, true))))
	})

	t.Run("Like_InvalidEscape_DeletesNothing", func(t *testing.T) {
		seedPtItems(t, db)
		bad := relica.Like("name", "zzz").EscapeChars("!")
		require.Error(t, bad.Err())
		_, err := db.Delete("pt_item").Where(bad).Execute()
		require.Error(t, err, "an invalid filter must fail, not run an unconditional DELETE")
		assert.Equal(t, int64(3), ptCount(t, db))

		_, err = db.Update("pt_item").Set(map[string]any{"state": 0}).
			Where(relica.And(relica.Eq("state", 1), relica.Not(bad))).Execute()
		require.Error(t, err)
		got := ptNames(t, db.Select("name").From("pt_item").Where(relica.Eq("state", 0)))
		assert.Empty(t, got)
	})

	// ── QueryHook, QueryEvent, Logger ────────────────────────────────────

	t.Run("QueryHook_ReceivesEvents", func(t *testing.T) {
		seedPtItems(t, db)
		var mu sync.Mutex
		var events []relica.QueryEvent
		hdb := reopen(t, ds, relica.WithQueryHook(func(_ context.Context, e relica.QueryEvent) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		}))
		_, err := hdb.Update("pt_item").Set(map[string]any{"state": 5}).Where(relica.Eq("name", "beta")).Execute()
		require.NoError(t, err)
		var missing ptItem
		err = hdb.Select().From("pt_item").Where(relica.Eq("name", "nobody")).One(&missing)
		require.ErrorIs(t, err, relica.ErrNotFound)

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, events, 2)
		assert.Equal(t, "UPDATE", events[0].Operation)
		assert.Equal(t, int64(1), events[0].RowsAffected)
		assert.NoError(t, events[0].Error)
		assert.Equal(t, "SELECT", events[1].Operation)
		assert.ErrorIs(t, events[1].Error, relica.ErrNotFound)
	})

	t.Run("WithLogger_SlogAdapter", func(t *testing.T) {
		seedPtItems(t, db)
		var buf bytes.Buffer
		ldb := reopen(t, ds, relica.WithLogger(relica.NewSlogAdapter(slog.New(slog.NewTextHandler(&buf, nil)))))
		_, err := ldb.Select().From("pt_item").Count()
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "query executed")
		assert.Contains(t, buf.String(), "pt_item")
	})

	// ── TxOptions, PoolStats, QueryPlan ──────────────────────────────────

	t.Run("TxOptions_ReadOnlyRejectsWrites", func(t *testing.T) {
		if ds.Dialect == "sqlite" {
			t.Skip("SQLite has no read-only transactions; covered by TxOptions_SQLite")
		}
		seedPtItems(t, db)
		tx, err := db.BeginTx(ctx, &relica.TxOptions{ReadOnly: true})
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		n, err := tx.Select().From("pt_item").Count()
		require.NoError(t, err)
		assert.Equal(t, int64(3), n)
		_, err = tx.Delete("pt_item").Execute()
		require.Error(t, err, "a write inside a read-only transaction must fail")
		_ = tx.Rollback()
		assert.Equal(t, int64(3), ptCount(t, db))
	})

	t.Run("TxOptions_IsolationLevel", func(t *testing.T) {
		if ds.Dialect == "sqlite" {
			t.Skip("covered by TxOptions_SQLite")
		}
		seedPtItems(t, db)
		err := db.TransactionalTx(ctx, &relica.TxOptions{Isolation: sql.LevelSerializable}, func(tx *relica.Tx) error {
			_, err := tx.Update("pt_item").Set(map[string]any{"state": 7}).Where(relica.Eq("name", "alpha")).Execute()
			return err
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"alpha"}, ptNames(t, db.Select("name").From("pt_item").Where(relica.Eq("state", 7))))
	})

	t.Run("PoolStats", func(t *testing.T) {
		_, err := db.Select().From("pt_item").Count()
		require.NoError(t, err)
		var stats relica.PoolStats = db.Stats()
		assert.GreaterOrEqual(t, stats.OpenConnections, 1)
		assert.GreaterOrEqual(t, stats.OpenConnections, stats.InUse)
	})

	t.Run("QueryPlan_Explain", func(t *testing.T) {
		seedPtItems(t, db)
		plan, err := db.Select().From("pt_item").Where(relica.Eq("state", 1)).Explain()
		require.NoError(t, err)
		require.NotNil(t, plan)
		var _ *relica.QueryPlan = plan
		assert.NotEmpty(t, plan.RawOutput)
		assert.True(t, strings.HasPrefix(plan.Database, ds.Dialect[:3]), "Database = %q", plan.Database)
	})

	t.Run("QueryPlan_ExplainAnalyze", func(t *testing.T) {
		seedPtItems(t, db)
		q := db.Select().From("pt_item").Where(relica.Eq("state", 1))
		plan, err := q.ExplainAnalyze()
		if ds.Dialect == "sqlite" {
			// Negative: SQLite has no EXPLAIN ANALYZE; the error must be explicit.
			require.Error(t, err)
			assert.Nil(t, plan)
			return
		}
		require.NoError(t, err)
		require.NotNil(t, plan)
		assert.NotEmpty(t, plan.RawOutput)
		assert.Equal(t, int64(1), plan.ActualRows, "one row has state = 1; raw:\n%s", plan.RawOutput)
		assert.Positive(t, plan.ActualTime, "raw:\n%s", plan.RawOutput)
		assert.True(t, plan.FullScan, "no index on state: expected a full scan; raw:\n%s", plan.RawOutput)
	})

	// ── BeforeInserter ───────────────────────────────────────────────────

	t.Run("BeforeInserter_SetsFieldsAndCanAbort", func(t *testing.T) {
		seedPtItems(t, db)
		row := &hookedItem{Name: "delta", Price: 1, MinPrice: 1, State: 1}
		require.NoError(t, db.Model(row).Insert())
		var note string
		require.NoError(t, db.Select("note").From("pt_item").Where(relica.Eq("name", "delta")).Row(&note))
		assert.Equal(t, "stamped", note)

		// Negative: an error from BeforeInsert aborts the insert.
		bad := &hookedItem{Name: "epsilon", Price: 1, MinPrice: 1, State: 1, fail: true}
		err := db.Model(bad).Insert()
		require.ErrorIs(t, err, errHookRejected)
		assert.Empty(t, ptNames(t, db.Select("name").From("pt_item").Where(relica.Eq("name", "epsilon"))))
	})
}

var errHookRejected = errors.New("rejected by BeforeInsert")

// hookedItem implements relica.BeforeInserter. It declares its fields instead
// of embedding ptItem: the Model API does not walk embedded structs
// (V1-DESIGN-DEBT #7).
type hookedItem struct {
	ID       int            `db:"id"`
	Name     string         `db:"name"`
	Price    int            `db:"price"`
	MinPrice int            `db:"min_price"`
	State    int            `db:"state"`
	Note     sql.NullString `db:"note"`
	fail     bool
}

func (hookedItem) TableName() string { return "pt_item" }

var _ relica.BeforeInserter = (*hookedItem)(nil)

func (h *hookedItem) BeforeInsert() error {
	if h.fail {
		return errHookRejected
	}
	h.Note = sql.NullString{String: "stamped", Valid: true}
	return nil
}

// ---------------------------------------------------------------------------
// Entry points — one per database; CI selects the database with RELICA_TEST_DB.
// ---------------------------------------------------------------------------

func TestPublicTypes_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runPublicTypesTests(t, ds)
}

func TestPublicTypes_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runPublicTypesTests(t, ds)
}

// TestPublicTypes_SQLite uses a file so the suite can reopen the same database
// with options (hooks, logger).
func TestPublicTypes_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	path := filepath.Join(t.TempDir(), "public_types.db")
	db, err := relica.NewDB("sqlite", path)
	require.NoError(t, err)
	ds := &DatabaseSetup{DB: db, Dialect: "sqlite", DSN: path}
	defer ds.Close()
	runPublicTypesTests(t, ds)

	// SQLite cannot honor ReadOnly or Serializable through database/sql
	// with this driver in the same way; pin down what actually happens so a
	// driver change is noticed.
	t.Run("TxOptions_SQLite", func(t *testing.T) {
		tx, err := db.BeginTx(context.Background(), &relica.TxOptions{ReadOnly: true})
		if err != nil {
			t.Logf("BeginTx(ReadOnly) rejected by driver: %v", err)
			return
		}
		defer func() { _ = tx.Rollback() }()
		_, err = tx.Select().From("pt_item").Count()
		require.NoError(t, err)
	})
}
