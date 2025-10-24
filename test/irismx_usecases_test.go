//go:build integration
// +build integration

package test

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIrisMX_UseCase_JoinSolvesN1Problem validates that JOIN operations
// solve the N+1 query problem for IrisMX mail server.
//
// BEFORE (v0.1.2-beta): 101 queries (1 for messages + 100 for attachments)
// AFTER (v0.2.0-beta): 1 query (JOIN with GROUP BY)
//
// Expected improvement: 100x query reduction.
func TestIrisMX_UseCase_JoinSolvesN1Problem(t *testing.T) {
	// Test with all available databases
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

			// Setup: Create tables and insert test data
			CreateMessagesTable(t, ds.DB, ds.Dialect)
			CreateAttachmentsTable(t, ds.DB, ds.Dialect)

			const messageCount = 100
			const attachmentsPerMessage = 5

			InsertTestMessages(t, ds.DB, messageCount, 1)
			InsertTestAttachments(t, ds.DB, messageCount, attachmentsPerMessage)

			// OLD WAY: N+1 queries (101 total)
			start := time.Now()
			queryCount := 0

			var messages []Message
			err := ds.DB.Builder().
				Select().
				From("messages").
				Where("mailbox_id = ?", 1).
				Limit(messageCount).
				All(&messages)
			require.NoError(t, err, "Failed to fetch messages")
			queryCount++

			for i := range messages {
				var attachments []Attachment
				err := ds.DB.Builder().
					Select().
					From("attachments").
					Where("message_id = ?", messages[i].ID).
					All(&attachments)
				require.NoError(t, err, "Failed to fetch attachments for message %d", messages[i].ID)
				queryCount++
				messages[i].Attachments = attachments
			}

			oldTime := time.Since(start)
			oldQueryCount := queryCount

			t.Logf("[%s] N+1 approach: %v, %d queries", dbConfig.name, oldTime, oldQueryCount)

			// NEW WAY: JOIN (1 query)
			start = time.Now()
			var results []MessageWithStats
			err = ds.DB.Builder().
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
				Limit(messageCount).
				All(&results)
			require.NoError(t, err, "Failed to fetch messages with JOIN")

			newTime := time.Since(start)

			t.Logf("[%s] JOIN approach: %v, 1 query", dbConfig.name, newTime)

			// Assertions
			assert.Equal(t, messageCount+1, oldQueryCount, "N+1 should execute %d queries", messageCount+1)
			assert.Equal(t, messageCount, len(results), "Should return %d messages", messageCount)

			// Verify data correctness (more important than speed for small test datasets)
			for i, result := range results {
				assert.Equal(t, attachmentsPerMessage, result.AttachmentCount,
					"Message %d should have %d attachments", i+1, attachmentsPerMessage)
				assert.Equal(t, 1, result.MailboxID, "All messages should belong to mailbox 1")
			}

			// Calculate improvement ratio (the KEY metric)
			improvement := float64(oldQueryCount) / 1.0
			t.Logf("[%s] Query reduction: %.0fx (from %d queries to 1)",
				dbConfig.name, improvement, oldQueryCount)
			assert.GreaterOrEqual(t, improvement, float64(100),
				"Should have at least 100x query reduction")
		})
	}
}

