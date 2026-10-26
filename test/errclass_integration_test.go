//go:build integration

package test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coregx/relica"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Error classification on real servers and real drivers (ADR-017): every class
// is provoked for real, and each error must match exactly one helper. The
// PostgreSQL suite runs twice, through lib/pq and through pgx.

var errClassifiers = []struct {
	name string
	fn   func(error) bool
}{
	{"IsUniqueViolation", relica.IsUniqueViolation},
	{"IsForeignKeyViolation", relica.IsForeignKeyViolation},
	{"IsNotNullViolation", relica.IsNotNullViolation},
	{"IsCheckViolation", relica.IsCheckViolation},
	{"IsDeadlock", relica.IsDeadlock},
	{"IsSerializationFailure", relica.IsSerializationFailure},
	{"IsLockTimeout", relica.IsLockTimeout},
}

// assertClassifiedAs checks that err matches the helper named want and no other.
func assertClassifiedAs(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t, err, "expected a %s error", want)
	for _, c := range errClassifiers {
		assert.Equalf(t, c.name == want, c.fn(err), "%s(%T: %v)", c.name, rootCause(err), err)
	}
}

// rootCause returns the innermost error of a %w chain, for messages.
func rootCause(err error) error {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			return err
		}
		err = next
	}
}

func setupErrClassTables(t *testing.T, db *relica.DB) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		"DROP TABLE IF EXISTS ec_child",
		"DROP TABLE IF EXISTS ec_parent",
		"DROP TABLE IF EXISTS ec_lock",
		"CREATE TABLE ec_parent (id INT PRIMARY KEY)",
		// Table-level FOREIGN KEY: InnoDB ignores a column-level REFERENCES.
		`CREATE TABLE ec_child (
			id INT PRIMARY KEY,
			parent_id INT NOT NULL,
			email VARCHAR(100) NOT NULL,
			qty INT NOT NULL,
			CONSTRAINT ec_child_email_uq UNIQUE (email),
			CONSTRAINT ec_child_parent_fk FOREIGN KEY (parent_id) REFERENCES ec_parent (id),
			CONSTRAINT ec_child_qty_ck CHECK (qty > 0)
		)`,
		"CREATE TABLE ec_lock (id INT PRIMARY KEY, n INT NOT NULL)",
		"INSERT INTO ec_parent (id) VALUES (1)",
		"INSERT INTO ec_child (id, parent_id, email, qty) VALUES (1, 1, 'a@example.com', 1)",
		"INSERT INTO ec_lock (id, n) VALUES (1, 0), (2, 0)",
	} {
		_, err := db.ExecContext(ctx, q)
		require.NoError(t, err, q)
	}
}

func insertChild(db relica.Executor, id int, parentID, email, qty any) error {
	_, err := db.Insert("ec_child", map[string]any{
		"id": id, "parent_id": parentID, "email": email, "qty": qty,
	}).Execute()
	return err
}

func bumpLockRow(tx *relica.Tx, id int) error {
	_, err := tx.Update("ec_lock").
		Set(map[string]any{"n": relica.NewExp("n + 1")}).
		Where(relica.Eq("id", id)).
		Execute()
	return err
}

