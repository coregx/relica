package relica_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestDB_Wrapper tests all DB wrapper methods.
func TestDB_Wrapper(t *testing.T) {
	t.Run("Open", func(t *testing.T) {
		db, err := relica.Open("sqlite", ":memory:")
		require.NoError(t, err)
		defer db.Close()
		assert.NotNil(t, db)
	})

	t.Run("NewDB", func(t *testing.T) {
		db, err := relica.NewDB("sqlite", ":memory:")
		require.NoError(t, err)
		defer db.Close()
		assert.NotNil(t, db)
	})

	t.Run("WrapDB", func(t *testing.T) {
		sqlDB, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)
		defer sqlDB.Close()

		db := relica.WrapDB(sqlDB, "sqlite")
		assert.NotNil(t, db)
	})

	t.Run("Close", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		err := db.Close()
		assert.NoError(t, err)
	})

	t.Run("WithContext", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		ctx := context.Background()
		dbWithCtx := db.WithContext(ctx)
		assert.NotNil(t, dbWithCtx)
	})

	t.Run("Builder", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		qb := db.Builder()
		assert.NotNil(t, qb)
	})

	t.Run("Begin", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		ctx := context.Background()
		tx, err := db.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback()

		assert.NotNil(t, tx)
	})

	t.Run("BeginTx", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		ctx := context.Background()
		opts := &relica.TxOptions{
			Isolation: sql.LevelSerializable,
			ReadOnly:  false,
		}
		tx, err := db.BeginTx(ctx, opts)
		require.NoError(t, err)
		defer tx.Rollback()

		assert.NotNil(t, tx)
	})

	t.Run("ExecContext", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		ctx := context.Background()
		_, err := db.ExecContext(ctx, "CREATE TABLE test (id INTEGER)")
		assert.NoError(t, err)
	})

	t.Run("QueryContext", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		ctx := context.Background()
		db.ExecContext(ctx, "CREATE TABLE test (id INTEGER)")

		rows, err := db.QueryContext(ctx, "SELECT * FROM test")
		require.NoError(t, err)
		defer rows.Close()
		assert.NotNil(t, rows)
	})

	t.Run("QueryRowContext", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		ctx := context.Background()
		row := db.QueryRowContext(ctx, "SELECT 1")
		assert.NotNil(t, row)

		var val int
		err := row.Scan(&val)
		assert.NoError(t, err)
		assert.Equal(t, 1, val)
	})

	t.Run("QuoteTableName", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		quoted := db.QuoteTableName("users")
		assert.Equal(t, `"users"`, quoted)
	})

	t.Run("QuoteColumnName", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		quoted := db.QuoteColumnName("user_id")
		assert.Equal(t, `"user_id"`, quoted)
	})

	t.Run("GenerateParamName", func(t *testing.T) {
		db, _ := relica.Open("sqlite", ":memory:")
		defer db.Close()

		param := db.GenerateParamName()
		assert.NotEmpty(t, param)
	})
}

// TestTx_Wrapper tests all Tx wrapper methods.
func TestTx_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()

	t.Run("Builder", func(t *testing.T) {
		tx, _ := db.Begin(ctx)
		defer tx.Rollback()

		qb := tx.Builder()
		assert.NotNil(t, qb)
	})

	t.Run("Commit", func(t *testing.T) {
		tx, _ := db.Begin(ctx)

		err := tx.Commit()
		assert.NoError(t, err)
	})

	t.Run("Rollback", func(t *testing.T) {
		tx, _ := db.Begin(ctx)

		err := tx.Rollback()
		assert.NoError(t, err)
	})
}