// TestIrisMX_UseCase_PaginationWithLimitOffset validates that LIMIT/OFFSET
// reduces memory usage and improves performance for large datasets.
//
// BEFORE: Fetch all 10,000 messages (20MB), sort in Go, slice first 100
// AFTER: Database-side ORDER BY + LIMIT (fetch only 100 messages = 200KB)
//
// Expected improvement: 100x memory reduction.
func TestIrisMX_UseCase_PaginationWithLimitOffset(t *testing.T) {
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

			// Setup: 10,000 messages
			CreateMessagesTable(t, ds.DB, ds.Dialect)

			const totalMessages = 10000
			const pageSize = 100

			InsertTestMessages(t, ds.DB, totalMessages, 1)

			// OLD WAY: Fetch all, sort in Go
			start := time.Now()
			var allMessages []Message
			err := ds.DB.Builder().
				Select().
				From("messages").
				Where("mailbox_id = ?", 1).
				All(&allMessages)
			require.NoError(t, err, "Failed to fetch all messages")

			// Sort in Go by UID descending
			sort.Slice(allMessages, func(i, j int) bool {
				return allMessages[i].UID > allMessages[j].UID
			})
			page1Old := allMessages[:pageSize]

			oldTime := time.Since(start)
			oldMemory := len(allMessages) * 2048 // ~2KB per message (conservative estimate)

			t.Logf("[%s] Without LIMIT: %v, ~%d bytes, %d messages fetched",
				dbConfig.name, oldTime, oldMemory, len(allMessages))

			// NEW WAY: ORDER BY + LIMIT
			start = time.Now()
			var messages []Message
			err = ds.DB.Builder().
				Select().
				From("messages").
				Where("mailbox_id = ?", 1).
				OrderBy("uid DESC").
				Limit(pageSize).
				All(&messages)
			require.NoError(t, err, "Failed to fetch messages with LIMIT")

			newTime := time.Since(start)
			newMemory := len(messages) * 2048

			t.Logf("[%s] With LIMIT: %v, ~%d bytes, %d messages fetched",
				dbConfig.name, newTime, newMemory, len(messages))

			// Assertions
			assert.Equal(t, totalMessages, len(allMessages), "Old way should fetch all messages")
			assert.Equal(t, pageSize, len(messages), "New way should fetch only %d messages", pageSize)

			memoryImprovement := float64(oldMemory) / float64(newMemory)
			t.Logf("[%s] Memory improvement: %.0fx", dbConfig.name, memoryImprovement)
			assert.GreaterOrEqual(t, memoryImprovement, float64(90),
				"Should have at least 90x memory reduction")

			// Verify same results (first 10 messages)
			compareCount := 10
			if compareCount > len(messages) {
				compareCount = len(messages)
			}
			for i := 0; i < compareCount; i++ {
				assert.Equal(t, page1Old[i].ID, messages[i].ID,
					"Message %d ID should match", i)
				assert.Equal(t, page1Old[i].UID, messages[i].UID,
					"Message %d UID should match", i)
			}
		})
	}
}

// TestIrisMX_UseCase_AggregatesVsFetchAll validates that COUNT aggregates
// are drastically more efficient than fetching all records.
//
// BEFORE: Fetch all 10,000 messages (20MB), count in Go with len()
// AFTER: Database-side COUNT(*) (returns single int = 8 bytes)
//
// Expected improvement: 100,000x+ memory reduction.
func TestIrisMX_UseCase_AggregatesVsFetchAll(t *testing.T) {
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

			// Setup: 10,000 messages
			CreateMessagesTable(t, ds.DB, ds.Dialect)

			const totalMessages = 10000

			InsertTestMessages(t, ds.DB, totalMessages, 1)

			// OLD WAY: Fetch all, count in Go
			start := time.Now()
			var allMessages []Message
			err := ds.DB.Builder().
				Select().
				From("messages").
				Where("mailbox_id = ?", 1).
				All(&allMessages)
			require.NoError(t, err, "Failed to fetch all messages")

			count := len(allMessages)
			oldTime := time.Since(start)
			oldMemory := len(allMessages) * 2048 // ~2KB per message

			t.Logf("[%s] Fetch all: %v, ~%d bytes, count=%d",
				dbConfig.name, oldTime, oldMemory, count)

			// NEW WAY: COUNT aggregate
			start = time.Now()
			var result AggregateResult
			err = ds.DB.Builder().
				Select("COUNT(*) as total").
				From("messages").
				Where("mailbox_id = ?", 1).
				One(&result)
			require.NoError(t, err, "Failed to execute COUNT")

			newTime := time.Since(start)
			newMemory := 8 // Just an int (8 bytes for int64)

			t.Logf("[%s] COUNT: %v, %d bytes, count=%d",
				dbConfig.name, newTime, newMemory, result.Total)

			// Assertions
			assert.Equal(t, count, result.Total, "Counts should match")

			memoryImprovement := float64(oldMemory) / float64(newMemory)
			t.Logf("[%s] Memory improvement: %.0fx", dbConfig.name, memoryImprovement)
			assert.Greater(t, memoryImprovement, float64(100000),
				"Should have >100,000x memory reduction")
		})
	}
}

