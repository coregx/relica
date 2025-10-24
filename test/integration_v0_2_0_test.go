//go:build integration
// +build integration

package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJOIN_AllDatabases validates JOIN operations across all supported databases.
func TestJOIN_AllDatabases(t *testing.T) {
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

			// Setup tables
			CreateUsersTable(t, ds.DB, ds.Dialect)
			CreatePostsTable(t, ds.DB, ds.Dialect)

			// Insert test data
			InsertTestUsers(t, ds.DB, 10)
			InsertTestPosts(t, ds.DB, 1, 5) // User 1 has 5 posts
			InsertTestPosts(t, ds.DB, 2, 3) // User 2 has 3 posts

			t.Run("InnerJoin", func(t *testing.T) {
				var results []struct {
					UserID   int    `db:"user_id"`
					UserName string `db:"user_name"`
					PostID   int    `db:"post_id"`
					Title    string `db:"title"`
				}

				err := ds.DB.Builder().
					Select("u.id as user_id", "u.name as user_name", "p.id as post_id", "p.title").
					From("users u").
					InnerJoin("posts p", "p.user_id = u.id").
					OrderBy("u.id ASC", "p.id ASC").
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 8, "Should have 8 results (5 posts for user 1 + 3 for user 2)")

				// Verify first result
				assert.Equal(t, 1, results[0].UserID)
				assert.Equal(t, "User1", results[0].UserName)
			})

			t.Run("LeftJoin", func(t *testing.T) {
				var results []struct {
					UserID    int    `db:"user_id"`
					UserName  string `db:"user_name"`
					PostCount int    `db:"post_count"`
				}

				err := ds.DB.Builder().
					Select("u.id as user_id", "u.name as user_name", "COUNT(p.id) as post_count").
					From("users u").
					LeftJoin("posts p", "p.user_id = u.id").
					GroupBy("u.id", "u.name").
					OrderBy("u.id ASC").
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 10, "Should have 10 users")

				// User 1 should have 5 posts
				assert.Equal(t, 1, results[0].UserID)
				assert.Equal(t, 5, results[0].PostCount)

				// User 2 should have 3 posts
				assert.Equal(t, 2, results[1].UserID)
				assert.Equal(t, 3, results[1].PostCount)

				// User 3 should have 0 posts (LEFT JOIN includes users without posts)
				assert.Equal(t, 3, results[2].UserID)
				assert.Equal(t, 0, results[2].PostCount)
			})

			t.Run("MultipleJoins", func(t *testing.T) {
				// Test multiple JOINs in one query
				var results []struct {
					MessageID       int    `db:"message_id"`
					Subject         string `db:"subject"`
					AttachmentCount int    `db:"attachment_count"`
				}

				CreateMessagesTable(t, ds.DB, ds.Dialect)
				CreateAttachmentsTable(t, ds.DB, ds.Dialect)
				InsertTestMessages(t, ds.DB, 5, 1)
				InsertTestAttachments(t, ds.DB, 5, 2) // 2 attachments per message

				err := ds.DB.Builder().
					Select("m.id as message_id", "m.subject", "COUNT(a.id) as attachment_count").
					From("messages m").
					LeftJoin("attachments a", "a.message_id = m.id").
					GroupBy("m.id", "m.subject").
					OrderBy("m.id ASC").
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 5, "Should have 5 messages")

				// Each message should have 2 attachments
				for i, result := range results {
					assert.Equal(t, 2, result.AttachmentCount, "Message %d should have 2 attachments", i+1)
				}
			})
		})
	}
}

// TestOrderByLimitOffset_AllDatabases validates ORDER BY, LIMIT, OFFSET across databases.
func TestOrderByLimitOffset_AllDatabases(t *testing.T) {
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

			CreateUsersTable(t, ds.DB, ds.Dialect)
			InsertTestUsers(t, ds.DB, 50)

			t.Run("OrderBy_SingleColumn", func(t *testing.T) {
				var results []TestUser
				err := ds.DB.Builder().
					Select().
					From("users").
					OrderBy("age ASC").
					Limit(10).
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 10)

				// Verify ascending order
				for i := 1; i < len(results); i++ {
					assert.GreaterOrEqual(t, results[i].Age, results[i-1].Age,
						"Ages should be in ascending order")
				}
			})

			t.Run("OrderBy_MultipleColumns", func(t *testing.T) {
				var results []TestUser
				err := ds.DB.Builder().
					Select().
					From("users").
					OrderBy("status DESC", "age ASC").
					Limit(10).
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 10)
			})

			t.Run("Limit", func(t *testing.T) {
				var results []TestUser
				err := ds.DB.Builder().
					Select().
					From("users").
					Limit(15).
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 15, "Should return exactly 15 rows")
			})

			t.Run("Offset", func(t *testing.T) {
				// Get first 10
				var page1 []TestUser
				err := ds.DB.Builder().
					Select().
					From("users").
					OrderBy("id ASC").
					Limit(10).
					All(&page1)
				require.NoError(t, err)

				// Get second 10 (offset 10)
				var page2 []TestUser
				err = ds.DB.Builder().
					Select().
					From("users").
					OrderBy("id ASC").
					Limit(10).
					Offset(10).
					All(&page2)
				require.NoError(t, err)

				assert.Len(t, page1, 10)
				assert.Len(t, page2, 10)

				// Verify no overlap
				assert.NotEqual(t, page1[0].ID, page2[0].ID, "Pages should not overlap")
				assert.Less(t, page1[9].ID, page2[0].ID, "Page 2 IDs should be greater than Page 1")
			})

			t.Run("Pagination_Combined", func(t *testing.T) {
				const pageSize = 10
				const pageNumber = 2 // Third page (0-indexed)

				var results []TestUser
				err := ds.DB.Builder().
					Select().
					From("users").
					OrderBy("id ASC").
					Limit(pageSize).
					Offset(pageNumber * pageSize).
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, pageSize)

				// Verify we got the correct page
				if len(results) > 0 {
					expectedMinID := (pageNumber * pageSize) + 1
					assert.GreaterOrEqual(t, results[0].ID, expectedMinID,
						"First ID on page 3 should be >= %d", expectedMinID)
				}
			})
		})
	}
}

