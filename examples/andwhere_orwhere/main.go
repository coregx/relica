// Package main demonstrates AndWhere() and OrWhere() methods for dynamic query building.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/coregx/relica"
	_ "modernc.org/sqlite"
)

func main() {
	// Open SQLite in-memory database.
	db, err := relica.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create users table using ExecContext.
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT,
			age INTEGER,
			status INTEGER,
			role TEXT
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Insert test data.
	users := []map[string]interface{}{
		{"id": 1, "name": "Alice", "age": 25, "status": 1, "role": "user"},
		{"id": 2, "name": "Bob", "age": 17, "status": 1, "role": "user"},
		{"id": 3, "name": "Charlie", "age": 30, "status": 0, "role": "user"},
		{"id": 4, "name": "Diana", "age": 22, "status": 1, "role": "admin"},
		{"id": 5, "name": "Eve", "age": 35, "status": 1, "role": "moderator"},
	}

	for _, user := range users {
		_, err = db.Builder().Insert("users", user).Execute()
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("=== AndWhere Example ===")
	// Find active users older than 18.
	type User struct {
		ID     int    `db:"id"`
		Name   string `db:"name"`
		Age    int    `db:"age"`
		Status int    `db:"status"`
		Role   string `db:"role"`
	}

	var result1 []User
	err = db.Builder().
		Select("*").
		From("users").
		Where("status = ?", 1).
		AndWhere("age > ?", 18).
		All(&result1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Active users older than 18:\n")
	for _, u := range result1 {
		fmt.Printf("  - %s (age: %d, role: %s)\n", u.Name, u.Age, u.Role)
	}

	fmt.Println("\n=== OrWhere Example ===")
	// Find active users older than 18 OR admins (regardless of age).
	var result2 []User
	err = db.Builder().
		Select("*").
		From("users").
		Where("status = ?", 1).
		AndWhere("age > ?", 18).
		OrWhere("role = ?", "admin").
		All(&result2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Active users older than 18 OR admins:\n")
	for _, u := range result2 {
		fmt.Printf("  - %s (age: %d, role: %s)\n", u.Name, u.Age, u.Role)
	}

	fmt.Println("\n=== Dynamic Query Building ===")
	// Build query dynamically based on filters.
	name := "Alice"
	minAge := 20
	roleFilter := "admin"

	query := db.Builder().Select("*").From("users").Where("status = ?", 1)

	if name != "" {
		query = query.AndWhere(relica.Like("name", name))
	}
	if minAge > 0 {
		query = query.AndWhere(relica.GreaterThan("age", minAge))
	}
	if roleFilter != "" {
		query = query.OrWhere(relica.Eq("role", roleFilter))
	}

	var result3 []User
	err = query.All(&result3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Dynamic query results:\n")
	for _, u := range result3 {
		fmt.Printf("  - %s (age: %d, role: %s)\n", u.Name, u.Age, u.Role)
	}

	fmt.Println("\n=== UPDATE with AndWhere/OrWhere ===")
	// Update status for users who are either inactive OR too young.
	result, err := db.Builder().
		Update("users").
		Set(map[string]interface{}{"status": 0}).
		Where("status = ?", 0).
		OrWhere("age < ?", 18).
		Execute()
	if err != nil {
		log.Fatal(err)
	}
	if res, ok := result.(sql.Result); ok {
		affected, _ := res.RowsAffected()
		fmt.Printf("Updated %d users\n", affected)
	}

	fmt.Println("\n=== DELETE with AndWhere/OrWhere ===")
	// Delete inactive users OR users with no role.
	result, err = db.Builder().
		Delete("users").
		Where("status = ?", 0).
		OrWhere("role = ?", "").
		Execute()
	if err != nil {
		log.Fatal(err)
	}
	if res, ok := result.(sql.Result); ok {
		affected, _ := res.RowsAffected()
		fmt.Printf("Deleted %d users\n", affected)
	}
}
