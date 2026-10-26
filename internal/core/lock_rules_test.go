package core

import (
	"context"
	"strings"
	"testing"
)

// ADR-020 rules 3 and 4: a row lock outside a transaction, or combined with a
// set operation, GROUP BY, DISTINCT, Count or Exists, is a build error on every
// dialect; nothing is sent.

const lockOutsideTx = "require a transaction"
const lockCombination = "cannot be combined with"

func lockQueries(qb *QueryBuilder) map[string]*SelectQuery {
	return map[string]*SelectQuery{
		"ForUpdate":           qb.Select("id").From("t").ForUpdate(),
		"ForShare":            qb.Select("id").From("t").ForShare(),
		"ForUpdateSkipLocked": qb.Select("id").From("t").ForUpdateSkipLocked(),
	}
}

func TestLock_OutsideTransaction_IsBuildError(t *testing.T) {
	for _, d := range allDialects {
		for name, sq := range lockQueries(&QueryBuilder{db: mockDB(d)}) {
			t.Run(d+"/"+name, func(t *testing.T) {
				buildErrContains(t, sq.Build(), lockOutsideTx)
			})
		}
	}
}

func TestLock_InTransaction_RendersPerDialect(t *testing.T) {
	db := openCovDB(t) // memdb wrapped with the SQLite dialect
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	// SQLite: the clause is dropped inside a transaction (database-level locking).
	q := tx.Builder().Select("id").From("t").ForUpdate().Build()
	buildOK(t, q, `SELECT "id" FROM "t"`)

	// The same transaction state with the PostgreSQL dialect renders the clause.
	pgBuilder := &QueryBuilder{db: mockDB("postgres"), tx: tx.tx}
	buildOK(t, pgBuilder.Select("id").From("t").Where(Eq("id", 1)).ForUpdateSkipLocked().Build(),
		`SELECT "id" FROM "t" WHERE "id" = $1 FOR UPDATE SKIP LOCKED`)
	mysqlBuilder := &QueryBuilder{db: mockDB("mysql"), tx: tx.tx}
	buildOK(t, mysqlBuilder.Select("id").From("t").ForShare().Build(), "SELECT `id` FROM `t` FOR SHARE")
}

func TestLock_SQLiteDetectedByDialect(t *testing.T) {
	// FIX-012: SQLite was detected by driver name; a DB whose driver name is
	// not "sqlite"/"sqlite3" but whose dialect is SQLite rendered FOR UPDATE.
	db := openCovDB(t)
	db.driverName = "custom-sqlite-driver"
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	buildOK(t, tx.Builder().Select("id").From("t").ForUpdate().Build(), `SELECT "id" FROM "t"`)
}

func TestLock_InvalidCombinations_AreBuildErrors(t *testing.T) {
	db := openCovDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, d := range allDialects {
		qb := &QueryBuilder{db: mockDB(d), tx: tx.tx}
		cases := map[string]*SelectQuery{
			"UNION":         qb.Select("id").From("a").ForUpdate().Union(qb.Select("id").From("b")),
			"lock in UNION": qb.Select("id").From("a").Union(qb.Select("id").From("b").ForUpdate()),
			"INTERSECT":     qb.Select("id").From("a").ForShare().Intersect(qb.Select("id").From("b")),
			"EXCEPT":        qb.Select("id").From("a").ForUpdate().Except(qb.Select("id").From("b")),
			"GROUP BY":      qb.Select("kind").From("a").GroupBy("kind").ForUpdate(),
			"DISTINCT":      qb.Select("kind").From("a").Distinct().ForUpdate(),
		}
		for name, sq := range cases {
			t.Run(d+"/"+name, func(t *testing.T) {
				buildErrContains(t, sq.Build(), lockCombination)
			})
		}
	}
}

func TestLock_CountAndExists_AreErrors(t *testing.T) {
	db := openCovDB(t)
	seedCovTable(t, db)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	// Count and Exists used to drop the lock silently.
	if _, err := tx.Builder().Select().From("cov_items").ForUpdate().Count(); err == nil || !strings.Contains(err.Error(), lockCombination) {
		t.Fatalf("Count with a lock: err = %v", err)
	}
	if _, err := tx.Builder().Select().From("cov_items").ForUpdate().Exists(); err == nil || !strings.Contains(err.Error(), lockCombination) {
		t.Fatalf("Exists with a lock: err = %v", err)
	}
}
