//go:build integration
// +build integration

// Package test contains integration tests for the Relica query builder.
// This file validates the Model API (Insert, Update, Delete, Upsert, UpdateChanged)
// against real databases, verifying that the P0/P2 enterprise audit fixes (PRs #23-#27)
// work correctly when operating through the struct-based CRUD layer.
package test

import (
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Product is the test model for test_products.
// TableName() implements the optional interface so Relica uses the correct table.
type Product struct {
	ID       int    `db:"id,pk"`
	SKU      string `db:"sku"`
	Name     string `db:"name"`
	Price    int    `db:"price"`
	Category string `db:"category"`
}

// TableName returns the table name for the Product model.
func (Product) TableName() string { return "test_products" }

// runModelAPITests executes all Model API integration tests against ds.
// Shared by the three dialect entry points below.
func runModelAPITests(t *testing.T, ds *DatabaseSetup) {
	t.Helper()

	db := ds.DB

	SetupTestData(t, db, ds.Dialect)
	defer CleanupTestData(t, db)

	// ------------------------------------------------------------------ Insert
	t.Run("Model_Insert", func(t *testing.T) {
		p := Product{
			SKU:      "SKU-NEW-001",
			Name:     "New Widget",
			Price:    1500,
			Category: "widgets",
		}

		err := db.Model(&p).Insert()
		require.NoError(t, err, "Model Insert must succeed")
		// After Insert the primary key should be populated.
		assert.Greater(t, p.ID, 0, "ID must be populated after Insert")

		// Verify the row exists in the database.
		var fetched Product
		err = db.Select().
			From("test_products").
			Where(relica.Eq("sku", "SKU-NEW-001")).
			One(&fetched)
		require.NoError(t, err)
		assert.Equal(t, "New Widget", fetched.Name)
		assert.Equal(t, 1500, fetched.Price)
		assert.Equal(t, "widgets", fetched.Category)

		// Cleanup: delete the inserted row to avoid leaking state into later sub-tests.
		_, err = db.Delete("test_products").Where(relica.Eq("sku", "SKU-NEW-001")).Execute()
		require.NoError(t, err)
	})

	// ------------------------------------------------------------------ Update
	t.Run("Model_Update", func(t *testing.T) {
		// Fetch an existing product from seed data.
		var p Product
		err := db.Select().
			From("test_products").
			Where(relica.Eq("sku", "SKU-001")).
			One(&p)
		require.NoError(t, err, "fetch seed product for Update test")
		require.Greater(t, p.ID, 0)

		originalPrice := p.Price
		p.Price = originalPrice + 100
		p.Name = "Widget A Updated"

		err = db.Model(&p).Update()
		require.NoError(t, err, "Model Update must succeed")

		// Verify in DB.
		var updated Product
		err = db.Select().
			From("test_products").
			Where(relica.Eq("id", p.ID)).
			One(&updated)
		require.NoError(t, err)
		assert.Equal(t, originalPrice+100, updated.Price)
		assert.Equal(t, "Widget A Updated", updated.Name)

		// Restore seed value.
		p.Price = originalPrice
		p.Name = "Widget A"
		require.NoError(t, db.Model(&p).Update())
	})

	// ------------------------------------------------------------------ UpdateChanged
	t.Run("Model_UpdateChanged", func(t *testing.T) {
		var p Product
		err := db.Select().
			From("test_products").
			Where(relica.Eq("sku", "SKU-002")).
			One(&p)
		require.NoError(t, err)

		// Snapshot before modification.
		original := p

		// Modify only the price.
		p.Price = 2500

		err = db.Model(&p).UpdateChanged(&original)
		require.NoError(t, err, "Model UpdateChanged must succeed")

		// Verify only price changed.
		var after Product
		err = db.Select().
			From("test_products").
			Where(relica.Eq("id", p.ID)).
			One(&after)
		require.NoError(t, err)
		assert.Equal(t, 2500, after.Price)
		assert.Equal(t, original.Name, after.Name, "name must be unchanged")

		// Restore.
		p.Price = original.Price
		require.NoError(t, db.Model(&p).Update())
	})

	// ------------------------------------------------------------------ UpdateChanged no-op
	t.Run("Model_UpdateChanged_NoOp_WhenUnchanged", func(t *testing.T) {
		var p Product
		err := db.Select().
			From("test_products").
			Where(relica.Eq("sku", "SKU-003")).
			One(&p)
		require.NoError(t, err)

		original := p // identical snapshot

		// UpdateChanged with no differences must not error (no query executed).
		err = db.Model(&p).UpdateChanged(&original)
		require.NoError(t, err, "UpdateChanged with no changes must return nil without error")
	})

	// ------------------------------------------------------------------ Delete
	t.Run("Model_Delete", func(t *testing.T) {
		// Insert a disposable product.
		toDelete := Product{
			SKU:      "SKU-DELETE-TEST",
			Name:     "Delete Me",
			Price:    1,
			Category: "misc",
		}
		err := db.Model(&toDelete).Insert()
		require.NoError(t, err)
		require.Greater(t, toDelete.ID, 0)

		err = db.Model(&toDelete).Delete()
		require.NoError(t, err, "Model Delete must succeed")

		// Verify it is gone.
		var count struct {
			N int `db:"n"`
		}
		err = db.Select("COUNT(*) as n").
			From("test_products").
			Where(relica.Eq("id", toDelete.ID)).
			One(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count.N)
	})

	// ------------------------------------------------------------------ Upsert (Model API)
	t.Run("Model_Upsert_Insert", func(t *testing.T) {
		// Insert a brand-new product via Model Upsert.
		newProduct := Product{
			SKU:      "SKU-UPSERT-NEW",
			Name:     "Upsert New Product",
			Price:    3000,
			Category: "gadgets",
		}

		err := db.Model(&newProduct).Upsert()
		require.NoError(t, err, "Model Upsert (insert path) must succeed")

		var fetched Product
		err = db.Select().
			From("test_products").
			Where(relica.Eq("sku", "SKU-UPSERT-NEW")).
			One(&fetched)
		require.NoError(t, err)
		assert.Equal(t, "Upsert New Product", fetched.Name)

		// Cleanup.
		_, err = db.Delete("test_products").Where(relica.Eq("sku", "SKU-UPSERT-NEW")).Execute()
		require.NoError(t, err)
	})

	// ------------------------------------------------------------------ Exclude
	t.Run("Model_Insert_Exclude", func(t *testing.T) {
		// Insert excluding "category" — it should receive the DB default ('general').
		p := Product{
			SKU:      "SKU-EXCL-001",
			Name:     "Excluded Category",
			Price:    750,
			Category: "should_be_ignored",
		}

		err := db.Model(&p).Exclude("category").Insert()
		require.NoError(t, err, "Model Insert with Exclude must succeed")
		require.Greater(t, p.ID, 0)

		var fetched Product
		err = db.Select().
			From("test_products").
			Where(relica.Eq("id", p.ID)).
			One(&fetched)
		require.NoError(t, err)
		// When "category" is excluded on INSERT, the DB default kicks in.
		assert.Equal(t, "general", fetched.Category)

		// Cleanup.
		_, err = db.Delete("test_products").Where(relica.Eq("id", p.ID)).Execute()
		require.NoError(t, err)
	})

	// ------------------------------------------------------------------ Table override
	t.Run("Model_Table_Override", func(t *testing.T) {
		// Model.Table() allows overriding the table name used for the operation.
		// We insert using the default TableName(), then verify the row with a Select.
		p := Product{
			SKU:      "SKU-TABLE-OVERRIDE",
			Name:     "Table Override Product",
			Price:    500,
			Category: "misc",
		}

		err := db.Model(&p).Table("test_products").Insert()
		require.NoError(t, err, "Model Insert with Table override must succeed")
		require.Greater(t, p.ID, 0)

		var fetched Product
		err = db.Select().
			From("test_products").
			Where(relica.Eq("sku", "SKU-TABLE-OVERRIDE")).
			One(&fetched)
		require.NoError(t, err)
		assert.Equal(t, "Table Override Product", fetched.Name)

		// Cleanup.
		_, err = db.Delete("test_products").Where(relica.Eq("sku", "SKU-TABLE-OVERRIDE")).Execute()
		require.NoError(t, err)
	})
}

// ============================================================
// Dialect entry points
// ============================================================

// TestModelAPI_SQLite verifies the Model API on SQLite
// (in-memory, no Docker required).
func TestModelAPI_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runModelAPITests(t, ds)
}

// TestModelAPI_PostgreSQL verifies the Model API on PostgreSQL
// (testcontainers, skipped when Docker is unavailable).
func TestModelAPI_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runModelAPITests(t, ds)
}

// TestModelAPI_MySQL verifies the Model API on MySQL
// (testcontainers, skipped when Docker is unavailable).
func TestModelAPI_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runModelAPITests(t, ds)
}