// TestAggregates_AllDatabases validates aggregate functions across databases.
func TestAggregates_AllDatabases(t *testing.T) {
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

			CreateMessagesTable(t, ds.DB, ds.Dialect)
			InsertTestMessages(t, ds.DB, 100, 1)

			t.Run("COUNT", func(t *testing.T) {
				var result AggregateResult
				err := ds.DB.Builder().
					Select("COUNT(*) as total").
					From("messages").
					One(&result)

				require.NoError(t, err)
				assert.Equal(t, 100, result.Total)
			})

			t.Run("SUM", func(t *testing.T) {
				var result AggregateResult
				err := ds.DB.Builder().
					Select("SUM(size) as sum").
					From("messages").
					One(&result)

				require.NoError(t, err)
				assert.Greater(t, result.Sum, int64(0), "SUM should be positive")
			})

			t.Run("AVG", func(t *testing.T) {
				var result AggregateResult
				err := ds.DB.Builder().
					Select("AVG(size) as avg").
					From("messages").
					One(&result)

				require.NoError(t, err)
				assert.Greater(t, result.Avg, float64(0), "AVG should be positive")
			})

			t.Run("MIN_MAX", func(t *testing.T) {
				var result AggregateResult
				err := ds.DB.Builder().
					Select("MIN(uid) as min", "MAX(uid) as max").
					From("messages").
					One(&result)

				require.NoError(t, err)
				assert.Equal(t, 1, result.Min, "MIN UID should be 1")
				assert.Equal(t, 100, result.Max, "MAX UID should be 100")
			})

			t.Run("MultipleAggregates", func(t *testing.T) {
				var result AggregateResult
				err := ds.DB.Builder().
					Select(
						"COUNT(*) as total",
						"SUM(size) as sum",
						"AVG(size) as avg",
						"MIN(uid) as min",
						"MAX(uid) as max",
					).
					From("messages").
					One(&result)

				require.NoError(t, err)
				assert.Equal(t, 100, result.Total)
				assert.Greater(t, result.Sum, int64(0))
				assert.Greater(t, result.Avg, float64(0))
				assert.Equal(t, 1, result.Min)
				assert.Equal(t, 100, result.Max)
			})
		})
	}
}

// TestGroupBy_AllDatabases validates GROUP BY across databases.
func TestGroupBy_AllDatabases(t *testing.T) {
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

			CreateMessagesTable(t, ds.DB, ds.Dialect)

			// Insert messages for 3 mailboxes
			InsertTestMessages(t, ds.DB, 50, 1)
			InsertTestMessages(t, ds.DB, 30, 2)
			InsertTestMessages(t, ds.DB, 20, 3)

			t.Run("GroupBy_SingleColumn", func(t *testing.T) {
				var results []struct {
					MailboxID int `db:"mailbox_id"`
					Count     int `db:"count"`
				}

				err := ds.DB.Builder().
					Select("mailbox_id", "COUNT(*) as count").
					From("messages").
					GroupBy("mailbox_id").
					OrderBy("mailbox_id ASC").
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 3)

				assert.Equal(t, 1, results[0].MailboxID)
				assert.Equal(t, 50, results[0].Count)

				assert.Equal(t, 2, results[1].MailboxID)
				assert.Equal(t, 30, results[1].Count)

				assert.Equal(t, 3, results[2].MailboxID)
				assert.Equal(t, 20, results[2].Count)
			})

			t.Run("GroupBy_WithAggregates", func(t *testing.T) {
				var results []struct {
					MailboxID int     `db:"mailbox_id"`
					Count     int     `db:"count"`
					TotalSize int64   `db:"total_size"`
					AvgSize   float64 `db:"avg_size"`
				}

				err := ds.DB.Builder().
					Select(
						"mailbox_id",
						"COUNT(*) as count",
						"SUM(size) as total_size",
						"AVG(size) as avg_size",
					).
					From("messages").
					GroupBy("mailbox_id").
					OrderBy("mailbox_id ASC").
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 3)

				for i, result := range results {
					assert.Greater(t, result.Count, 0, "Mailbox %d should have messages", i+1)
					assert.GreaterOrEqual(t, result.TotalSize, int64(0))
					assert.GreaterOrEqual(t, result.AvgSize, float64(0))
				}
			})
		})
	}
}

