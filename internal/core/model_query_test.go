package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ModelTestUser is a test model with TableName() interface.
type ModelTestUser struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Status    string    `db:"status"`
	CreatedAt time.Time `db:"created_at"`
}

func (ModelTestUser) TableName() string {
	return "test_users"
}

// TestProduct is a test model without TableName() interface.
type TestProduct struct {
	ID    int    `db:"id"`
	Title string `db:"title"`
	Price int    `db:"price"`
}

// TestPost with explicit primary key tag.
type TestPost struct {
	PostID  int    `db:"post_id,pk"`
	Content string `db:"content"`
}

// TestOrderItem with composite primary key.
type TestOrderItem struct {
	OrderID   int `db:"order_id,pk"`
	ProductID int `db:"product_id,pk"`
	Quantity  int `db:"quantity"`
}

func (TestOrderItem) TableName() string {
	return "order_items"
}

// TestComment with "ID" field name (no tag).
type TestComment struct {
	ID      int    `db:"comment_id"`
	Message string `db:"message"`
}

func TestInferTableName_WithTableNameMethod(t *testing.T) {
	user := ModelTestUser{}
	name := inferTableName(&user)
	assert.Equal(t, "test_users", name, "Should use TableName() method")
}

func TestInferTableName_DefaultPluralization(t *testing.T) {
	product := TestProduct{}
	name := inferTableName(&product)
	assert.Equal(t, "testproducts", name, "Should lowercase struct name + 's'")
}

func TestInferTableName_AlreadyPlural(t *testing.T) {
	type News struct {
		ID int `db:"id"`
	}
	news := News{}
	name := inferTableName(&news)
	assert.Equal(t, "news", name, "Should keep unchanged if already ends with 's'")
}

func TestModelQuery_Table_Override(t *testing.T) {
	user := ModelTestUser{}
	mq := &ModelQuery{
		model:   &user,
		table:   "test_users",
		exclude: make(map[string]bool),
	}

	mq.Table("archived_users")
	assert.Equal(t, "archived_users", mq.table, "Should override table name")
}

func TestModelQuery_Exclude(t *testing.T) {
	user := ModelTestUser{}
	mq := &ModelQuery{
		model:   &user,
		table:   "test_users",
		exclude: make(map[string]bool),
	}

	mq.Exclude("created_at", "status")

	assert.True(t, mq.exclude["created_at"], "Should exclude created_at")
	assert.True(t, mq.exclude["status"], "Should exclude status")
	assert.False(t, mq.exclude["name"], "Should not exclude name")
}

func TestModelQuery_FilterFields_OnlySpecified(t *testing.T) {
	mq := &ModelQuery{
		exclude: make(map[string]bool),
	}

	data := map[string]interface{}{
		"id":    1,
		"name":  "Alice",
		"email": "alice@example.com",
	}

	result := mq.filterFields(data, []string{"name", "email"})

	assert.Equal(t, 2, len(result), "Should have 2 fields")
	assert.Equal(t, "Alice", result["name"])
	assert.Equal(t, "alice@example.com", result["email"])
	assert.Nil(t, result["id"], "Should not include id")
}

func TestModelQuery_FilterFields_AllExceptExcluded(t *testing.T) {
	mq := &ModelQuery{
		exclude: map[string]bool{
			"created_at": true,
			"status":     true,
		},
	}

	data := map[string]interface{}{
		"id":         1,
		"name":       "Alice",
		"email":      "alice@example.com",
		"status":     "active",
		"created_at": time.Now(),
	}

	result := mq.filterFields(data, nil)

	assert.Equal(t, 3, len(result), "Should have 3 fields")
	assert.Equal(t, 1, result["id"])
	assert.Equal(t, "Alice", result["name"])
	assert.Equal(t, "alice@example.com", result["email"])
	assert.Nil(t, result["status"], "Should exclude status")
	assert.Nil(t, result["created_at"], "Should exclude created_at")
}

