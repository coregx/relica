package core

import (
	"fmt"
	"testing"
)

// TestDistinct_Demo demonstrates Distinct() usage with SQL output.
func TestDistinct_Demo(t *testing.T) {
	db := mockDB("postgres")
	qb := &QueryBuilder{db: db}

	fmt.Println("\n=== Distinct() Feature Demo ===")

	// Example 1: Basic DISTINCT usage.
	fmt.Println("\n1. Basic DISTINCT:")
	q1 := qb.Select("category").
		From("products").
		Distinct(true).
		Build()
	fmt.Printf("   SQL: %s\n", q1.sql)
	fmt.Printf("   Expected: SELECT DISTINCT \"category\" FROM \"products\"\n")

	// Example 2: DISTINCT with multiple columns.
	fmt.Println("\n2. DISTINCT with multiple columns:")
	q2 := qb.Select("country", "city").
		From("locations").
		Distinct(true).
		Build()
	fmt.Printf("   SQL: %s\n", q2.sql)
	fmt.Printf("   Expected: SELECT DISTINCT \"country\", \"city\" FROM \"locations\"\n")

	// Example 3: DISTINCT with WHERE.
	fmt.Println("\n3. DISTINCT with WHERE:")
	q3 := qb.Select("status").
		From("orders").
		Where("total > ?", 100).
		Distinct(true).
		Build()
	fmt.Printf("   SQL: %s\n", q3.sql)
	fmt.Printf("   Params: %v\n", q3.params)
	fmt.Printf("   Expected: SELECT DISTINCT \"status\" FROM \"orders\" WHERE total > $1\n")

	// Example 4: DISTINCT with JOIN, WHERE, ORDER BY, LIMIT.
	fmt.Println("\n4. Complex query with DISTINCT:")
	q4 := qb.Select("u.country").
		From("users u").
		InnerJoin("orders o", "o.user_id = u.id").
		Where("o.status = ?", "completed").
		Distinct(true).
		OrderBy("u.country ASC").
		Limit(10).
		Build()
	fmt.Printf("   SQL: %s\n", q4.sql)
	fmt.Printf("   Params: %v\n", q4.params)

	// Example 5: Without DISTINCT (default).
	fmt.Println("\n5. Without DISTINCT (default):")
	q5 := qb.Select("category").
		From("products").
		Build()
	fmt.Printf("   SQL: %s\n", q5.sql)
	fmt.Printf("   Expected: SELECT \"category\" FROM \"products\"\n")

	// Example 6: Explicitly disable DISTINCT.
	fmt.Println("\n6. Explicitly disable DISTINCT:")
	q6 := qb.Select("name").
		From("users").
		Distinct(false).
		Build()
	fmt.Printf("   SQL: %s\n", q6.sql)
	fmt.Printf("   Expected: SELECT \"name\" FROM \"users\"\n")

	// Example 7: Toggle DISTINCT (last call wins).
	fmt.Println("\n7. Toggle DISTINCT (last call wins):")
	q7 := qb.Select("role").
		From("users").
		Distinct(true).
		Distinct(false).
		Build()
	fmt.Printf("   SQL: %s\n", q7.sql)
	fmt.Printf("   Note: DISTINCT(false) overrides DISTINCT(true)\n")

	fmt.Println("\n=== Demo Complete ===")
}