// testCtx bounds every lock scenario, so a missed deadlock fails the test
// instead of hanging the CI job.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func runConstraintClassTests(t *testing.T, db *relica.DB) {
	t.Run("Unique", func(t *testing.T) {
		assertClassifiedAs(t, insertChild(db, 2, 1, "a@example.com", 1), "IsUniqueViolation")
	})
	t.Run("PrimaryKey", func(t *testing.T) {
		assertClassifiedAs(t, insertChild(db, 1, 1, "pk@example.com", 1), "IsUniqueViolation")
	})
	t.Run("ForeignKey", func(t *testing.T) {
		assertClassifiedAs(t, insertChild(db, 2, 99, "fk@example.com", 1), "IsForeignKeyViolation")
	})
	t.Run("ForeignKey_DeleteReferencedParent", func(t *testing.T) {
		_, err := db.Delete("ec_parent").Where(relica.Eq("id", 1)).Execute()
		assertClassifiedAs(t, err, "IsForeignKeyViolation")
	})
	t.Run("NotNull", func(t *testing.T) {
		assertClassifiedAs(t, insertChild(db, 2, 1, nil, 1), "IsNotNullViolation")
	})
	t.Run("NotNull_MultiRowInsert", func(t *testing.T) {
		_, err := db.NewQuery("INSERT INTO ec_child (id, parent_id, email, qty) VALUES " +
			"(10, 1, 'm1@example.com', 1), (11, 1, NULL, 1)").Execute()
		assertClassifiedAs(t, err, "IsNotNullViolation")
	})
	t.Run("Check", func(t *testing.T) {
		assertClassifiedAs(t, insertChild(db, 2, 1, "ck@example.com", 0), "IsCheckViolation")
	})
	t.Run("ThroughTransactional", func(t *testing.T) {
		err := db.Transactional(testCtx(t), func(tx *relica.Tx) error {
			return insertChild(tx, 2, 1, "a@example.com", 1)
		})
		assertClassifiedAs(t, err, "IsUniqueViolation")
	})
	t.Run("ThroughModelAPI", func(t *testing.T) {
		type ecChild struct {
			ID       int    `db:"id"`
			ParentID int    `db:"parent_id"`
			Email    string `db:"email"`
			Qty      int    `db:"qty"`
		}
		err := db.Model(&ecChild{ID: 3, ParentID: 1, Email: "a@example.com", Qty: 1}).Table("ec_child").Insert()
		assertClassifiedAs(t, err, "IsUniqueViolation")
	})
	t.Run("NoErrorIsNoClass", func(t *testing.T) {
		require.NoError(t, insertChild(db, 20, 1, "ok@example.com", 1))
	})
}

// runDeadlockTest makes two transactions lock two rows in opposite order. The
// cycle closes whichever statement reaches the server second, so no timing is
// needed: the server picks one victim, the other transaction proceeds.
func runDeadlockTest(t *testing.T, db *relica.DB) {
	ctx := testCtx(t)
	tx1, err := db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx1.Rollback() }()
	tx2, err := db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx2.Rollback() }()

	require.NoError(t, bumpLockRow(tx1, 1))
	require.NoError(t, bumpLockRow(tx2, 2))

	// The victim rolls back at once so the survivor is never left waiting.
	errCh := make(chan error, 1)
	go func() {
		err := bumpLockRow(tx1, 2)
		if err != nil {
			_ = tx1.Rollback()
		}
		errCh <- err
	}()
	err2 := bumpLockRow(tx2, 1)
	if err2 != nil {
		_ = tx2.Rollback()
	}
	err1 := <-errCh

	require.True(t, (err1 == nil) != (err2 == nil),
		"exactly one transaction must be the victim: tx1=%v tx2=%v", err1, err2)
	victim := err1
	if victim == nil {
		victim = err2
	}
	assertClassifiedAs(t, victim, "IsDeadlock")
}

// runSerializationTest: a REPEATABLE READ transaction updates a row another
// transaction changed after its snapshot (PostgreSQL SQLSTATE 40001).
func runSerializationTest(t *testing.T, db *relica.DB) {
	ctx := testCtx(t)
	tx, err := db.BeginTx(ctx, &relica.TxOptions{Isolation: sql.LevelRepeatableRead})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	var n int
	require.NoError(t, tx.NewQuery("SELECT n FROM ec_lock WHERE id = 1").Row(&n)) // takes the snapshot
	_, err = db.Update("ec_lock").Set(map[string]any{"n": relica.NewExp("n + 1")}).
		Where(relica.Eq("id", 1)).Execute()
	require.NoError(t, err)

	assertClassifiedAs(t, bumpLockRow(tx, 1), "IsSerializationFailure")
}