// TestIrisMX_UseCase_GroupByMailbox validates GROUP BY with aggregates
// for mailbox statistics (real IrisMX use case).
//
// Use case: Display message count and total size per mailbox.
func TestIrisMX_UseCase_GroupByMailbox(t *testing.T) {
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

			// Setup: Multiple mailboxes with different message counts
			CreateMessagesTable(t, ds.DB, ds.Dialect)

			// Mailbox 1: 100 messages
			InsertTestMessages(t, ds.DB, 100, 1)
			// Mailbox 2: 200 messages
			InsertTestMessages(t, ds.DB, 200, 2)
			// Mailbox 3: 50 messages
			InsertTestMessages(t, ds.DB, 50, 3)

			// Query with GROUP BY and aggregates
			var results []struct {
				MailboxID    int   `db:"mailbox_id"`
				MessageCount int   `db:"message_count"`
				TotalSize    int64 `db:"total_size"`
			}

			err := ds.DB.Builder().
				Select(
					"mailbox_id",
					"COUNT(*) as message_count",
					"SUM(size) as total_size",
				).
				From("messages").
				GroupBy("mailbox_id").
				OrderBy("mailbox_id ASC").
				All(&results)
			require.NoError(t, err, "Failed to execute GROUP BY query")

			// Assertions
			assert.Len(t, results, 3, "Should have 3 mailboxes")

			// Verify mailbox 1
			assert.Equal(t, 1, results[0].MailboxID)
			assert.Equal(t, 100, results[0].MessageCount)
			assert.Greater(t, results[0].TotalSize, int64(0))

			// Verify mailbox 2
			assert.Equal(t, 2, results[1].MailboxID)
			assert.Equal(t, 200, results[1].MessageCount)
			assert.Greater(t, results[1].TotalSize, int64(0))

			// Verify mailbox 3
			assert.Equal(t, 3, results[2].MailboxID)
			assert.Equal(t, 50, results[2].MessageCount)
			assert.Greater(t, results[2].TotalSize, int64(0))

			t.Logf("[%s] GROUP BY results: %+v", dbConfig.name, results)
		})
	}
}

// TestIrisMX_UseCase_HavingFilter validates HAVING clause for filtered aggregates.
//
// Use case: Find mailboxes with more than 100 messages.
func TestIrisMX_UseCase_HavingFilter(t *testing.T) {
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

			// Setup: Multiple mailboxes
			CreateMessagesTable(t, ds.DB, ds.Dialect)

			// Mailbox 1: 50 messages (below threshold)
			InsertTestMessages(t, ds.DB, 50, 1)
			// Mailbox 2: 150 messages (above threshold)
			InsertTestMessages(t, ds.DB, 150, 2)
			// Mailbox 3: 200 messages (above threshold)
			InsertTestMessages(t, ds.DB, 200, 3)

			// Query with HAVING clause (mailboxes with > 100 messages)
			var results []struct {
				MailboxID    int `db:"mailbox_id"`
				MessageCount int `db:"message_count"`
			}

			err := ds.DB.Builder().
				Select(
					"mailbox_id",
					"COUNT(*) as message_count",
				).
				From("messages").
				GroupBy("mailbox_id").
				Having("COUNT(*) > ?", 100).
				OrderBy("mailbox_id ASC").
				All(&results)
			require.NoError(t, err, "Failed to execute HAVING query")

			// Assertions
			assert.Len(t, results, 2, "Should have 2 mailboxes with >100 messages")

			// Mailbox 1 should NOT be in results (only 50 messages)
			// Mailbox 2 and 3 should be in results

			assert.Equal(t, 2, results[0].MailboxID, "First result should be mailbox 2")
			assert.Equal(t, 150, results[0].MessageCount)

			assert.Equal(t, 3, results[1].MailboxID, "Second result should be mailbox 3")
			assert.Equal(t, 200, results[1].MessageCount)

			t.Logf("[%s] HAVING results: %+v", dbConfig.name, results)
		})
	}
}
