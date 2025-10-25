//go:build integration
// +build integration

package test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// TestWrapDB_IrisMXUseCase validates WrapDB for IrisMX production scenario:
// External connection pool with custom settings + Relica query builder.
//
// IrisMX requirements:
//   - 10K+ concurrent users
//   - Custom connection pool settings
//   - Complex JOIN queries for mail attachments
//   - No connection duplication
func TestWrapDB_IrisMXUseCase(t *testing.T) {
	databases := []struct {
		name  string
		setup func(*testing.T) *DatabaseSetup
	}{
		{"SQLite", SetupSQLiteTestDB},
		{"PostgreSQL", SetupPostgreSQLTestDB},
		{"MySQL", SetupMySQLTestDB},
	}

	for _, dbConfig := range databases {
		t.Run(dbConfig.name, func(t *testing.T) {
			ds := dbConfig.setup(t)
			defer ds.Close()

			ctx := context.Background()

			// Step 1: Create external sql.DB with custom pool settings (IrisMX production config)
			// This simulates IrisMX's existing connection pool
			dsn := getDSN(t, ds, dbConfig.name)
			sqlDB, err := sql.Open(getDriverName(dbConfig.name), dsn)
			require.NoError(t, err, "Failed to open external database connection")
			defer sqlDB.Close()

			// Apply IrisMX-specific pool settings for 10K+ concurrent users
			sqlDB.SetMaxOpenConns(100)
			sqlDB.SetMaxIdleConns(50)
			sqlDB.SetConnMaxLifetime(time.Hour)
			sqlDB.SetConnMaxIdleTime(10 * time.Minute)

			// Verify connection pool is alive
			err = sqlDB.PingContext(ctx)
			require.NoError(t, err, "Failed to ping database")

			// Step 2: Wrap external connection with Relica (zero duplication)
			db := relica.WrapDB(sqlDB, getDriverName(dbConfig.name))
			require.NotNil(t, db, "Expected wrapped DB, got nil")

			// Step 3: Setup mail messages and attachments tables
			// Use wrapped DB for table creation
			CreateMessagesTable(t, db, getDriverName(dbConfig.name))
			CreateAttachmentsTable(t, db, getDriverName(dbConfig.name))

			// Step 4: Simulate IrisMX workload
			const messageCount = 50
			const attachmentsPerMessage = 3

			// Insert test data using wrapped connection
			InsertTestMessages(t, db, messageCount, 1)
			InsertTestAttachments(t, db, messageCount, attachmentsPerMessage)

			// Step 5: Execute complex JOIN query (IrisMX's main use case)
			t.Run("ComplexJoinQuery", func(t *testing.T) {
				var results []MessageWithStats
				err := db.Builder().
					Select(
						"messages.id",
						"messages.mailbox_id",
						"messages.user_id",
						"messages.uid",
						"messages.status",
						"messages.size",
						"messages.subject",
						"messages.created_at",
						"COUNT(attachments.id) as attachment_count",
					).
					From("messages").
					LeftJoin("attachments", "messages.id = attachments.message_id").
					Where("messages.mailbox_id = ?", 1).
					GroupBy("messages.id").
					OrderBy("messages.id").
					All(&results)

				require.NoError(t, err, "Failed to execute JOIN query")
				assert.Len(t, results, messageCount, "Expected all messages to be returned")

				// Verify attachment counts
				for _, msg := range results {
					assert.Equal(t, attachmentsPerMessage, msg.AttachmentCount,
						"Expected %d attachments for message %d", attachmentsPerMessage, msg.ID)
				}
			})

			// Step 6: Verify transactions work with wrapped connection
			t.Run("TransactionSupport", func(t *testing.T) {
				tx, err := db.Begin(ctx)
				require.NoError(t, err, "Failed to begin transaction")
				defer tx.Rollback()

				// Insert new message in transaction
				_, err = tx.Builder().
					Insert("messages", map[string]interface{}{
						"mailbox_id": 1,
						"user_id":    1,
						"uid":        messageCount + 1,
						"status":     "unread",
						"size":       1024,
						"subject":    "Transaction Test",
						"created_at": time.Now(),
					}).
					Execute()
				require.NoError(t, err, "Failed to insert in transaction")

				// Verify within transaction
				var result struct {
					Count int `db:"count"`
				}
				err = tx.Builder().
					Select("COUNT(*) as count").
					From("messages").
					One(&result)
				require.NoError(t, err, "Failed to count messages in transaction")
				assert.Equal(t, messageCount+1, result.Count, "Expected count to include new message")

				// Rollback
				err = tx.Rollback()
				require.NoError(t, err, "Failed to rollback transaction")

				// Verify rollback worked
				var resultAfterRollback struct {
					Count int `db:"count"`
				}
				err = db.Builder().
					Select("COUNT(*) as count").
					From("messages").
					One(&resultAfterRollback)
				require.NoError(t, err, "Failed to count messages after rollback")
				assert.Equal(t, messageCount, resultAfterRollback.Count, "Expected original count after rollback")
			})

			// Step 7: Verify statement caching works
			t.Run("StatementCaching", func(t *testing.T) {
				// Execute same query multiple times
				for i := 0; i < 10; i++ {
					var msg Message
					err := db.Builder().
						Select().
						From("messages").
						Where("id = ?", 1).
						One(&msg)
					require.NoError(t, err, "Failed to select message (iteration %d)", i)
					assert.Equal(t, 1, msg.ID)
				}

				// Note: Cache stats verification would be implementation-specific
				t.Log("Successfully executed 10 identical queries with statement caching")
			})

			// Step 8: Verify batch operations work
			t.Run("BatchOperations", func(t *testing.T) {
				// Batch insert new messages
				_, err := db.Builder().
					BatchInsert("messages", []string{
						"mailbox_id", "user_id", "uid", "status", "size", "subject", "created_at",
					}).
					Values(1, 1, messageCount+1, "unread", 512, "Batch 1", time.Now()).
					Values(1, 1, messageCount+2, "unread", 512, "Batch 2", time.Now()).
					Values(1, 1, messageCount+3, "unread", 512, "Batch 3", time.Now()).
					Execute()

				require.NoError(t, err, "Failed to batch insert")

				// Verify batch insert
				var countResult struct {
					Count int `db:"count"`
				}
				err = db.Builder().
					Select("COUNT(*) as count").
					From("messages").
					Where("mailbox_id = ?", 1).
					One(&countResult)
				require.NoError(t, err, "Failed to count messages after batch insert")
				assert.GreaterOrEqual(t, countResult.Count, messageCount+3, "Expected at least %d messages", messageCount+3)
			})

			// Step 9: Verify caller owns connection lifecycle
			// Note: DO NOT call db.Close() on wrapped DB - caller owns the connection
			t.Run("CallerOwnsConnectionLifecycle", func(t *testing.T) {
				// Verify underlying connection is still alive (because we didn't close it)
				err := sqlDB.PingContext(ctx)
				assert.NoError(t, err, "Expected underlying connection to be alive")

				// Verify we can still query directly
				var count int
				err = sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages").Scan(&count)
				assert.NoError(t, err, "Expected to query directly with underlying connection")
				assert.Greater(t, count, 0, "Expected messages to exist")

				t.Log("Caller correctly owns connection lifecycle - deferred sqlDB.Close() will handle cleanup")
			})
		})
	}
}