// TestQueryBuilder_Wrapper tests all QueryBuilder wrapper methods.
func TestQueryBuilder_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT, status INTEGER)")

	t.Run("WithContext", func(t *testing.T) {
		qb := db.Builder().WithContext(ctx)
		assert.NotNil(t, qb)
	})

	t.Run("Select", func(t *testing.T) {
		sq := db.Builder().Select("id", "name")
		assert.NotNil(t, sq)
	})

	t.Run("Insert", func(t *testing.T) {
		q := db.Builder().Insert("users", map[string]interface{}{
			"name":   "Alice",
			"email":  "alice@example.com",
			"status": 1,
		})
		assert.NotNil(t, q)
		_, err := q.Execute()
		assert.NoError(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		uq := db.Builder().Update("users")
		assert.NotNil(t, uq)
	})

	t.Run("Delete", func(t *testing.T) {
		dq := db.Builder().Delete("users")
		assert.NotNil(t, dq)
	})

	t.Run("BatchInsert", func(t *testing.T) {
		biq := db.Builder().BatchInsert("users", []string{"name", "email", "status"})
		assert.NotNil(t, biq)
	})

	t.Run("BatchUpdate", func(t *testing.T) {
		buq := db.Builder().BatchUpdate("users", "id")
		assert.NotNil(t, buq)
	})

	t.Run("Upsert", func(t *testing.T) {
		uq := db.Builder().Upsert("users", map[string]interface{}{
			"id":     1,
			"name":   "Alice",
			"email":  "alice@example.com",
			"status": 1,
		})
		assert.NotNil(t, uq)
	})
}

// TestSelectQuery_Wrapper tests all SelectQuery wrapper methods.
func TestSelectQuery_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT, status INTEGER, age INTEGER)")
	db.ExecContext(ctx, "INSERT INTO users (name, email, status, age) VALUES ('Alice', 'alice@example.com', 1, 25)")
	db.ExecContext(ctx, "INSERT INTO users (name, email, status, age) VALUES ('Bob', 'bob@example.com', 2, 30)")

	t.Run("WithContext", func(t *testing.T) {
		sq := db.Builder().Select("*").WithContext(ctx)
		assert.NotNil(t, sq)
	})

	t.Run("From", func(t *testing.T) {
		sq := db.Builder().Select("*").From("users")
		assert.NotNil(t, sq)
	})

	t.Run("FromSelect", func(t *testing.T) {
		sub := db.Builder().Select("id").From("users")
		sq := db.Builder().Select("*").FromSelect(sub, "sub")
		assert.NotNil(t, sq)
	})

	t.Run("SelectExpr", func(t *testing.T) {
		sq := db.Builder().Select("id").SelectExpr("COUNT(*) as count").From("users")
		assert.NotNil(t, sq)
	})

	t.Run("Where", func(t *testing.T) {
		sq := db.Builder().Select("*").From("users").Where("id = ?", 1)
		assert.NotNil(t, sq)
	})

	t.Run("InnerJoin", func(t *testing.T) {
		db.ExecContext(ctx, "CREATE TABLE orders (id INTEGER, user_id INTEGER)")
		sq := db.Builder().Select("u.name").From("users u").InnerJoin("orders o", "o.user_id = u.id")
		assert.NotNil(t, sq)
	})

	t.Run("LeftJoin", func(t *testing.T) {
		sq := db.Builder().Select("u.name").From("users u").LeftJoin("orders o", "o.user_id = u.id")
		assert.NotNil(t, sq)
	})

	t.Run("RightJoin", func(t *testing.T) {
		sq := db.Builder().Select("u.name").From("users u").RightJoin("orders o", "o.user_id = u.id")
		assert.NotNil(t, sq)
	})

	t.Run("FullJoin", func(t *testing.T) {
		sq := db.Builder().Select("u.name").From("users u").FullJoin("orders o", "o.user_id = u.id")
		assert.NotNil(t, sq)
	})

	t.Run("CrossJoin", func(t *testing.T) {
		sq := db.Builder().Select("*").From("users").CrossJoin("orders")
		assert.NotNil(t, sq)
	})

	t.Run("OrderBy", func(t *testing.T) {
		sq := db.Builder().Select("*").From("users").OrderBy("name DESC")
		assert.NotNil(t, sq)
	})

	t.Run("Limit", func(t *testing.T) {
		sq := db.Builder().Select("*").From("users").Limit(10)
		assert.NotNil(t, sq)
	})

	t.Run("Offset", func(t *testing.T) {
		sq := db.Builder().Select("*").From("users").Offset(5)
		assert.NotNil(t, sq)
	})

	t.Run("GroupBy", func(t *testing.T) {
		sq := db.Builder().Select("status", "COUNT(*) as count").From("users").GroupBy("status")
		assert.NotNil(t, sq)
	})

	t.Run("Having", func(t *testing.T) {
		sq := db.Builder().Select("status", "COUNT(*) as count").From("users").GroupBy("status").Having("COUNT(*) > ?", 0)
		assert.NotNil(t, sq)
	})

	t.Run("Union", func(t *testing.T) {
		q1 := db.Builder().Select("name").From("users").Where("status = ?", 1)
		q2 := db.Builder().Select("name").From("users").Where("status = ?", 2)
		sq := q1.Union(q2)
		assert.NotNil(t, sq)
	})

	t.Run("UnionAll", func(t *testing.T) {
		q1 := db.Builder().Select("name").From("users").Where("status = ?", 1)
		q2 := db.Builder().Select("name").From("users").Where("status = ?", 2)
		sq := q1.UnionAll(q2)
		assert.NotNil(t, sq)
	})

	t.Run("Intersect", func(t *testing.T) {
		q1 := db.Builder().Select("name").From("users")
		q2 := db.Builder().Select("name").From("users").Where("status = ?", 1)
		sq := q1.Intersect(q2)
		assert.NotNil(t, sq)
	})

	t.Run("Except", func(t *testing.T) {
		q1 := db.Builder().Select("name").From("users")
		q2 := db.Builder().Select("name").From("users").Where("status = ?", 2)
		sq := q1.Except(q2)
		assert.NotNil(t, sq)
	})

	t.Run("With", func(t *testing.T) {
		cte := db.Builder().Select("status", "COUNT(*) as count").From("users").GroupBy("status")
		sq := db.Builder().Select("*").With("stats", cte).From("stats")
		assert.NotNil(t, sq)
	})

	t.Run("WithRecursive", func(t *testing.T) {
		db.ExecContext(ctx, "CREATE TABLE tree (id INTEGER, parent_id INTEGER)")
		anchor := db.Builder().Select("id", "parent_id", "1 as level").From("tree").Where("parent_id IS NULL")
		recursive := db.Builder().Select("t.id", "t.parent_id", "h.level + 1").From("tree t").InnerJoin("hierarchy h", "t.parent_id = h.id")
		cte := anchor.UnionAll(recursive)
		sq := db.Builder().Select("*").WithRecursive("hierarchy", cte).From("hierarchy")
		assert.NotNil(t, sq)
	})

	t.Run("Build", func(t *testing.T) {
		q := db.Builder().Select("*").From("users").Build()
		assert.NotNil(t, q)
	})

	t.Run("One", func(t *testing.T) {
		type User struct {
			ID     int    `db:"id"`
			Name   string `db:"name"`
			Email  string `db:"email"`
			Status int    `db:"status"`
			Age    int    `db:"age"`
		}

		var user User
		err := db.Builder().Select("*").From("users").Where("id = ?", 1).One(&user)
		require.NoError(t, err)
		assert.Equal(t, "Alice", user.Name)
	})

	t.Run("All", func(t *testing.T) {
		type User struct {
			ID     int    `db:"id"`
			Name   string `db:"name"`
			Email  string `db:"email"`
			Status int    `db:"status"`
			Age    int    `db:"age"`
		}

		var users []User
		err := db.Builder().Select("*").From("users").All(&users)
		require.NoError(t, err)
		assert.Len(t, users, 2)
	})

	t.Run("AsExpression", func(t *testing.T) {
		sub := db.Builder().Select("id").From("users").Where("status = ?", 1)
		expr := sub.AsExpression()
		assert.NotNil(t, expr)
	})
}

