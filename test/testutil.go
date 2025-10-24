//go:build integration
// +build integration

package test

import (
	"context"
	"fmt"
	"os"
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

// DatabaseSetup encapsulates database connection and cleanup.
type DatabaseSetup struct {
	DB        *relica.DB
	Container testcontainers.Container
	Dialect   string
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
	ctx := context.Background()

	// Check for manual DSN first (allows testing without Docker)
	if dsn := os.Getenv("POSTGRES_TEST_DSN"); dsn != "" {
		db, err := relica.NewDB("postgres", dsn)
		require.NoError(t, err)
		return &DatabaseSetup{DB: db, Dialect: "postgres"}
	}

	// Start PostgreSQL in Docker via testcontainers
	pgContainer, err := postgres.Run(
		ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skip("Docker not available for PostgreSQL integration tests: " + err.Error())
	}

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := relica.NewDB("postgres", dsn)
	require.NoError(t, err)

	return &DatabaseSetup{
		DB:        db,
		Container: pgContainer,
		Dialect:   "postgres",
	}
}

// SetupMySQLTestDB creates a MySQL test database.
// Uses testcontainers if available, falls back to env DSN.
func SetupMySQLTestDB(t *testing.T) *DatabaseSetup {
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
		db, err := relica.NewDB("mysql", dsn)
		require.NoError(t, err)
		return &DatabaseSetup{DB: db, Dialect: "mysql"}
	}

	// Start MySQL in Docker via testcontainers
	mysqlContainer, err := mysql.Run(
		ctx,
		"mysql:8.0",
		mysql.WithDatabase("testdb"),
		mysql.WithUsername("user"),
		mysql.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("port: 3306  MySQL Community Server").
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Skip("Docker not available for MySQL integration tests: " + err.Error())
	}

	dsn, err := mysqlContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Add parseTime=true to enable time.Time parsing for DATETIME/TIMESTAMP columns
	// Without this, MySQL driver returns []uint8 instead of time.Time
	// See: https://github.com/go-sql-driver/mysql#parsetime
	dsn += "?parseTime=true"

	db, err := relica.NewDB("mysql", dsn)
	require.NoError(t, err)

	return &DatabaseSetup{
		DB:        db,
		Container: mysqlContainer,
		Dialect:   "mysql",
	}
}

// SetupSQLiteTestDB creates an in-memory SQLite database.
// Always works, no external dependencies.
func SetupSQLiteTestDB(t *testing.T) *DatabaseSetup {
	db, err := relica.NewDB("sqlite", ":memory:")
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
				role VARCHAR(50)
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
				role VARCHAR(50)
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
				role VARCHAR(50)
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
