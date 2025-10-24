//go:build integration
// +build integration

package test

import (
	"context"
	"testing"
	"time"

	"github.com/coregx/relica"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type User struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

func TestPostgresIntegration(t *testing.T) {
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
        CREATE TABLE users (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            email TEXT UNIQUE NOT NULL
        )
    `)
	require.NoError(t, err)

	// Test INSERT (PostgreSQL requires RETURNING clause instead of LastInsertId)
	user := User{Name: "Alice", Email: "alice@example.com"}
	var insertedID int
	err = db.QueryRowContext(ctx,
		`INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`,
		user.Name, user.Email,
	).Scan(&insertedID)
	require.NoError(t, err)
	require.Greater(t, insertedID, 0, "ID should be auto-generated")
	user.ID = insertedID

	// Test SELECT via Query Builder
	var fetched User
	err = db.Builder().Select().From("users").Where("id = ?", user.ID).One(&fetched)
	require.NoError(t, err)
	require.Equal(t, user.Name, fetched.Name)
	require.Equal(t, user.Email, fetched.Email)
	require.Equal(t, user.ID, fetched.ID)
}
