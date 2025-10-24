package benchmark

import (
	"context"
	"testing"

	"github.com/coregx/relica/internal/core"
	_ "modernc.org/sqlite"
)

type BenchItem struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

func BenchmarkSelectQuery(b *testing.B) {
	db, _ := core.NewDB("sqlite", ":memory:")
	defer db.Close()

	_, _ = db.ExecContext(context.Background(), `
        CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)
    `)

	// Insert test data
	_, _ = db.ExecContext(context.Background(), `
        INSERT INTO items (id, name) VALUES (1, 'test')
    `)

	b.Run("SimpleSelect", func(b *testing.B) {
		var items []BenchItem
		for i := 0; i < b.N; i++ {
			_ = db.Builder().Select("id", "name").From("items").All(&items)
		}
	})

	b.Run("PreparedStatement", func(b *testing.B) {
		query := db.Builder().Select("id", "name").From("items")
		var items []BenchItem
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = query.All(&items)
		}
	})
}
