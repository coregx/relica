//go:build integration
// +build integration

package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coregx/relica"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	_ "modernc.org/sqlite" // Pure Go SQLite driver (no CGO required)
)

// selectedDB is the database a run is limited to: RELICA_TEST_DB=postgres,
// mysql or sqlite (one CI job per database). Empty runs every database.
//
// Jobs select databases this way, not with -run name filters: a filter on
// test names misses tests whose names do not spell the database the same way
// and every multi-database test (databases are subtests there).
var selectedDB = os.Getenv("RELICA_TEST_DB")

// requireDB skips the test unless dialect ("postgres", "mysql", "sqlite") is
// selected for this run.
func requireDB(t *testing.T, dialect string) {
	t.Helper()
	switch selectedDB {
	case "", dialect:
	case "postgres", "mysql", "sqlite":
		t.Skipf("RELICA_TEST_DB=%s", selectedDB)
	default:
		t.Fatalf("RELICA_TEST_DB=%q: want postgres, mysql or sqlite", selectedDB)
	}
}

// containerUnavailable skips the test when a database container cannot start
// locally (no Docker), but fails it when the run is selected for that
// database: in CI a silent skip would report untested code as passing.
func containerUnavailable(t *testing.T, dialect string, err error) {
	t.Helper()
	if selectedDB == dialect {
		t.Fatalf("RELICA_TEST_DB=%s but the %s container did not start: %v", dialect, dialect, err)
	}
	t.Skipf("Docker not available for %s integration tests: %v", dialect, err)
}

// containerStartupTimeout bounds the wait for a database container to accept
// connections. A cold MySQL 8.0 start (first initialisation after a Docker
// restart) took 63 s on a developer machine, past the old 60 s limit. It is
// passed both to the log strategy and as the overall deadline:
// testcontainers.WithWaitStrategy caps every wait at 60 s on its own.
const containerStartupTimeout = 3 * time.Minute

// terminateFailedContainer stops a container whose start failed: a module's
// Run returns the container together with the error, and with the Ryuk reaper
// disabled (TESTCONTAINERS_RYUK_DISABLED=true) nothing else would remove it.
func terminateFailedContainer(t *testing.T, c testcontainers.Container) {
	t.Helper()
	if err := testcontainers.TerminateContainer(c); err != nil {
		t.Logf("terminate failed container: %v", err)
	}
}

// startContainer runs start and turns a panic into an error: testcontainers
// panics instead of returning an error when it finds no usable Docker host
// (for example "rootless Docker is not supported on Windows").
func startContainer[C any](start func() (C, error)) (c C, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("testcontainers: %v", p)
		}
	}()
	return start()
}

// DatabaseSetup encapsulates database connection and cleanup.
type DatabaseSetup struct {
	DB        *relica.DB
	Container testcontainers.Container
	Dialect   string
	// DSN reopens the same database with options (hooks, loggers); empty for
	// in-memory SQLite.
	DSN string
}

// Close cleans up database resources.
func (ds *DatabaseSetup) Close() {
	if ds.DB != nil {
		ds.DB.Close() //nolint:errcheck
	}
	if ds.Container != nil {
		ds.Container.Terminate(context.Background()) //nolint:errcheck
	}
}

// SetupPostgreSQLTestDB creates a PostgreSQL test database.
// Uses testcontainers if available, falls back to env DSN.
func SetupPostgreSQLTestDB(t *testing.T) *DatabaseSetup {
	requireDB(t, "postgres")
	ctx := context.Background()

	// Check for manual DSN first (allows testing without Docker)
	if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
		db, err := relica.Open("postgres", dsn)
		require.NoError(t, err)
		return &DatabaseSetup{DB: db, Dialect: "postgres", DSN: dsn}
	}

	// Start PostgreSQL in Docker via testcontainers
	pgContainer, err := startContainer(func() (*postgres.PostgresContainer, error) {
		return postgres.Run(
			ctx,
			"postgres:15-alpine",
			postgres.WithDatabase("testdb"),
			postgres.WithUsername("user"),
			postgres.WithPassword("password"),
			testcontainers.WithWaitStrategyAndDeadline(containerStartupTimeout,
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(containerStartupTimeout),
			),
		)
	})
	if err != nil {
		terminateFailedContainer(t, pgContainer)
		containerUnavailable(t, "postgres", err)
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := relica.Open("postgres", dsn)
	require.NoError(t, err)

	return &DatabaseSetup{
		DB:        db,
		Container: pgContainer,
		Dialect:   "postgres",
		DSN:       dsn,
	}
}

