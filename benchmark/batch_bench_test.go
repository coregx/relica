package benchmark

import (
	"context"
	"fmt"
	"testing"

	"github.com/coregx/relica/internal/core"
	_ "modernc.org/sqlite"
)

// setupBenchDB creates an in-memory SQLite database for benchmarking.
func setupBenchDB(b *testing.B) *core.DB {
	db, err := core.NewDB("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("Failed to open database: %v", err)
	}

	// Create test table
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			age INTEGER
		)
	`)
	if err != nil {
		b.Fatalf("Failed to create table: %v", err)
	}

	b.Cleanup(func() {
		db.Close()
	})

	return db
}

// BenchmarkBatchInsert_10rows benchmarks batch INSERT with 10 rows.
func BenchmarkBatchInsert_10rows(b *testing.B) {
	db := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
		for j := 0; j < 10; j++ {
			batch.Values(
				fmt.Sprintf("User %d", j),
				fmt.Sprintf("user%d@example.com", j),
				20+j,
			)
		}
		_, err := batch.Execute()
		if err != nil {
			b.Fatalf("Batch insert failed: %v", err)
		}

		// Clean up for next iteration
		db.ExecContext(context.Background(), "DELETE FROM users")
	}
}

// BenchmarkBatchInsert_100rows benchmarks batch INSERT with 100 rows.
func BenchmarkBatchInsert_100rows(b *testing.B) {
	db := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
		for j := 0; j < 100; j++ {
			batch.Values(
				fmt.Sprintf("User %d", j),
				fmt.Sprintf("user%d@example.com", j),
				20+j,
			)
		}
		_, err := batch.Execute()
		if err != nil {
			b.Fatalf("Batch insert failed: %v", err)
		}

		// Clean up for next iteration
		db.ExecContext(context.Background(), "DELETE FROM users")
	}
}

// BenchmarkBatchInsert_1000rows benchmarks batch INSERT with 1000 rows.
func BenchmarkBatchInsert_1000rows(b *testing.B) {
	db := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
		for j := 0; j < 1000; j++ {
			batch.Values(
				fmt.Sprintf("User %d", j),
				fmt.Sprintf("user%d@example.com", j),
				20+j,
			)
		}
		_, err := batch.Execute()
		if err != nil {
			b.Fatalf("Batch insert failed: %v", err)
		}

		// Clean up for next iteration
		db.ExecContext(context.Background(), "DELETE FROM users")
	}
}

// BenchmarkSingleInsert_100rows benchmarks individual INSERTs for comparison.
func BenchmarkSingleInsert_100rows(b *testing.B) {
	db := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			_, err := db.Builder().Insert("users", map[string]interface{}{
				"name":  fmt.Sprintf("User %d", j),
				"email": fmt.Sprintf("user%d@example.com", j),
				"age":   20 + j,
			}).Execute()
			if err != nil {
				b.Fatalf("Insert failed: %v", err)
			}
		}

		// Clean up for next iteration
		db.ExecContext(context.Background(), "DELETE FROM users")
	}
}

// BenchmarkSingleInsert_1000rows benchmarks individual INSERTs for 1000 rows.
func BenchmarkSingleInsert_1000rows(b *testing.B) {
	db := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 1000; j++ {
			_, err := db.Builder().Insert("users", map[string]interface{}{
				"name":  fmt.Sprintf("User %d", j),
				"email": fmt.Sprintf("user%d@example.com", j),
				"age":   20 + j,
			}).Execute()
			if err != nil {
				b.Fatalf("Insert failed: %v", err)
			}
		}

		// Clean up for next iteration
		db.ExecContext(context.Background(), "DELETE FROM users")
	}
}

// BenchmarkBatchUpdate_10rows benchmarks batch UPDATE with 10 rows.
func BenchmarkBatchUpdate_10rows(b *testing.B) {
	db := setupBenchDB(b)

	// Insert initial data
	batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
	for j := 0; j < 10; j++ {
		batch.Values(
			fmt.Sprintf("User %d", j),
			fmt.Sprintf("user%d@example.com", j),
			20+j,
		)
	}
	batch.Execute()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updateBatch := db.Builder().BatchUpdate("users", "id")
		for j := 1; j <= 10; j++ {
			updateBatch.Set(j, map[string]interface{}{
				"name":  fmt.Sprintf("Updated %d", j),
				"email": fmt.Sprintf("updated%d@example.com", j),
			})
		}
		_, err := updateBatch.Execute()
		if err != nil {
			b.Fatalf("Batch update failed: %v", err)
		}
	}
}

// BenchmarkBatchUpdate_100rows benchmarks batch UPDATE with 100 rows.
func BenchmarkBatchUpdate_100rows(b *testing.B) {
	db := setupBenchDB(b)

	// Insert initial data
	batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
	for j := 0; j < 100; j++ {
		batch.Values(
			fmt.Sprintf("User %d", j),
			fmt.Sprintf("user%d@example.com", j),
			20+j,
		)
	}
	batch.Execute()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updateBatch := db.Builder().BatchUpdate("users", "id")
		for j := 1; j <= 100; j++ {
			updateBatch.Set(j, map[string]interface{}{
				"name":  fmt.Sprintf("Updated %d", j),
				"email": fmt.Sprintf("updated%d@example.com", j),
			})
		}
		_, err := updateBatch.Execute()
		if err != nil {
			b.Fatalf("Batch update failed: %v", err)
		}
	}
}

// BenchmarkSingleUpdate_100rows benchmarks individual UPDATEs for comparison.
func BenchmarkSingleUpdate_100rows(b *testing.B) {
	db := setupBenchDB(b)

	// Insert initial data
	batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
	for j := 0; j < 100; j++ {
		batch.Values(
			fmt.Sprintf("User %d", j),
			fmt.Sprintf("user%d@example.com", j),
			20+j,
		)
	}
	batch.Execute()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 1; j <= 100; j++ {
			_, err := db.Builder().
				Update("users").
				Set(map[string]interface{}{
					"name":  fmt.Sprintf("Updated %d", j),
					"email": fmt.Sprintf("updated%d@example.com", j),
				}).
				Where("id = ?", j).
				Execute()
			if err != nil {
				b.Fatalf("Update failed: %v", err)
			}
		}
	}
}

// BenchmarkBatchInsert_ValuesMap benchmarks ValuesMap vs Values performance.
func BenchmarkBatchInsert_ValuesMap_100rows(b *testing.B) {
	db := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
		for j := 0; j < 100; j++ {
			batch.ValuesMap(map[string]interface{}{
				"name":  fmt.Sprintf("User %d", j),
				"email": fmt.Sprintf("user%d@example.com", j),
				"age":   20 + j,
			})
		}
		_, err := batch.Execute()
		if err != nil {
			b.Fatalf("Batch insert failed: %v", err)
		}

		// Clean up for next iteration
		db.ExecContext(context.Background(), "DELETE FROM users")
	}
}

// BenchmarkBatchInsert_Values benchmarks direct Values() method.
func BenchmarkBatchInsert_Values_100rows(b *testing.B) {
	db := setupBenchDB(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
		for j := 0; j < 100; j++ {
			batch.Values(
				fmt.Sprintf("User %d", j),
				fmt.Sprintf("user%d@example.com", j),
				20+j,
			)
		}
		_, err := batch.Execute()
		if err != nil {
			b.Fatalf("Batch insert failed: %v", err)
		}

		// Clean up for next iteration
		db.ExecContext(context.Background(), "DELETE FROM users")
	}
}

// BenchmarkBatchUpdate_DifferentColumns benchmarks partial column updates.
func BenchmarkBatchUpdate_DifferentColumns_100rows(b *testing.B) {
	db := setupBenchDB(b)

	// Insert initial data
	batch := db.Builder().BatchInsert("users", []string{"name", "email", "age"})
	for j := 0; j < 100; j++ {
		batch.Values(
			fmt.Sprintf("User %d", j),
			fmt.Sprintf("user%d@example.com", j),
			20+j,
		)
	}
	batch.Execute()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updateBatch := db.Builder().BatchUpdate("users", "id")
		for j := 1; j <= 100; j++ {
			// Update different columns for different rows
			switch j % 3 {
			case 0:
				updateBatch.Set(j, map[string]interface{}{"name": fmt.Sprintf("Updated %d", j)})
			case 1:
				updateBatch.Set(j, map[string]interface{}{"email": fmt.Sprintf("new%d@example.com", j)})
			default:
				updateBatch.Set(j, map[string]interface{}{"age": 30 + j})
			}
		}
		_, err := updateBatch.Execute()
		if err != nil {
			b.Fatalf("Batch update failed: %v", err)
		}
	}
}