// runLockTimeoutTest: tx1 holds a row lock; tx2 waits for it with a short
// timeout set by setTimeout (inside tx2, so it stays on tx2's connection).
func runLockTimeoutTest(t *testing.T, db *relica.DB, setTimeout, resetTimeout string) {
	ctx := testCtx(t)
	tx1, err := db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx1.Rollback() }()
	require.NoError(t, bumpLockRow(tx1, 1))

	tx2, err := db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx2.Rollback() }()
	_, err = tx2.NewQuery(setTimeout).Execute()
	require.NoError(t, err)

	assertClassifiedAs(t, bumpLockRow(tx2, 1), "IsLockTimeout")

	if resetTimeout != "" {
		// MySQL: the timeout is a session variable and the connection returns to
		// the pool; a lock wait timeout rolls back only the statement, so tx2
		// is still usable here.
		_, err = tx2.NewQuery(resetTimeout).Execute()
		require.NoError(t, err)
	}
}

// runNoWaitTest: SELECT ... FOR UPDATE NOWAIT on a locked row fails at once.
func runNoWaitTest(t *testing.T, db *relica.DB) {
	ctx := testCtx(t)
	tx1, err := db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx1.Rollback() }()
	require.NoError(t, bumpLockRow(tx1, 1))

	tx2, err := db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx2.Rollback() }()
	var n int
	err = tx2.NewQuery("SELECT n FROM ec_lock WHERE id = 1 FOR UPDATE NOWAIT").Row(&n)
	assertClassifiedAs(t, err, "IsLockTimeout")
}

// runLocalizedPostgresTest: with a non-English server the message text is
// useless, the code still classifies. lc_messages takes effect only if the
// server has the translations and the OS locale; when it does not, the same
// property is checked on the real driver error with its message replaced.
func runLocalizedPostgresTest(t *testing.T, db *relica.DB) {
	ctx := testCtx(t)
	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	localized := false
	for _, loc := range []string{"de_DE.UTF-8", "de_DE.utf8", "de_DE"} {
		// SET LOCAL lc_messages needs superuser: the container user is one.
		if _, err := tx.NewQuery("SET LOCAL lc_messages TO '" + loc + "'").Execute(); err == nil {
			localized = true
			break
		}
		_ = tx.Rollback()
		tx, err = db.Begin(ctx)
		require.NoError(t, err)
	}

	err = insertChild(tx, 2, 1, "a@example.com", 1)
	require.Error(t, err)
	if localized && !strings.Contains(err.Error(), "duplicate key") {
		t.Logf("server message is localized: %v", err)
		assertClassifiedAs(t, err, "IsUniqueViolation")
	} else {
		t.Logf("server has no German messages (%v); checking the real driver error with a German message", err)
	}

	german := "doppelter Schlüsselwert verletzt Unique-Constraint »ec_child_email_uq«"
	var rewritten error
	var pqErr *pq.Error
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pqErr):
		e := *pqErr
		e.Message = german
		rewritten = fmt.Errorf("insert: %w", &e)
	case errors.As(err, &pgErr):
		e := *pgErr
		e.Message = german
		rewritten = fmt.Errorf("insert: %w", &e)
	default:
		t.Fatalf("unexpected driver error type %T", rootCause(err))
	}
	require.NotContains(t, rewritten.Error(), "duplicate key")
	assertClassifiedAs(t, rewritten, "IsUniqueViolation")
}

func runPostgresErrClassTests(t *testing.T, db *relica.DB) {
	setupErrClassTables(t, db)
	runConstraintClassTests(t, db)
	t.Run("Deadlock", func(t *testing.T) { runDeadlockTest(t, db) })
	t.Run("SerializationFailure", func(t *testing.T) { runSerializationTest(t, db) })
	t.Run("LockTimeout", func(t *testing.T) {
		runLockTimeoutTest(t, db, "SET LOCAL lock_timeout = '200ms'", "")
	})
	t.Run("LockNoWait", func(t *testing.T) { runNoWaitTest(t, db) })
	t.Run("LocalizedServer", func(t *testing.T) { runLocalizedPostgresTest(t, db) })
}

// ---------------------------------------------------------------------------
// Entry points — one per database; CI selects the database with RELICA_TEST_DB.
// ---------------------------------------------------------------------------