// SetupMySQLTestDB creates a MySQL test database.
// Uses testcontainers if available, falls back to env DSN.
func SetupMySQLTestDB(t *testing.T) *DatabaseSetup {
	requireDB(t, "mysql")
	ctx := context.Background()

	// Check for manual DSN first
	if dsn := os.Getenv("MYSQL_TEST_DSN"); dsn != "" {
		// Ensure parseTime=true is set for time.Time support
		if !strings.Contains(dsn, "parseTime=true") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		db, err := relica.Open("mysql", dsn)
		require.NoError(t, err)
		return &DatabaseSetup{DB: db, Dialect: "mysql", DSN: dsn}
	}

	// Start MySQL in Docker via testcontainers
	mysqlContainer, err := startContainer(func() (*mysql.MySQLContainer, error) {
		return mysql.Run(
			ctx,
			"mysql:8.0",
			mysql.WithDatabase("testdb"),
			mysql.WithUsername("user"),
			mysql.WithPassword("password"),
			testcontainers.WithWaitStrategyAndDeadline(containerStartupTimeout,
				wait.ForLog("port: 3306  MySQL Community Server").
					WithStartupTimeout(containerStartupTimeout),
			),
		)
	})
	if err != nil {
		terminateFailedContainer(t, mysqlContainer)
		containerUnavailable(t, "mysql", err)
	}

	dsn, err := mysqlContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Add parseTime=true to enable time.Time parsing for DATETIME/TIMESTAMP columns
	// Without this, MySQL driver returns []uint8 instead of time.Time
	// See: https://github.com/go-sql-driver/mysql#parsetime
	dsn += "?parseTime=true"

	db, err := relica.Open("mysql", dsn)
	require.NoError(t, err)

	return &DatabaseSetup{
		DB:        db,
		Container: mysqlContainer,
		Dialect:   "mysql",
		DSN:       dsn,
	}
}

// SetupSQLiteTestDB creates an in-memory SQLite database.
// Always works, no external dependencies.
func SetupSQLiteTestDB(t *testing.T) *DatabaseSetup {
	requireDB(t, "sqlite")
	db, err := relica.Open("sqlite", ":memory:")
	require.NoError(t, err)

	return &DatabaseSetup{
		DB:      db,
		Dialect: "sqlite",
	}
}

// CreateMessagesTable creates the messages table for IrisMX use case tests.
func CreateMessagesTable(t *testing.T, db *relica.DB, dialect string) {
	var createSQL string

	switch dialect {
	case "postgres":
		createSQL = `
			CREATE TABLE IF NOT EXISTS messages (
				id SERIAL PRIMARY KEY,
				mailbox_id INTEGER NOT NULL,
				user_id INTEGER NOT NULL,
				uid INTEGER NOT NULL,
				status INTEGER DEFAULT 1,
				size INTEGER DEFAULT 0,
				subject TEXT,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)
		`
	case "mysql":
		createSQL = `
			CREATE TABLE IF NOT EXISTS messages (
				id INT AUTO_INCREMENT PRIMARY KEY,
				mailbox_id INT NOT NULL,
				user_id INT NOT NULL,
				uid INT NOT NULL,
				status INT DEFAULT 1,
				size INT DEFAULT 0,
				subject TEXT,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)
		`
	case "sqlite":
		createSQL = `
			CREATE TABLE IF NOT EXISTS messages (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				mailbox_id INTEGER NOT NULL,
				user_id INTEGER NOT NULL,
				uid INTEGER NOT NULL,
				status INTEGER DEFAULT 1,
				size INTEGER DEFAULT 0,
				subject TEXT,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)
		`
	}

	_, err := db.ExecContext(context.Background(), createSQL)
	require.NoError(t, err)
}

// CreateAttachmentsTable creates the attachments table.
func CreateAttachmentsTable(t *testing.T, db *relica.DB, dialect string) {
	var createSQL string

	switch dialect {
	case "postgres":
		createSQL = `
			CREATE TABLE IF NOT EXISTS attachments (
				id SERIAL PRIMARY KEY,
				message_id INTEGER NOT NULL,
				filename VARCHAR(255),
				size INTEGER DEFAULT 0
			)
		`
	case "mysql":
		createSQL = `
			CREATE TABLE IF NOT EXISTS attachments (
				id INT AUTO_INCREMENT PRIMARY KEY,
				message_id INT NOT NULL,
				filename VARCHAR(255),
				size INT DEFAULT 0
			)
		`
	case "sqlite":
		createSQL = `
			CREATE TABLE IF NOT EXISTS attachments (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				message_id INTEGER NOT NULL,
				filename VARCHAR(255),
				size INTEGER DEFAULT 0
			)
		`
	}

	_, err := db.ExecContext(context.Background(), createSQL)
	require.NoError(t, err)
}

// CreateUsersTable creates a users table for multi-table JOIN tests.
func CreateUsersTable(t *testing.T, db *relica.DB, dialect string) {
	var createSQL string

	switch dialect {
	case "postgres":
		createSQL = `
			CREATE TABLE IF NOT EXISTS users (
				id SERIAL PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				email VARCHAR(255) UNIQUE NOT NULL,
				age INTEGER,
				status INTEGER DEFAULT 1,
				role VARCHAR(50),
				deleted_at VARCHAR(50)
			)
		`
	case "mysql":
		createSQL = `
			CREATE TABLE IF NOT EXISTS users (
				id INT AUTO_INCREMENT PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				email VARCHAR(255) UNIQUE NOT NULL,
				age INT,
				status INT DEFAULT 1,
				role VARCHAR(50),
				deleted_at VARCHAR(50)
			)
		`
	case "sqlite":
		createSQL = `
			CREATE TABLE IF NOT EXISTS users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name VARCHAR(255) NOT NULL,
				email VARCHAR(255) UNIQUE NOT NULL,
				age INTEGER,
				status INTEGER DEFAULT 1,
				role VARCHAR(50),
				deleted_at VARCHAR(50)
			)
		`
	}

	_, err := db.ExecContext(context.Background(), createSQL)
	require.NoError(t, err)
}

