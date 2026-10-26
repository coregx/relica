//go:build integration

package test

import (
	"context"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ADR-018 on real databases: derived table/column names, embedded structs on
// write, TableName receivers, legacy db:"pk", ScanMode, WrapDB options.

type NmAudit struct {
	UpdatedBy string
}

// NmUserProfile has no TableName and no db tags: every name is derived.
type NmUserProfile struct {
	NmAudit
	ID          int
	DisplayName string
	CreatedAt   string
}

type nmPtrRecv struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

func (*nmPtrRecv) TableName() string { return "nm_custom" }

type nmLegacyPK struct {
	UserKey int `db:"pk"`
	Name    string
}

func (nmLegacyPK) TableName() string { return "nm_legacy" }

type nmHTTP struct {
	ID         int `db:"id"`
	HTTPStatus int
}

func (nmHTTP) TableName() string { return "nm_http" }

type nmWithExtra struct {
	ID          int    `db:"id"`
	DisplayName string `db:"display_name"`
	Missing     string `db:"missing"`
}

func runNamingV020Tests(t *testing.T, db *relica.DB, dialect string) {
	ctx := context.Background()
	intType := "INTEGER"
	if dialect == "mysql" {
		intType = "INT"
	}
	for _, s := range []string{
		"DROP TABLE IF EXISTS nm_user_profile",
		"DROP TABLE IF EXISTS nm_custom",
		"DROP TABLE IF EXISTS nm_legacy",
		"DROP TABLE IF EXISTS nm_http",
		"CREATE TABLE nm_user_profile (id " + intType + " PRIMARY KEY, display_name VARCHAR(50), created_at VARCHAR(30), updated_by VARCHAR(30))",
		"CREATE TABLE nm_custom (id " + intType + " PRIMARY KEY, name VARCHAR(50))",
		"CREATE TABLE nm_legacy (user_key " + intType + " PRIMARY KEY, name VARCHAR(50))",
		"CREATE TABLE nm_http (id " + intType + " PRIMARY KEY, http_status " + intType + ")",
	} {
		_, err := db.ExecContext(ctx, s)
		require.NoError(t, err, s)
	}

	t.Run("derived table and columns, embedded flattened on write and read", func(t *testing.T) {
		in := NmUserProfile{NmAudit: NmAudit{UpdatedBy: "ops"}, ID: 1, DisplayName: "Ann", CreatedAt: "2026-10-26"}
		require.NoError(t, db.Model(&in).Insert())

		var raw relica.NullStringMap
		require.NoError(t, db.Select().From("nm_user_profile").Where(relica.Eq("id", 1)).One(&raw))
		keys := raw.Keys()
		sort.Strings(keys)
		assert.Equal(t, []string{"created_at", "display_name", "id", "updated_by"}, keys)
		assert.Equal(t, "ops", raw["updated_by"].String)

		var out NmUserProfile
		require.NoError(t, db.Select().From("nm_user_profile").Where(relica.Eq("id", 1)).One(&out))
		assert.Equal(t, in, out)

		in.DisplayName, in.UpdatedBy = "Ann B", "admin"
		require.NoError(t, db.Model(&in).Update())
		var again NmUserProfile
		require.NoError(t, db.Select().From("nm_user_profile").Where(relica.Eq("id", 1)).One(&again))
		assert.Equal(t, in, again)
	})

	t.Run("pointer-receiver TableName on a value model", func(t *testing.T) {
		require.NoError(t, db.Model(nmPtrRecv{ID: 1, Name: "value model"}).Insert())
		var got nmPtrRecv
		require.NoError(t, db.Select().From("nm_custom").Where(relica.Eq("id", 1)).One(&got))
		assert.Equal(t, "value model", got.Name)
	})

	t.Run("legacy db:pk tag", func(t *testing.T) {
		m := nmLegacyPK{UserKey: 7, Name: "first"}
		require.NoError(t, db.Model(&m).Insert())
		m.Name = "second"
		require.NoError(t, db.Model(&m).Update())
		var got nmLegacyPK
		require.NoError(t, db.Select().From("nm_legacy").Where(relica.Eq("user_key", 7)).One(&got))
		assert.Equal(t, m, got)
	})

	t.Run("ScanRequireFields default, lenient and listed columns", func(t *testing.T) {
		var x nmWithExtra
		err := db.Select().From("nm_user_profile").Where(relica.Eq("id", 1)).One(&x)
		require.ErrorContains(t, err, `field Missing expects column "missing"`)

		require.NoError(t, db.Select().From("nm_user_profile").Where(relica.Eq("id", 1)).ScanMode(relica.ScanLenient).One(&x))
		assert.Equal(t, "Ann B", x.DisplayName)

		var listed nmWithExtra
		require.NoError(t, db.Select("id", "display_name").From("nm_user_profile").Where(relica.Eq("id", 1)).One(&listed))
		assert.Equal(t, 1, listed.ID)

		var many []nmWithExtra
		err = db.NewQuery("SELECT * FROM nm_user_profile").All(&many)
		require.ErrorContains(t, err, `expects column "missing"`)
	})

	t.Run("WrapDB with options: naming, strict scan, hook", func(t *testing.T) {
		var events atomic.Int64
		wrapped := relica.WrapDB(db.SqlDB(), dialect,
			relica.WithNaming(relica.Snake(relica.Initialisms(relica.GoInitialisms()...))),
			relica.WithScanMode(relica.ScanStrict),
			relica.WithQueryHook(func(context.Context, relica.QueryEvent) { events.Add(1) }))

		require.NoError(t, wrapped.Model(&nmHTTP{ID: 1, HTTPStatus: 204}).Insert())
		var got nmHTTP
		require.NoError(t, wrapped.Select().From("nm_http").Where(relica.Eq("id", 1)).One(&got))
		assert.Equal(t, 204, got.HTTPStatus)

		var short struct {
			ID int `db:"id"`
		}
		err := wrapped.Select().From("nm_http").Where(relica.Eq("id", 1)).One(&short)
		require.ErrorContains(t, err, `result column "http_status" matches no field`)
		assert.GreaterOrEqual(t, events.Load(), int64(3), "FEAT-013: hooks fire on a wrapped *sql.DB")
	})
}

func TestNamingV020_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runNamingV020Tests(t, ds.DB, "postgres")
}

func TestNamingV020_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runNamingV020Tests(t, ds.DB, "mysql")
}

func TestNamingV020_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", filepath.Join(t.TempDir(), "naming.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	runNamingV020Tests(t, db, "sqlite")
}
