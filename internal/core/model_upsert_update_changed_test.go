package core

import (
	"reflect"
	"testing"
	"time"

	"github.com/coregx/relica/internal/dialects"
	"github.com/coregx/relica/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test models
// ============================================================================

// upsertUser is a test model for upsert SQL generation tests.
type upsertUser struct {
	ID     int    `db:"id"`
	Name   string `db:"name"`
	Email  string `db:"email"`
	Status string `db:"status"`
}

func (upsertUser) TableName() string { return "users" }

// upsertPost uses explicit pk tag.
type upsertPost struct {
	PostID  int    `db:"post_id,pk"`
	Content string `db:"content"`
	Views   int    `db:"views"`
}

func (upsertPost) TableName() string { return "posts" }

// diffUser for UpdateChanged tests.
type diffUser struct {
	ID     int    `db:"id"`
	Name   string `db:"name"`
	Email  string `db:"email"`
	Status string `db:"status"`
}

// diffUserWithTime includes a time field for deep equality testing.
type diffUserWithTime struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

// noIDModelUpsert has no primary key for error path tests.
type noIDModelUpsert struct {
	Name string `db:"name"`
}

func (noIDModelUpsert) TableName() string { return "things" }

// ============================================================================
// Helpers
// ============================================================================

// upsertMockDB creates a minimal DB for SQL generation testing (dialect only).
func upsertMockDB(dialectName string) *DB {
	return &DB{
		dialect:    dialects.GetDialect(dialectName),
		driverName: dialectName,
	}
}

// newTestMQ creates a ModelQuery for use in unit tests without a real DB.
func newTestMQ(db *DB, model interface{}, table string) *ModelQuery {
	return &ModelQuery{
		db:      db,
		model:   model,
		table:   table,
		exclude: make(map[string]bool),
	}
}

// ============================================================================
// buildUpsertUpdateCols unit tests
// ============================================================================

func TestBuildUpsertUpdateCols_NoFieldsSpecified(t *testing.T) {
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"id": 1, "name": "Alice", "email": "a@b.com"}
	pkCols := []string{"id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	assert.ElementsMatch(t, []string{"name", "email"}, result)
	for _, col := range result {
		assert.NotEqual(t, "id", col, "PK should not be in update cols")
	}
}

func TestBuildUpsertUpdateCols_SpecificFieldsWithPK(t *testing.T) {
	// If caller includes the PK in fields, it must be excluded.
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"id": 1, "name": "Alice", "email": "a@b.com"}
	pkCols := []string{"id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, []string{"name", "id"})

	assert.Equal(t, []string{"name"}, result)
}

func TestBuildUpsertUpdateCols_SingleSpecificField(t *testing.T) {
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"id": 1, "name": "Alice", "email": "a@b.com"}
	pkCols := []string{"id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, []string{"email"})

	assert.Equal(t, []string{"email"}, result)
}

func TestBuildUpsertUpdateCols_CompositePK(t *testing.T) {
	mq := &ModelQuery{exclude: make(map[string]bool)}
	dataMap := map[string]interface{}{"order_id": 1, "product_id": 2, "qty": 5}
	pkCols := []string{"order_id", "product_id"}

	result := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	assert.Equal(t, []string{"qty"}, result)
}

// ============================================================================
// Upsert SQL generation tests (no real DB, only SQL verification)
// ============================================================================

func TestModelUpsert_SQL_PostgreSQL_AllFields(t *testing.T) {
	db := upsertMockDB("postgres")
	user := upsertUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}

	dataMap, err := util.StructToMap(&user)
	require.NoError(t, err)

	pkCols := []string{"id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, `INSERT INTO "users"`)
	assert.Contains(t, q.sql, "ON CONFLICT (id)")
	assert.Contains(t, q.sql, "DO UPDATE SET")
	assert.Contains(t, q.sql, "name = EXCLUDED.name")
	assert.Contains(t, q.sql, "email = EXCLUDED.email")
	assert.Contains(t, q.sql, "status = EXCLUDED.status")
	assert.NotContains(t, q.sql, "id = EXCLUDED.id")
}

func TestModelUpsert_SQL_MySQL_AllFields(t *testing.T) {
	db := upsertMockDB("mysql")
	user := upsertUser{ID: 2, Name: "Bob", Email: "bob@example.com", Status: "pending"}

	dataMap, err := util.StructToMap(&user)
	require.NoError(t, err)

	pkCols := []string{"id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, "INSERT INTO `users`")
	assert.Contains(t, q.sql, "ON DUPLICATE KEY UPDATE")
	assert.Contains(t, q.sql, "name = VALUES(name)")
	assert.Contains(t, q.sql, "email = VALUES(email)")
	assert.Contains(t, q.sql, "status = VALUES(status)")
}

func TestModelUpsert_SQL_SQLite_AllFields(t *testing.T) {
	db := upsertMockDB("sqlite")
	user := upsertUser{ID: 3, Name: "Carol", Email: "carol@example.com", Status: "active"}

	dataMap, err := util.StructToMap(&user)
	require.NoError(t, err)

	pkCols := []string{"id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, `INSERT INTO "users"`)
	assert.Contains(t, q.sql, "ON CONFLICT (id)")
	assert.Contains(t, q.sql, "DO UPDATE SET")
	assert.Contains(t, q.sql, "name = excluded.name")
}

func TestModelUpsert_SQL_SelectiveFields(t *testing.T) {
	tests := []struct {
		name        string
		dialectName string
		updateCol   string
		expectSQL   string
		notExpect   []string
	}{
		{
			name:        "postgres selective",
			dialectName: "postgres",
			updateCol:   "name = EXCLUDED.name",
			expectSQL:   "DO UPDATE SET",
			notExpect:   []string{"email = EXCLUDED.email", "status = EXCLUDED.status"},
		},
		{
			name:        "mysql selective",
			dialectName: "mysql",
			updateCol:   "name = VALUES(name)",
			expectSQL:   "ON DUPLICATE KEY UPDATE",
			notExpect:   []string{"email = VALUES(email)", "status = VALUES(status)"},
		},
		{
			name:        "sqlite selective",
			dialectName: "sqlite",
			updateCol:   "name = excluded.name",
			expectSQL:   "DO UPDATE SET",
			notExpect:   []string{"email = excluded.email", "status = excluded.status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := upsertMockDB(tt.dialectName)
			user := upsertUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}

			dataMap, err := util.StructToMap(&user)
			require.NoError(t, err)

			pkCols := []string{"id"}
			mq := &ModelQuery{exclude: make(map[string]bool)}
			// Only "name" on conflict.
			updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, []string{"name"})

			qb := &QueryBuilder{db: db}
			q := qb.Upsert("users", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

			require.NotNil(t, q)
			assert.Contains(t, q.sql, tt.updateCol)
			assert.Contains(t, q.sql, tt.expectSQL)
			for _, ne := range tt.notExpect {
				assert.NotContains(t, q.sql, ne)
			}
		})
	}
}

func TestModelUpsert_SQL_ExplicitPKTag(t *testing.T) {
	db := upsertMockDB("postgres")
	post := upsertPost{PostID: 10, Content: "Hello", Views: 5}

	dataMap, err := util.StructToMap(&post)
	require.NoError(t, err)

	pkCols := []string{"post_id"}
	mq := &ModelQuery{exclude: make(map[string]bool)}
	updateCols := mq.buildUpsertUpdateCols(dataMap, pkCols, nil)

	qb := &QueryBuilder{db: db}
	q := qb.Upsert("posts", dataMap).OnConflict(pkCols...).DoUpdate(updateCols...).Build()

	require.NotNil(t, q)
	assert.Contains(t, q.sql, "ON CONFLICT (post_id)")
	assert.Contains(t, q.sql, "content = EXCLUDED.content")
	assert.Contains(t, q.sql, "views = EXCLUDED.views")
	assert.NotContains(t, q.sql, "post_id = EXCLUDED.post_id")
}

// ============================================================================
// Upsert error path tests
// ============================================================================

func TestModelUpsert_Error_EmptyTable(t *testing.T) {
	db := upsertMockDB("postgres")
	user := upsertUser{ID: 1}

	mq := newTestMQ(db, &user, "")
	err := mq.Upsert()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "table name not specified")
}

func TestModelUpsert_Error_NoPrimaryKey(t *testing.T) {
	db := upsertMockDB("postgres")
	thing := noIDModelUpsert{Name: "thing"}

	mq := newTestMQ(db, &thing, "things")
	err := mq.Upsert()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary key not found")
}

// ============================================================================
// diffFields (UpdateChanged internals) unit tests
// ============================================================================

func TestDiffFields_SomeFieldsChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original
	current.Name = "Alice Updated"
	current.Status = "inactive"

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	require.NoError(t, err)
	assert.Len(t, changed, 2)
	assert.Equal(t, "Alice Updated", changed["name"])
	assert.Equal(t, "inactive", changed["status"])
	assert.NotContains(t, changed, "id")    // PK excluded
	assert.NotContains(t, changed, "email") // Unchanged
}