// TestUpdateQuery_Wrapper tests all UpdateQuery wrapper methods.
func TestUpdateQuery_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, status INTEGER)")
	db.ExecContext(ctx, "INSERT INTO users (name, status) VALUES ('Alice', 1)")

	t.Run("WithContext", func(t *testing.T) {
		uq := db.Builder().Update("users").WithContext(ctx)
		assert.NotNil(t, uq)
	})

	t.Run("Set", func(t *testing.T) {
		uq := db.Builder().Update("users").Set(map[string]interface{}{"status": 2})
		assert.NotNil(t, uq)
	})

	t.Run("Where", func(t *testing.T) {
		uq := db.Builder().Update("users").Set(map[string]interface{}{"status": 2}).Where("id = ?", 1)
		assert.NotNil(t, uq)
	})

	t.Run("Build", func(t *testing.T) {
		q := db.Builder().Update("users").Set(map[string]interface{}{"status": 2}).Build()
		assert.NotNil(t, q)
	})

	t.Run("Execute", func(t *testing.T) {
		result, err := db.Builder().Update("users").Set(map[string]interface{}{"status": 2}).Where("id = ?", 1).Execute()
		require.NoError(t, err)
		rows, _ := result.RowsAffected()
		assert.Equal(t, int64(1), rows)
	})
}

