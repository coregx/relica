//go:build integration
// +build integration

package test

import (
	"context"
	"testing"
	"time"

	"github.com/coregx/relica"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestUser struct {
	ID        int     `db:"id"`
	Name      string  `db:"name"`
	Email     string  `db:"email"`
	Age       int     `db:"age"`
	Status    int     `db:"status"`
	Role      string  `db:"role"`
	DeletedAt *string `db:"deleted_at"`
}

func TestExpressionAPI_PostgreSQL(t *testing.T) {
	ctx := context.Background()

	// Start PostgreSQL in Docker
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
	require.NoError(t, err)
	defer pgContainer.Terminate(ctx) //nolint:errcheck // Test cleanup

	// Get connection string (DSN)
	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Connect to database
	db, err := relica.NewDB("postgres", dsn)
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck // Test cleanup

	// Create test table
	_, err = db.ExecContext(ctx, `
        CREATE TABLE test_users (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            email TEXT NOT NULL,
            age INT NOT NULL,
            status INT NOT NULL,
            role TEXT NOT NULL,
            deleted_at TEXT
        )
    `)
	require.NoError(t, err)

	// Insert test data
	testData := []TestUser{
		{Name: "Alice", Email: "alice@example.com", Age: 25, Status: 1, Role: "admin", DeletedAt: nil},
		{Name: "Bob", Email: "bob@example.com", Age: 30, Status: 1, Role: "user", DeletedAt: nil},
		{Name: "Charlie", Email: "charlie@example.com", Age: 35, Status: 2, Role: "moderator", DeletedAt: nil},
		{Name: "David", Email: "david@example.com", Age: 28, Status: 1, Role: "user", DeletedAt: nil},
		{Name: "Eve", Email: "eve@example.com", Age: 22, Status: 0, Role: "user", DeletedAt: stringPtr("2025-10-20")},
	}

	for _, user := range testData {
		var deletedAt interface{} = nil
		if user.DeletedAt != nil {
			deletedAt = *user.DeletedAt
		}
		_, err := db.ExecContext(ctx,
			`INSERT INTO test_users (name, email, age, status, role, deleted_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			user.Name, user.Email, user.Age, user.Status, user.Role, deletedAt,
		)
		require.NoError(t, err)
	}

	// Test HashExp - simple equality
	t.Run("HashExp_SimpleEquality", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.HashExp{"status": 1}).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 3) // Alice, Bob, David
	})

	// Test HashExp - multiple conditions (AND)
	t.Run("HashExp_MultipleConditions", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.HashExp{
				"status": 1,
				"role":   "user",
			}).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Bob, David
	})

	// Test HashExp - IN clause
	t.Run("HashExp_IN", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.HashExp{
				"status": []interface{}{1, 2},
			}).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 4) // Alice, Bob, Charlie, David
	})

	// Test HashExp - IS NULL
	t.Run("HashExp_IsNull", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.HashExp{
				"deleted_at": nil,
			}).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 4) // All except Eve
	})

	// Test HashExp - combined (IN + NULL)
	t.Run("HashExp_Combined", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.HashExp{
				"status":     []interface{}{1, 2},
				"deleted_at": nil,
			}).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 4) // Alice, Bob, Charlie, David
	})

	// Test Eq
	t.Run("Eq", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.Eq("name", "Alice")).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 1)
		assert.Equal(t, "Alice", users[0].Name)
	})

	// Test NotEq
	t.Run("NotEq", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.NotEq("role", "admin")).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 4) // All except Alice
	})

	// Test GreaterThan
	t.Run("GreaterThan", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.GreaterThan("age", 28)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Bob (30), Charlie (35)
	})

	// Test LessThan
	t.Run("LessThan", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.LessThan("age", 26)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Alice (25), Eve (22)
	})

	// Test GreaterOrEqual
	t.Run("GreaterOrEqual", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.GreaterOrEqual("age", 30)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Bob (30), Charlie (35)
	})

	// Test LessOrEqual
	t.Run("LessOrEqual", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.LessOrEqual("age", 25)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Alice (25), Eve (22)
	})

	// Test In
	t.Run("In", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.In("role", "admin", "moderator")).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Alice, Charlie
	})

	// Test NotIn
	t.Run("NotIn", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.NotIn("role", "admin", "moderator")).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 3) // Bob, David, Eve
	})

	// Test Between
	t.Run("Between", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.Between("age", 25, 30)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 3) // Alice (25), David (28), Bob (30)
	})

	// Test NotBetween
	t.Run("NotBetween", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.NotBetween("age", 25, 30)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Eve (22), Charlie (35)
	})

	// Test Like
	t.Run("Like", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.Like("email", "example.com")).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 5) // All users have @example.com
	})

	// Test Like - specific pattern
	t.Run("Like_Specific", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.Like("name", "li")). // Matches Alice (contains "li")
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Alice, Charlie
	})

	// Test NotLike
	t.Run("NotLike", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.NotLike("name", "e")). // Not containing "e"
			All(&users)

		require.NoError(t, err)
		// Names: Alice (has e), Bob (no e), Charlie (has e), David (no e), Eve (has e)
		assert.Len(t, users, 2) // Bob and David don't contain 'e'
		// Verify it's actually Bob and David
		names := make(map[string]bool)
		for _, u := range users {
			names[u.Name] = true
		}
		assert.True(t, names["Bob"])
		assert.True(t, names["David"])
	})

	// Test And
	t.Run("And", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.And(
				relica.Eq("status", 1),
				relica.GreaterThan("age", 25),
			)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Bob (30), David (28)
	})

	// Test Or
	t.Run("Or", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.Or(
				relica.Eq("role", "admin"),
				relica.Eq("role", "moderator"),
			)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Alice, Charlie
	})

	// Test Not
	t.Run("Not", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.Not(
				relica.Eq("status", 1),
			)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Charlie (status=2), Eve (status=0)
	})

	// Test Complex - nested And/Or
	t.Run("Complex_NestedAndOr", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.And(
				relica.Eq("status", 1),
				relica.Or(
					relica.Eq("role", "admin"),
					relica.GreaterThan("age", 27),
				),
			)).
			All(&users)

		require.NoError(t, err)
		// status=1 AND (role='admin' OR age>27)
		// Alice: status=1, role=admin ✓
		// Bob: status=1, age=30 ✓
		// David: status=1, age=28 ✓
		assert.Len(t, users, 3)
	})

	// Test Complex - HashExp + Expression
	t.Run("Complex_HashExpWithExpression", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where(relica.And(
				relica.HashExp{
					"status":     1,
					"deleted_at": nil,
				},
				relica.Or(
					relica.Like("email", "alice"),
					relica.Like("email", "bob"),
				),
			)).
			All(&users)

		require.NoError(t, err)
		// status=1 AND deleted_at IS NULL AND (email LIKE '%alice%' OR email LIKE '%bob%')
		assert.Len(t, users, 2) // Alice, Bob
	})

	// Test UPDATE with Expression
	t.Run("Update_WithExpression", func(t *testing.T) {
		_, err := db.Builder().
			Update("test_users").
			Set(map[string]interface{}{
				"status": 3,
			}).
			Where(relica.Eq("name", "Eve")).
			Execute()

		require.NoError(t, err)

		// Verify update (check rows affected is not necessary for correctness test)
		var user TestUser
		err = db.Builder().
			Select().
			From("test_users").
			Where(relica.Eq("name", "Eve")).
			One(&user)

		require.NoError(t, err)
		assert.Equal(t, 3, user.Status)
	})

	// Test DELETE with Expression
	t.Run("Delete_WithExpression", func(t *testing.T) {
		_, err := db.Builder().
			Delete("test_users").
			Where(relica.And(
				relica.Eq("status", 3),
				relica.Eq("name", "Eve"),
			)).
			Execute()

		require.NoError(t, err)

		// Verify deletion
		var count int
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM test_users WHERE name = $1`, "Eve").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	// Test backward compatibility - string-based Where still works
	t.Run("BackwardCompatibility_String", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where("status = ? AND role = ?", 1, "admin").
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 1) // Alice
	})

	// Test mixed - string + Expression
	t.Run("Mixed_StringAndExpression", func(t *testing.T) {
		var users []TestUser
		err := db.Builder().
			Select().
			From("test_users").
			Where("status = ?", 1).
			Where(relica.GreaterThan("age", 25)).
			All(&users)

		require.NoError(t, err)
		assert.Len(t, users, 2) // Bob (30), David (28)
	})
}

func stringPtr(s string) *string {
	return &s
}