func TestDiffFields_NoFieldsChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original // exact copy

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	require.NoError(t, err)
	assert.Empty(t, changed)
}

func TestDiffFields_AllNonPKFieldsChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := diffUser{ID: 1, Name: "Bob", Email: "bob@example.com", Status: "inactive"}

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	require.NoError(t, err)
	assert.Len(t, changed, 3) // name, email, status
	assert.Equal(t, "Bob", changed["name"])
	assert.Equal(t, "bob@example.com", changed["email"])
	assert.Equal(t, "inactive", changed["status"])
	assert.NotContains(t, changed, "id") // PK never included
}

func TestDiffFields_TimeFieldChanged(t *testing.T) {
	db := upsertMockDB("postgres")
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	original := diffUserWithTime{ID: 1, Name: "Alice", CreatedAt: t1}
	current := diffUserWithTime{ID: 1, Name: "Alice", CreatedAt: t2}

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	require.NoError(t, err)
	assert.Len(t, changed, 1)
	assert.Equal(t, t2, changed["created_at"])
}

func TestDiffFields_TimeFieldUnchanged(t *testing.T) {
	db := upsertMockDB("postgres")
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	original := diffUserWithTime{ID: 1, Name: "Alice", CreatedAt: ts}
	current := original

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	require.NoError(t, err)
	assert.Empty(t, changed)
}