// CreatePostsTable creates a posts table for multi-JOIN tests.
func CreatePostsTable(t *testing.T, db *relica.DB, dialect string) {
	var createSQL string

	switch dialect {
	case "postgres":
		createSQL = `
			CREATE TABLE IF NOT EXISTS posts (
				id SERIAL PRIMARY KEY,
				user_id INTEGER NOT NULL,
				title VARCHAR(255),
				content TEXT
			)
		`
	case "mysql":
		createSQL = `
			CREATE TABLE IF NOT EXISTS posts (
				id INT AUTO_INCREMENT PRIMARY KEY,
				user_id INT NOT NULL,
				title VARCHAR(255),
				content TEXT
			)
		`
	case "sqlite":
		createSQL = `
			CREATE TABLE IF NOT EXISTS posts (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				title VARCHAR(255),
				content TEXT
			)
		`
	}

	_, err := db.ExecContext(context.Background(), createSQL)
	require.NoError(t, err)
}

// InsertTestMessages inserts test messages into the database.
func InsertTestMessages(t *testing.T, db *relica.DB, count, mailboxID int) {
	for i := 1; i <= count; i++ {
		_, err := db.Builder().Insert("messages", map[string]interface{}{
			"mailbox_id": mailboxID,
			"user_id":    i % 100, // Distribute across 100 users
			"uid":        i,
			"status":     1,
			"size":       1024 * (i % 10), // 0KB to 9KB
			"subject":    fmt.Sprintf("Test Message %d", i),
		}).Execute()
		require.NoError(t, err)
	}
}

// InsertTestAttachments inserts test attachments for messages.
func InsertTestAttachments(t *testing.T, db *relica.DB, messageCount, attachmentsPerMessage int) {
	for msgID := 1; msgID <= messageCount; msgID++ {
		for i := 0; i < attachmentsPerMessage; i++ {
			_, err := db.Builder().Insert("attachments", map[string]interface{}{
				"message_id": msgID,
				"filename":   fmt.Sprintf("file%d.pdf", i),
				"size":       1024 * (i + 1), // 1KB, 2KB, 3KB, etc.
			}).Execute()
			require.NoError(t, err)
		}
	}
}

// InsertTestUsers inserts test users.
func InsertTestUsers(t *testing.T, db *relica.DB, count int) {
	for i := 1; i <= count; i++ {
		_, err := db.Builder().Insert("users", map[string]interface{}{
			"name":   fmt.Sprintf("User%d", i),
			"email":  fmt.Sprintf("user%d@example.com", i),
			"age":    20 + (i % 50), // Ages 20-70
			"status": 1,
			"role":   "user",
		}).Execute()
		require.NoError(t, err)
	}
}

// InsertTestPosts inserts test posts.
func InsertTestPosts(t *testing.T, db *relica.DB, userID, count int) {
	for i := 1; i <= count; i++ {
		_, err := db.Builder().Insert("posts", map[string]interface{}{
			"user_id": userID,
			"title":   fmt.Sprintf("Post %d by User %d", i, userID),
			"content": fmt.Sprintf("Content of post %d", i),
		}).Execute()
		require.NoError(t, err)
	}
}

// LoadTestData reads test/testdata/{dialect}.sql and executes each statement.
//
// The SQL file is split on ";" boundaries. Empty statements and lines that
// consist solely of SQL comments are skipped. This handles the full schema
// creation and seed data insertion in a single call.
//
// Example:
//
//	ds := SetupSQLiteTestDB(t)
//	defer ds.Close()
//	LoadTestData(t, ds.DB, ds.Dialect)
func LoadTestData(t *testing.T, db *relica.DB, dialect string) {
	t.Helper()

	// Resolve path relative to this file's directory so the test works
	// regardless of the working directory from which `go test` is invoked.
	dir, err := filepath.Abs("testdata")
	require.NoError(t, err, "resolving testdata directory")

	sqlFile := filepath.Join(dir, dialect+".sql")
	raw, err := os.ReadFile(sqlFile)
	require.NoError(t, err, "reading testdata file %s", sqlFile)

	ctx := context.Background()
	stmts := strings.Split(string(raw), ";")
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// Skip comment-only blocks (lines starting with "--").
		isComment := true
		for _, line := range strings.Split(stmt, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				isComment = false
				break
			}
		}
		if isComment {
			continue
		}
		_, execErr := db.ExecContext(ctx, stmt)
		require.NoError(t, execErr, "executing SQL statement:\n%s", stmt)
	}
}