func TestModelQuery_FilterFields_OnlyWithExclude(t *testing.T) {
	mq := &ModelQuery{
		exclude: map[string]bool{
			"email": true,
		},
	}

	data := map[string]interface{}{
		"id":    1,
		"name":  "Alice",
		"email": "alice@example.com",
	}

	// Only takes precedence, but excluded fields still filtered.
	result := mq.filterFields(data, []string{"name", "email"})

	assert.Equal(t, 1, len(result), "Should have 1 field")
	assert.Equal(t, "Alice", result["name"])
	assert.Nil(t, result["email"], "Should exclude even if in only list")
}

func TestModelQuery_GetPrimaryKeys_SinglePK_IDField(t *testing.T) {
	// ModelTestUser has ID field (int) with db:"id" tag - found by field name fallback
	user := ModelTestUser{ID: 123}
	mq := &ModelQuery{
		model: &user,
	}

	cols, vals, err := mq.getPrimaryKeys()
	assert.NoError(t, err)
	assert.Equal(t, []string{"id"}, cols, "Should find primary key by ID field name")
	// ID is int in ModelTestUser
	assert.Len(t, vals, 1)
	assert.Equal(t, 123, vals[0], "Should return primary key value")
}

func TestModelQuery_GetPrimaryKeys_SinglePK_ExplicitTag(t *testing.T) {
	// TestPost has explicit db:"post_id,pk" tag
	post := TestPost{PostID: 456}
	mq := &ModelQuery{
		model: &post,
	}

	cols, vals, err := mq.getPrimaryKeys()
	assert.NoError(t, err)
	assert.Equal(t, []string{"post_id"}, cols, "Should find primary key by db:\"column,pk\" tag")
	assert.Equal(t, []interface{}{456}, vals, "Should return primary key value")
}

func TestModelQuery_GetPrimaryKeys_CompositePK(t *testing.T) {
	// TestOrderItem has composite PK: order_id + product_id
	item := TestOrderItem{OrderID: 100, ProductID: 200, Quantity: 5}
	mq := &ModelQuery{
		model: &item,
	}

	cols, vals, err := mq.getPrimaryKeys()
	assert.NoError(t, err)
	assert.Equal(t, []string{"order_id", "product_id"}, cols, "Should find both PK columns")
	assert.Equal(t, []interface{}{100, 200}, vals, "Should return both PK values")
}

func TestModelQuery_GetPrimaryKeys_IDFieldWithDbTag(t *testing.T) {
	// TestComment has ID field with db:"comment_id" tag
	comment := TestComment{ID: 789}
	mq := &ModelQuery{
		model: &comment,
	}

	cols, vals, err := mq.getPrimaryKeys()
	assert.NoError(t, err)
	// Should find by "ID" field name, but use db tag value "comment_id"
	assert.Equal(t, []string{"comment_id"}, cols, "Should use db tag value for column name")
	assert.Equal(t, []interface{}{789}, vals, "Should return primary key value")
}

func TestModelQuery_GetPrimaryKeys_NotFound(t *testing.T) {
	type NoID struct {
		Name string `db:"name"`
	}
	noID := NoID{Name: "test"}
	mq := &ModelQuery{
		model: &noID,
	}

	cols, vals, err := mq.getPrimaryKeys()
	assert.Error(t, err, "Should return error for missing PK")
	assert.Nil(t, cols, "Should return nil columns")
	assert.Nil(t, vals, "Should return nil values")
}

func TestModelQuery_GetPrimaryKeys_ExplicitPK_TakesPrecedence(t *testing.T) {
	// Explicit pk tag should take precedence over ID field
	type WithExplicitPK struct {
		ID       int `db:"id"`
		TenantID int `db:"tenant_id,pk"`
	}
	model := WithExplicitPK{ID: 111, TenantID: 222}
	mq := &ModelQuery{
		model: &model,
	}

	cols, vals, err := mq.getPrimaryKeys()
	assert.NoError(t, err)
	// Explicit pk tag takes precedence
	assert.Equal(t, []string{"tenant_id"}, cols, "Explicit pk tag should take precedence")
	assert.Equal(t, []interface{}{222}, vals)
}