func TestDiffFields_TypeMismatch_Error(t *testing.T) {
	db := upsertMockDB("postgres")

	type OtherModel struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	current := diffUser{ID: 1, Name: "Alice", Email: "a@b.com", Status: "active"}
	other := OtherModel{ID: 1, Name: "Alice"}

	mq := newTestMQ(db, &current, "users")

	_, err := mq.diffFields(&other)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match model type")
}

func TestDiffFields_OriginalNotStruct_Error(t *testing.T) {
	db := upsertMockDB("postgres")
	current := diffUser{ID: 1}

	mq := newTestMQ(db, &current, "users")

	notStruct := 42
	_, err := mq.diffFields(&notStruct)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "original is not a struct")
}

func TestDiffFields_OriginalPassedByValue(t *testing.T) {
	// original passed as value (not pointer) — should work.
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original
	current.Name = "Bob"

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(original)
	require.NoError(t, err)
	assert.Len(t, changed, 1)
	assert.Equal(t, "Bob", changed["name"])
}

// ============================================================================
// UpdateChanged error path tests
// ============================================================================

func TestUpdateChanged_EmptyTable_Error(t *testing.T) {
	db := upsertMockDB("postgres")
	current := diffUser{ID: 1, Name: "Alice", Email: "a@b.com", Status: "active"}
	original := current
	current.Name = "Bob"

	mq := newTestMQ(db, &current, "")

	err := mq.UpdateChanged(&original)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "table name not specified")
}