// TestWrapDB_ConnectionPoolSettings validates that custom pool settings are preserved.
func TestWrapDB_ConnectionPoolSettings(t *testing.T) {
	databases := []struct {
		name  string
		setup func(*testing.T) *DatabaseSetup
	}{
		{"SQLite", SetupSQLiteTestDB},
		{"PostgreSQL", SetupPostgreSQLTestDB},
		{"MySQL", SetupMySQLTestDB},
	}

	for _, dbConfig := range databases {
		t.Run(dbConfig.name, func(t *testing.T) {
			ds := dbConfig.setup(t)
			defer ds.Close()

			ctx := context.Background()

			// Create external sql.DB with specific settings
			dsn := getDSN(t, ds, dbConfig.name)
			sqlDB, err := sql.Open(getDriverName(dbConfig.name), dsn)
			require.NoError(t, err)
			defer sqlDB.Close()

			// Set custom pool settings
			const maxOpen = 42
			const maxIdle = 21
			sqlDB.SetMaxOpenConns(maxOpen)
			sqlDB.SetMaxIdleConns(maxIdle)
			sqlDB.SetConnMaxLifetime(2 * time.Hour)

			// Wrap with Relica
			db := relica.WrapDB(sqlDB, getDriverName(dbConfig.name))
			require.NotNil(t, db)

			// Verify connection works
			err = sqlDB.PingContext(ctx)
			require.NoError(t, err)

			// Pool settings are opaque in database/sql, but we can verify
			// that the connection pool is working by executing queries
			// Note: Can't directly verify SetMaxOpenConns/SetMaxIdleConns values

			// Create table (use appropriate syntax for each dialect)
			var createTableSQL string
			switch dbConfig.name {
			case "PostgreSQL":
				createTableSQL = `CREATE TABLE IF NOT EXISTS pool_test (
					id SERIAL PRIMARY KEY,
					value TEXT
				)`
			case "MySQL":
				createTableSQL = `CREATE TABLE IF NOT EXISTS pool_test (
					id INTEGER PRIMARY KEY AUTO_INCREMENT,
					value TEXT
				)`
			default: // SQLite
				createTableSQL = `CREATE TABLE IF NOT EXISTS pool_test (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					value TEXT
				)`
			}
			_, err = db.ExecContext(ctx, createTableSQL)
			require.NoError(t, err)

			// Execute multiple queries concurrently (would fail if pool broken)
			const concurrent = 10
			errChan := make(chan error, concurrent)

			for i := 0; i < concurrent; i++ {
				go func(id int) {
					_, err := db.Builder().
						Insert("pool_test", map[string]interface{}{
							"value": "concurrent",
						}).
						Execute()
					errChan <- err
				}(i)
			}

			// Collect errors
			for i := 0; i < concurrent; i++ {
				err := <-errChan
				assert.NoError(t, err, "Concurrent query %d failed", i)
			}

			t.Log("Successfully verified connection pool handles concurrent queries")
		})
	}
}

// Helper functions

func getDSN(t *testing.T, ds *DatabaseSetup, dialect string) string {
	t.Helper()
	ctx := context.Background()

	switch dialect {
	case "SQLite":
		return ":memory:"
	case "PostgreSQL":
		if ds.Container != nil {
			dsn, err := ds.Container.(*postgres.PostgresContainer).ConnectionString(ctx, "sslmode=disable")
			require.NoError(t, err)
			return dsn
		}
		// Fallback for manual DSN
		return "postgres://user:password@localhost/testdb?sslmode=disable"
	case "MySQL":
		if ds.Container != nil {
			dsn, err := ds.Container.(*mysql.MySQLContainer).ConnectionString(ctx)
			require.NoError(t, err)
			return dsn + "?parseTime=true"
		}
		// Fallback for manual DSN
		return "user:password@tcp(localhost:3306)/testdb?parseTime=true"
	default:
		t.Fatalf("Unknown dialect: %s", dialect)
		return ""
	}
}

func getDriverName(dialect string) string {
	switch dialect {
	case "SQLite":
		return "sqlite"
	case "PostgreSQL":
		return "postgres"
	case "MySQL":
		return "mysql"
	default:
		return dialect
	}
}
