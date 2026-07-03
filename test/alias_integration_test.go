//go:build integration
// +build integration

// Package test contains integration tests for the Relica query builder.
// This file validates table-alias quoting in expression generation (PR #22 / fix branch).
// All expressions that reference "alias.column" must produce correctly quoted SQL
// regardless of whether the column name is a reserved word.
package test

import (
	"testing"

	"github.com/coregx/relica"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// companyRow is the scan target for test_companies rows.
type companyRow struct {
	ID        int     `db:"id"`
	Name      string  `db:"name"`
	Status    string  `db:"status"`
	DeletedAt *string `db:"deleted_at"`
}

// employeeRow is the scan target for test_employees rows.
type employeeRow struct {
	ID        int    `db:"id"`
	CompanyID int    `db:"company_id"`
	Name      string `db:"name"`
	Role      string `db:"role"`
	Salary    int    `db:"salary"`
}

// joinRow is used for JOIN result scanning.
type joinRow struct {
	CompanyName  string `db:"company_name"`
	EmployeeName string `db:"employee_name"`
	Role         string `db:"role"`
	Salary       int    `db:"salary"`
}

// runAliasTests executes the full table-alias quoting test suite against the
// database wrapped by ds. Shared by the three dialect entry points below.
func runAliasTests(t *testing.T, ds *DatabaseSetup) {
	t.Helper()

	db := ds.DB

	SetupTestData(t, db, ds.Dialect)
	defer CleanupTestData(t, db)

	// ------------------------------------------------------------------ basic alias Eq
	t.Run("AliasedEq_ActiveCompanies", func(t *testing.T) {
		var rows []companyRow
		err := db.Select("c.id", "c.name", "c.status", "c.deleted_at").
			From("test_companies c").
			Where(relica.Eq("c.status", "active")).
			All(&rows)
		require.NoError(t, err, "aliased Eq must succeed")
		// seed: Acme Corp, Beta Ltd, Delta GmbH are active (3 of 5)
		assert.Len(t, rows, 3)
		for _, r := range rows {
			assert.Equal(t, "active", r.Status)
		}
	})

	// ------------------------------------------------------------------ aliased IS NULL
	t.Run("AliasedEq_NullDeletedAt", func(t *testing.T) {
		var rows []companyRow
		err := db.Select("c.id", "c.name", "c.status", "c.deleted_at").
			From("test_companies c").
			Where(relica.Eq("c.deleted_at", nil)).
			All(&rows)
		require.NoError(t, err, "aliased IS NULL must succeed")
		// seed: Acme Corp, Beta Ltd, Delta GmbH have NULL deleted_at
		assert.Len(t, rows, 3)
	})

	// ------------------------------------------------------------------ aliased IS NOT NULL
	t.Run("AliasedNotEq_NonNullDeletedAt", func(t *testing.T) {
		var rows []companyRow
		err := db.Select("c.id", "c.name", "c.status", "c.deleted_at").
			From("test_companies c").
			Where(relica.NotEq("c.deleted_at", nil)).
			All(&rows)
		require.NoError(t, err, "aliased IS NOT NULL must succeed")
		// seed: Gamma Inc, Epsilon LLC have deleted_at set
		assert.Len(t, rows, 2)
	})

	// ------------------------------------------------------------------ aliased LIKE
	t.Run("AliasedLike_CompanyName", func(t *testing.T) {
		var rows []companyRow
		err := db.Select("c.id", "c.name", "c.status", "c.deleted_at").
			From("test_companies c").
			Where(relica.Like("c.name", "Corp")).
			All(&rows)
		require.NoError(t, err, "aliased LIKE must succeed")
		assert.Len(t, rows, 1)
		assert.Equal(t, "Acme Corp", rows[0].Name)
	})

	// ------------------------------------------------------------------ aliased IN
	t.Run("AliasedIn_StatusValues", func(t *testing.T) {
		var rows []companyRow
		err := db.Select("c.id", "c.name", "c.status", "c.deleted_at").
			From("test_companies c").
			Where(relica.In("c.status", "active", "inactive")).
			All(&rows)
		require.NoError(t, err, "aliased IN must succeed")
		assert.Len(t, rows, 5) // all companies
	})

	// ------------------------------------------------------------------ aliased BETWEEN
	t.Run("AliasedBetween_Salary", func(t *testing.T) {
		var rows []employeeRow
		err := db.Select("e.id", "e.company_id", "e.name", "e.role", "e.salary").
			From("test_employees e").
			Where(relica.Between("e.salary", 85000, 95000)).
			All(&rows)
		require.NoError(t, err, "aliased BETWEEN must succeed")
		// seed: Alice=90000, Charlie=85000, Diana=95000, Grace=88000 → 4 rows
		assert.Len(t, rows, 4)
	})

	// ------------------------------------------------------------------ JOIN with aliased expressions
	t.Run("LeftJoin_ActiveCompanies_AliasedWhere", func(t *testing.T) {
		var rows []joinRow
		err := db.Select(
			"c.name as company_name",
			"e.name as employee_name",
			"e.role",
			"e.salary",
		).
			From("test_companies c").
			LeftJoin("test_employees e", "e.company_id = c.id").
			Where(relica.And(
				relica.Eq("c.status", "active"),
				relica.Eq("c.deleted_at", nil),
			)).
			OrderBy("c.id", "e.id").
			All(&rows)
		require.NoError(t, err, "JOIN with aliased AND must succeed")
		// seed: active companies with NULL deleted_at = Acme(2 emp), Beta(2 emp), Delta(3 emp) → 7 rows
		assert.Len(t, rows, 7)
	})

	t.Run("InnerJoin_HighSalary_AliasedGreaterThan", func(t *testing.T) {
		var rows []joinRow
		err := db.Select(
			"c.name as company_name",
			"e.name as employee_name",
			"e.role",
			"e.salary",
		).
			From("test_companies c").
			InnerJoin("test_employees e", "e.company_id = c.id").
			Where(relica.GreaterThan("e.salary", 100000)).
			All(&rows)
		require.NoError(t, err, "JOIN with aliased GreaterThan must succeed")
		// seed: Bob=110000, Frank=105000 → 2 rows
		assert.Len(t, rows, 2)
	})

	t.Run("Join_AliasedOr", func(t *testing.T) {
		var rows []joinRow
		err := db.Select(
			"c.name as company_name",
			"e.name as employee_name",
			"e.role",
			"e.salary",
		).
			From("test_companies c").
			InnerJoin("test_employees e", "e.company_id = c.id").
			Where(relica.Or(
				relica.Eq("e.role", "manager"),
				relica.Eq("e.role", "lead"),
			)).
			OrderBy("e.salary DESC").
			All(&rows)
		require.NoError(t, err, "JOIN with aliased OR must succeed")
		// seed: Bob(manager), Frank(manager), Diana(lead) → 3 rows
		assert.Len(t, rows, 3)
	})

	// ------------------------------------------------------------------ compound aliased expressions
	t.Run("AliasedCompound_And_Or_Null", func(t *testing.T) {
		// Active companies (not deleted) whose employees earn between 80k and 100k.
		type companyEmpRow struct {
			CompanyName  string `db:"company_name"`
			EmployeeName string `db:"employee_name"`
			Salary       int    `db:"salary"`
		}
		var rows []companyEmpRow
		err := db.Select(
			"c.name as company_name",
			"e.name as employee_name",
			"e.salary",
		).
			From("test_companies c").
			InnerJoin("test_employees e", "e.company_id = c.id").
			Where(relica.And(
				relica.Eq("c.status", "active"),
				relica.Eq("c.deleted_at", nil),
				relica.Between("e.salary", 80000, 100000),
			)).
			OrderBy("e.salary").
			All(&rows)
		require.NoError(t, err, "compound aliased AND with NULL check must succeed")
		// active, not-deleted companies: Acme(1,2), Beta(3,4), Delta(6,7,8)
		// salary 80k-100k: Alice=90000, Charlie=85000, Diana=95000, Frank→105000(excluded),
		//                   Grace=88000, Hank=45000(excluded), Bob=110000(excluded)
		// → Alice, Charlie, Diana, Grace = 4 rows
		assert.Len(t, rows, 4)
	})

	// ------------------------------------------------------------------ NotIn with alias
	t.Run("AliasedNotIn_ExcludeRoles", func(t *testing.T) {
		var rows []employeeRow
		err := db.Select("e.id", "e.company_id", "e.name", "e.role", "e.salary").
			From("test_employees e").
			Where(relica.NotIn("e.role", "intern", "lead")).
			All(&rows)
		require.NoError(t, err, "aliased NotIn must succeed")
		// seed: 8 employees, Hank=intern, Diana=lead → 6 remaining
		assert.Len(t, rows, 6)
	})

	// ------------------------------------------------------------------ NotLike with alias
	t.Run("AliasedNotLike_CompanyName", func(t *testing.T) {
		var rows []companyRow
		err := db.Select("c.id", "c.name", "c.status", "c.deleted_at").
			From("test_companies c").
			Where(relica.NotLike("c.name", "Inc")).
			All(&rows)
		require.NoError(t, err, "aliased NotLike must succeed")
		// seed: Gamma Inc excluded → 4 rows
		assert.Len(t, rows, 4)
	})

	// ------------------------------------------------------------------ GROUP BY + HAVING with alias
	t.Run("GroupBy_Having_AliasedExpression", func(t *testing.T) {
		type companySalary struct {
			CompanyID int `db:"company_id"`
			AvgSalary int `db:"avg_salary"`
		}
		castExpr := "CAST(AVG(e.salary) AS INTEGER) as avg_salary"
		if ds.Dialect == "mysql" {
			castExpr = "CAST(AVG(e.salary) AS SIGNED) as avg_salary"
		}
		var rows []companySalary
		err := db.Select("e.company_id", castExpr).
			From("test_employees e").
			GroupBy("e.company_id").
			Having("AVG(e.salary) > ?", 80000).
			OrderBy("e.company_id").
			All(&rows)
		require.NoError(t, err, "GROUP BY + HAVING with aliased column must succeed")
		// seed averages: company1=(90000+110000)/2=100000, company2=(85000+95000)/2=90000,
		//                company3=80000 (Eve only, NOT > 80000 strictly), company4=(105000+88000+45000)/3=79333
		// → company1 and company2 satisfy AVG > 80000 strictly → 2 rows
		assert.Len(t, rows, 2)
	})

	// ------------------------------------------------------------------ Distinct with alias
	t.Run("Distinct_AliasedColumn", func(t *testing.T) {
		type roleRow struct {
			Role string `db:"role"`
		}
		var rows []roleRow
		err := db.Select("e.role").
			From("test_employees e").
			Distinct().
			OrderBy("e.role").
			All(&rows)
		require.NoError(t, err, "DISTINCT with aliased column must succeed")
		// seed roles: engineer, intern, lead, manager → 4 distinct values
		assert.Len(t, rows, 4)
	})
}

// ============================================================
// Dialect entry points
// ============================================================

// TestTableAlias_SQLite verifies table-alias quoting on SQLite
// (in-memory, no Docker required).
func TestTableAlias_SQLite(t *testing.T) {
	ds := SetupSQLiteTestDB(t)
	defer ds.Close()
	runAliasTests(t, ds)
}

// TestTableAlias_PostgreSQL verifies table-alias quoting on PostgreSQL
// (testcontainers, skipped when Docker is unavailable).
func TestTableAlias_PostgreSQL(t *testing.T) {
	ds := SetupPostgreSQLTestDB(t)
	defer ds.Close()
	runAliasTests(t, ds)
}

// TestTableAlias_MySQL verifies table-alias quoting on MySQL
// (testcontainers, skipped when Docker is unavailable).
func TestTableAlias_MySQL(t *testing.T) {
	ds := SetupMySQLTestDB(t)
	defer ds.Close()
	runAliasTests(t, ds)
}