// TestHaving_AllDatabases validates HAVING clause across databases.
func TestHaving_AllDatabases(t *testing.T) {
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

			CreateMessagesTable(t, ds.DB, ds.Dialect)

			// Insert messages for 5 mailboxes with varying counts
			InsertTestMessages(t, ds.DB, 10, 1)  // Below threshold
			InsertTestMessages(t, ds.DB, 120, 2) // Above threshold
			InsertTestMessages(t, ds.DB, 30, 3)  // Below threshold
			InsertTestMessages(t, ds.DB, 150, 4) // Above threshold
			InsertTestMessages(t, ds.DB, 5, 5)   // Below threshold

			t.Run("Having_Simple", func(t *testing.T) {
				var results []struct {
					MailboxID int `db:"mailbox_id"`
					Count     int `db:"count"`
				}

				err := ds.DB.Builder().
					Select("mailbox_id", "COUNT(*) as count").
					From("messages").
					GroupBy("mailbox_id").
					Having("COUNT(*) > ?", 100).
					OrderBy("mailbox_id ASC").
					All(&results)

				require.NoError(t, err)
				assert.Len(t, results, 2, "Should have 2 mailboxes with >100 messages")

				assert.Equal(t, 2, results[0].MailboxID)
				assert.Equal(t, 120, results[0].Count)

				assert.Equal(t, 4, results[1].MailboxID)
				assert.Equal(t, 150, results[1].Count)
			})

			t.Run("Having_WithMultipleConditions", func(t *testing.T) {
				var results []struct {
					MailboxID int   `db:"mailbox_id"`
					Count     int   `db:"count"`
					TotalSize int64 `db:"total_size"`
				}

				err := ds.DB.Builder().
					Select("mailbox_id", "COUNT(*) as count", "SUM(size) as total_size").
					From("messages").
					GroupBy("mailbox_id").
					Having("COUNT(*) > ?", 10).
					OrderBy("count DESC").
					All(&results)

				require.NoError(t, err)
				assert.GreaterOrEqual(t, len(results), 3, "Should have at least 3 mailboxes with >10 messages")

				// Verify all results have > 10 messages
				for _, result := range results {
					assert.Greater(t, result.Count, 10)
				}

				// Verify descending order by count
				for i := 1; i < len(results); i++ {
					assert.LessOrEqual(t, results[i].Count, results[i-1].Count,
						"Results should be in descending order by count")
				}
			})
		})
	}
}

// TestComplexQuery_AllDatabases validates complex queries combining all features.
func TestComplexQuery_AllDatabases(t *testing.T) {
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

			// Setup complex schema
			CreateMessagesTable(t, ds.DB, ds.Dialect)
			CreateAttachmentsTable(t, ds.DB, ds.Dialect)
			CreateUsersTable(t, ds.DB, ds.Dialect)

			InsertTestUsers(t, ds.DB, 10)
			InsertTestMessages(t, ds.DB, 100, 1)
			InsertTestAttachments(t, ds.DB, 100, 3) // 3 attachments per message

			// Complex query: JOIN + WHERE + GROUP BY + HAVING + ORDER BY + LIMIT
			var results []struct {
				UserID          int     `db:"user_id"`
				UserName        string  `db:"user_name"`
				MessageCount    int     `db:"message_count"`
				AttachmentCount int     `db:"attachment_count"`
				AvgMessageSize  float64 `db:"avg_message_size"`
			}

			err := ds.DB.Builder().
				Select(
					"u.id as user_id",
					"u.name as user_name",
					"COUNT(DISTINCT m.id) as message_count",
					"COUNT(a.id) as attachment_count",
					"AVG(m.size) as avg_message_size",
				).
				From("users u").
				InnerJoin("messages m", "m.user_id = u.id").
				LeftJoin("attachments a", "a.message_id = m.id").
				Where("m.mailbox_id = ?", 1).
				GroupBy("u.id", "u.name").
				Having("COUNT(DISTINCT m.id) > ?", 0).
				OrderBy("message_count DESC").
				Limit(5).
				All(&results)

			require.NoError(t, err)
			assert.NotEmpty(t, results, "Should return results")

			// Verify all users have messages
			for _, result := range results {
				assert.Greater(t, result.MessageCount, 0, "User should have messages")
				assert.Greater(t, result.AttachmentCount, 0, "User should have attachments")
				assert.NotEmpty(t, result.UserName)
			}

			t.Logf("[%s] Complex query returned %d users with messages", dbConfig.name, len(results))
		})
	}
}