func TestUpdateChanged_TypeMismatch_Error(t *testing.T) {
	db := upsertMockDB("postgres")

	type AnotherType struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	current := diffUser{ID: 1, Name: "Alice", Email: "a@b.com", Status: "active"}
	other := AnotherType{ID: 1, Name: "Bob"}

	mq := newTestMQ(db, &current, "users")

	err := mq.UpdateChanged(&other)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match model type")
}

func TestUpdateChanged_NoPrimaryKey_Error(t *testing.T) {
	db := upsertMockDB("postgres")

	type NoPK struct {
		Name  string `db:"name"`
		Email string `db:"email"`
	}
	current := NoPK{Name: "Alice", Email: "a@b.com"}
	original := current
	current.Name = "Bob"

	mq := newTestMQ(db, &current, "nopk")

	err := mq.UpdateChanged(&original)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary key not found")
}

// TestUpdateChanged_NothingChanged_NilNoQuery verifies that when nothing
// changed, UpdateChanged returns nil without executing any query.
// We verify via diffFields returning empty (no real DB needed).
func TestUpdateChanged_NothingChanged_NilNoQuery(t *testing.T) {
	db := upsertMockDB("postgres")
	original := diffUser{ID: 1, Name: "Alice", Email: "alice@example.com", Status: "active"}
	current := original // identical

	mq := newTestMQ(db, &current, "users")

	changed, err := mq.diffFields(&original)
	require.NoError(t, err)
	assert.Empty(t, changed, "no fields should be reported as changed — no query will execute")
}

// ============================================================================
// columnFromField unit tests
// ============================================================================

func TestColumnFromField_WithTag(t *testing.T) {
	type sample struct {
		Name string `db:"name"`
	}
	t2 := reflect.TypeOf(sample{})
	col, skip := columnFromField(t2.Field(0))
	assert.False(t, skip)
	assert.Equal(t, "name", col)
}

func TestColumnFromField_WithSkipTag(t *testing.T) {
	type sample struct {
		Name string `db:"-"`
	}
	t2 := reflect.TypeOf(sample{})
	_, skip := columnFromField(t2.Field(0))
	assert.True(t, skip)
}

func TestColumnFromField_NoTag(t *testing.T) {
	type sample struct {
		MyField string
	}
	t2 := reflect.TypeOf(sample{})
	col, skip := columnFromField(t2.Field(0))
	assert.False(t, skip)
	assert.Equal(t, "MyField", col)
}

func TestColumnFromField_PKCompositeTag(t *testing.T) {
	// db:"col_name,pk" → column = "col_name"
	type sample struct {
		TenantID int `db:"tenant_id,pk"`
	}
	t2 := reflect.TypeOf(sample{})
	col, skip := columnFromField(t2.Field(0))
	assert.False(t, skip)
	assert.Equal(t, "tenant_id", col)
}

// ============================================================================
// buildPKSet unit tests
// ============================================================================

func TestBuildPKSet_SinglePK(t *testing.T) {
	pkInfo := &util.PrimaryKeyInfo{
		Columns: []string{"id"},
	}
	set := buildPKSet(pkInfo)
	assert.True(t, set["id"])
	assert.False(t, set["name"])
}

func TestBuildPKSet_CompositePK(t *testing.T) {
	pkInfo := &util.PrimaryKeyInfo{
		Columns: []string{"order_id", "product_id"},
	}
	set := buildPKSet(pkInfo)
	assert.True(t, set["order_id"])
	assert.True(t, set["product_id"])
	assert.False(t, set["qty"])
}

func TestBuildPKSet_Nil(t *testing.T) {
	set := buildPKSet(nil)
	assert.Empty(t, set)
}