// TestDeleteQuery_Wrapper tests all DeleteQuery wrapper methods.
func TestDeleteQuery_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	db.ExecContext(ctx, "INSERT INTO users (name) VALUES ('Alice')")

	t.Run("WithContext", func(t *testing.T) {
		dq := db.Builder().Delete("users").WithContext(ctx)
		assert.NotNil(t, dq)
	})

	t.Run("Where", func(t *testing.T) {
		dq := db.Builder().Delete("users").Where("id = ?", 1)
		assert.NotNil(t, dq)
	})

	t.Run("Build", func(t *testing.T) {
		q := db.Builder().Delete("users").Build()
		assert.NotNil(t, q)
	})

	t.Run("Execute", func(t *testing.T) {
		result, err := db.Builder().Delete("users").Where("id = ?", 1).Execute()
		require.NoError(t, err)
		rows, _ := result.RowsAffected()
		assert.Equal(t, int64(1), rows)
	})
}

// TestUpsertQuery_Wrapper tests all UpsertQuery wrapper methods.
func TestUpsertQuery_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)")

	t.Run("WithContext", func(t *testing.T) {
		uq := db.Builder().Upsert("users", map[string]interface{}{"id": 1, "name": "Alice"}).WithContext(ctx)
		assert.NotNil(t, uq)
	})

	t.Run("OnConflict", func(t *testing.T) {
		uq := db.Builder().Upsert("users", map[string]interface{}{"id": 1, "name": "Alice"}).OnConflict("id")
		assert.NotNil(t, uq)
	})

	t.Run("DoUpdate", func(t *testing.T) {
		uq := db.Builder().Upsert("users", map[string]interface{}{"id": 1, "name": "Alice"}).OnConflict("id").DoUpdate("name")
		assert.NotNil(t, uq)
	})

	t.Run("DoNothing", func(t *testing.T) {
		uq := db.Builder().Upsert("users", map[string]interface{}{"id": 1, "name": "Alice"}).OnConflict("id").DoNothing()
		assert.NotNil(t, uq)
	})

	t.Run("Build", func(t *testing.T) {
		q := db.Builder().Upsert("users", map[string]interface{}{"id": 1, "name": "Alice"}).Build()
		assert.NotNil(t, q)
	})

	t.Run("Execute", func(t *testing.T) {
		_, err := db.Builder().Upsert("users", map[string]interface{}{
			"id":    1,
			"name":  "Alice",
			"email": "alice@example.com",
		}).OnConflict("id").DoUpdate("name", "email").Execute()
		assert.NoError(t, err)
	})
}

// TestBatchInsertQuery_Wrapper tests all BatchInsertQuery wrapper methods.
func TestBatchInsertQuery_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)")

	t.Run("WithContext", func(t *testing.T) {
		biq := db.Builder().BatchInsert("users", []string{"name", "email"}).WithContext(ctx)
		assert.NotNil(t, biq)
	})

	t.Run("Values", func(t *testing.T) {
		biq := db.Builder().BatchInsert("users", []string{"name", "email"}).
			Values("Alice", "alice@example.com")
		assert.NotNil(t, biq)
	})

	t.Run("ValuesMap", func(t *testing.T) {
		biq := db.Builder().BatchInsert("users", []string{"name", "email"}).
			ValuesMap(map[string]interface{}{"name": "Alice", "email": "alice@example.com"})
		assert.NotNil(t, biq)
	})

	t.Run("Build", func(t *testing.T) {
		q := db.Builder().BatchInsert("users", []string{"name", "email"}).
			Values("Alice", "alice@example.com").
			Build()
		assert.NotNil(t, q)
	})

	t.Run("Execute", func(t *testing.T) {
		_, err := db.Builder().BatchInsert("users", []string{"name", "email"}).
			Values("Alice", "alice@example.com").
			Values("Bob", "bob@example.com").
			Execute()
		assert.NoError(t, err)
	})
}