// TestErrorClassification_DriverTypes pins the binding to the
// real driver types without a server. Messages are "x", so only the code can
// match. Needs no database, so it runs in every job.
func TestErrorClassification_DriverTypes(t *testing.T) {
	sqlState := func(s string) [5]byte {
		var b [5]byte
		copy(b[:], s)
		return b
	}
	assertClassifiedAs(t, fmt.Errorf("q: %w", &mysql.MySQLError{Number: 1213, Message: "x"}), "IsDeadlock")
	assertClassifiedAs(t, &mysql.MySQLError{Number: 3572, Message: "x"}, "IsLockTimeout")
	assertClassifiedAs(t, &mysql.MySQLError{Number: 4025, SQLState: sqlState("23000"), Message: "x"}, "IsCheckViolation")
	assert.False(t, relica.IsCheckViolation(&mysql.MySQLError{Number: 4025, SQLState: sqlState("HY000"), Message: "x"}),
		"4025 is MariaDB's CHECK error only with SQLSTATE 23000")
	assertClassifiedAs(t, &pq.Error{Code: "40P01", Message: "x"}, "IsDeadlock")
	assertClassifiedAs(t, errors.Join(errors.New("work"), &pgconn.PgError{Code: "55P03", Message: "x"}), "IsLockTimeout")
}

func TestErrorClassification_PostgreSQL_LibPQ(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runPostgresErrClassTests(t, ds.DB)
}

func TestErrorClassification_PostgreSQL_Pgx(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	db, err := relica.Open("pgx", ds.DSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Guard: the suite must really run through pgx.
	err = insertChild(db, 1, 1, "x@example.com", 1) // tables do not exist yet
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "pgx must return *pgconn.PgError")

	runPostgresErrClassTests(t, db)
}

func TestErrorClassification_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	db := ds.DB
	setupErrClassTables(t, db)

	err := insertChild(db, 1, 1, "a@example.com", 1)
	var myErr *mysql.MySQLError
	require.ErrorAs(t, err, &myErr, "go-sql-driver must return *mysql.MySQLError")

	runConstraintClassTests(t, db)
	t.Run("Deadlock", func(t *testing.T) { runDeadlockTest(t, db) })
	t.Run("LockTimeout", func(t *testing.T) {
		runLockTimeoutTest(t, db,
			"SET SESSION innodb_lock_wait_timeout = 1", "SET SESSION innodb_lock_wait_timeout = DEFAULT")
	})
	t.Run("LockNoWait", func(t *testing.T) { runNoWaitTest(t, db) })
}

func TestErrorClassification_SQLite(t *testing.T) {
	requireDB(t, "sqlite")
	// A file database in WAL mode: the lock scenarios need two connections.
	dsn := filepath.Join(t.TempDir(), "errclass.db") +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(200)"
	db, err := relica.Open("sqlite", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	setupErrClassTables(t, db)

	runConstraintClassTests(t, db)

	t.Run("Deadlock_NotDetectedBySQLite", func(t *testing.T) {
		t.Skip("SQLite has a single writer and no deadlock detection; IsDeadlock never matches")
	})

	// A read transaction whose snapshot went stale tries to write:
	// SQLITE_BUSY_SNAPSHOT, which SQLite returns without waiting.
	t.Run("SerializationFailure_BusySnapshot", func(t *testing.T) {
		ctx := testCtx(t)
		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		var n int
		require.NoError(t, tx.NewQuery("SELECT n FROM ec_lock WHERE id = 1").Row(&n))
		_, err = db.Update("ec_lock").Set(map[string]any{"n": relica.NewExp("n + 1")}).
			Where(relica.Eq("id", 2)).Execute()
		require.NoError(t, err)
		assertClassifiedAs(t, bumpLockRow(tx, 1), "IsSerializationFailure")
	})

	// tx1 holds the write lock; tx2's write waits busy_timeout (200 ms) and
	// gets SQLITE_BUSY.
	t.Run("LockTimeout_Busy", func(t *testing.T) {
		ctx := testCtx(t)
		tx1, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx1.Rollback() }()
		require.NoError(t, bumpLockRow(tx1, 1))

		tx2, err := db.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx2.Rollback() }()
		start := time.Now()
		err = bumpLockRow(tx2, 2)
		assertClassifiedAs(t, err, "IsLockTimeout")
		assert.GreaterOrEqual(t, time.Since(start), 150*time.Millisecond, "busy_timeout was not honored")
	})
}
