// Package main demonstrates the Transactional() helper function for automatic transaction management.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/coregx/relica"
	_ "modernc.org/sqlite"
)

type User struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

type Account struct {
	ID      int `db:"id"`
	UserID  int `db:"user_id"`
	Balance int `db:"balance"`
}

func (User) TableName() string    { return "users" }
func (Account) TableName() string { return "accounts" }

func main() {
	// Open database connection.
	db, err := relica.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Setup schema.
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE TABLE accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			balance INTEGER NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Example 1: Basic Transactional - auto commit on success.
	fmt.Println("Example 1: Successful transaction")
	err = db.Transactional(ctx, func(tx *relica.Tx) error {
		user := User{Name: "Alice", Email: "alice@example.com"}
		if err := tx.Model(&user).Insert(); err != nil {
			return err // Auto rollback on error
		}

		account := Account{UserID: user.ID, Balance: 100}
		if err := tx.Model(&account).Insert(); err != nil {
			return err // Auto rollback on error
		}

		fmt.Printf("Created user %s with account balance %d\n", user.Name, account.Balance)
		return nil // Auto commit on success
	})
	if err != nil {
		log.Fatal(err)
	}

	// Example 2: Auto rollback on error.
	fmt.Println("\nExample 2: Transaction rollback on error")
	err = db.Transactional(ctx, func(tx *relica.Tx) error {
		user := User{Name: "Bob", Email: "bob@example.com"}
		if err := tx.Model(&user).Insert(); err != nil {
			return err
		}

		// Simulate error - negative balance not allowed.
		account := Account{UserID: user.ID, Balance: -50}
		if account.Balance < 0 {
			return fmt.Errorf("negative balance not allowed")
		}

		return tx.Model(&account).Insert()
	})
	if err != nil {
		fmt.Printf("Transaction rolled back: %v\n", err)
	}

	// Example 3: Panic recovery.
	fmt.Println("\nExample 3: Panic recovery and rollback")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Panic recovered: %v\n", r)
			}
		}()

		_ = db.Transactional(ctx, func(tx *relica.Tx) error {
			user := User{Name: "Charlie", Email: "charlie@example.com"}
			if err := tx.Model(&user).Insert(); err != nil {
				return err
			}

			// Simulate panic.
			panic("unexpected error")
		})
	}()

	// Example 4: Custom transaction options.
	fmt.Println("\nExample 4: Transaction with custom isolation level")
	err = db.TransactionalTx(ctx, &relica.TxOptions{
		Isolation: 0, // SQLite doesn't support isolation levels, use default
		ReadOnly:  false,
	}, func(tx *relica.Tx) error {
		user := User{Name: "Diana", Email: "diana@example.com"}
		if err := tx.Model(&user).Insert(); err != nil {
			return err
		}

		account := Account{UserID: user.ID, Balance: 200}
		if err := tx.Model(&account).Insert(); err != nil {
			return err
		}

		fmt.Printf("Created user %s with serializable transaction\n", user.Name)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	// Verify final state.
	fmt.Println("\nFinal database state:")
	var users []User
	err = db.Select("*").From("users").All(&users)
	if err != nil {
		log.Fatal(err)
	}

	for _, user := range users {
		var account Account
		err = db.Select("*").From("accounts").Where("user_id = ?", user.ID).One(&account)
		if err != nil {
			fmt.Printf("User: %s (%s) - no account\n", user.Name, user.Email)
		} else {
			fmt.Printf("User: %s (%s) - Balance: %d\n", user.Name, user.Email, account.Balance)
		}
	}
}