// TestBatchUpdateQuery_Wrapper tests all BatchUpdateQuery wrapper methods.
func TestBatchUpdateQuery_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, status INTEGER)")
	db.ExecContext(ctx, "INSERT INTO users (id, name, status) VALUES (1, 'Alice', 1)")
	db.ExecContext(ctx, "INSERT INTO users (id, name, status) VALUES (2, 'Bob', 1)")

	t.Run("WithContext", func(t *testing.T) {
		buq := db.Builder().BatchUpdate("users", "id").WithContext(ctx)
		assert.NotNil(t, buq)
	})

	t.Run("Set", func(t *testing.T) {
		buq := db.Builder().BatchUpdate("users", "id").
			Set(1, map[string]interface{}{"status": 2})
		assert.NotNil(t, buq)
	})

	t.Run("Build", func(t *testing.T) {
		q := db.Builder().BatchUpdate("users", "id").
			Set(1, map[string]interface{}{"status": 2}).
			Build()
		assert.NotNil(t, q)
	})

	t.Run("Execute", func(t *testing.T) {
		_, err := db.Builder().BatchUpdate("users", "id").
			Set(1, map[string]interface{}{"status": 2}).
			Set(2, map[string]interface{}{"status": 3}).
			Execute()
		assert.NoError(t, err)
	})
}

// TestQuery_Wrapper tests all Query wrapper methods.
func TestQuery_Wrapper(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")

	t.Run("Execute", func(t *testing.T) {
		q := db.Builder().Insert("users", map[string]interface{}{"name": "Alice"})
		result, err := q.Execute()
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("One", func(t *testing.T) {
		db.ExecContext(ctx, "INSERT INTO users (name) VALUES ('Bob')")

		type User struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}

		var user User
		q := db.Builder().Select("*").From("users").Where("name = ?", "Bob").Build()
		err := q.One(&user)
		require.NoError(t, err)
		assert.Equal(t, "Bob", user.Name)
	})

	t.Run("All", func(t *testing.T) {
		type User struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}

		var users []User
		q := db.Builder().Select("*").From("users").Build()
		err := q.All(&users)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(users), 1)
	})
}

// TestWrapper_TypeSafety verifies type safety at compile time.
func TestWrapper_TypeSafety(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	// These should compile without errors
	var _ *relica.DB = db
	var _ *relica.QueryBuilder = db.Builder()
	var _ *relica.SelectQuery = db.Builder().Select("*")
	var _ *relica.UpdateQuery = db.Builder().Update("users")
	var _ *relica.DeleteQuery = db.Builder().Delete("users")
	var _ *relica.UpsertQuery = db.Builder().Upsert("users", nil)
	var _ *relica.BatchInsertQuery = db.Builder().BatchInsert("users", nil)
	var _ *relica.BatchUpdateQuery = db.Builder().BatchUpdate("users", "id")

	ctx := context.Background()
	tx, _ := db.Begin(ctx)
	defer tx.Rollback()
	var _ *relica.Tx = tx
}

// TestWrapper_ErrorPropagation verifies errors bubble up correctly.
func TestWrapper_ErrorPropagation(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	t.Run("InvalidSQL", func(t *testing.T) {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "INVALID SQL")
		assert.Error(t, err)
	})

	t.Run("NonExistentTable", func(t *testing.T) {
		var results []struct{}
		err := db.Builder().Select("*").From("nonexistent").All(&results)
		assert.Error(t, err)
	})
}

// TestWrapper_ContextPropagation verifies context flows through wrappers.
func TestWrapper_ContextPropagation(t *testing.T) {
	db, _ := relica.Open("sqlite", ":memory:")
	defer db.Close()

	ctx := context.Background()
	db.ExecContext(ctx, "CREATE TABLE users (id INTEGER, name TEXT)")

	t.Run("DBWithContext", func(t *testing.T) {
		dbWithCtx := db.WithContext(ctx)
		assert.NotNil(t, dbWithCtx)
	})

	t.Run("QueryBuilderWithContext", func(t *testing.T) {
		qb := db.Builder().WithContext(ctx)
		assert.NotNil(t, qb)
	})

	t.Run("SelectQueryWithContext", func(t *testing.T) {
		sq := db.Builder().Select("*").From("users").WithContext(ctx)
		assert.NotNil(t, sq)
	})
}

// TestWrapper_NilSafety ensures graceful handling of nil values.
func TestWrapper_NilSafety(t *testing.T) {
	t.Run("WrapNilDB", func(t *testing.T) {
		// WrapDB with nil should not panic (it will fail on use, not construction)
		db := relica.WrapDB(nil, "sqlite")
		assert.NotNil(t, db)
	})
}
