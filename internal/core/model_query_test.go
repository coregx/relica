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

// TestPost with custom primary key tag.
type TestPost struct {
	PostID  int    `db:"post_id"`
	Content string `db:"content"`
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

func TestModelQuery_GetPrimaryKey_WithDbTag(t *testing.T) {
	user := ModelTestUser{ID: 123}
	mq := &ModelQuery{
		model: &user,
	}

	pk, pkValue := mq.getPrimaryKey()
	assert.Equal(t, "id", pk, "Should find primary key by db:\"id\" tag")
	assert.Equal(t, 123, pkValue, "Should return primary key value")
}

func TestModelQuery_GetPrimaryKey_WithSuffixId(t *testing.T) {
	post := TestPost{PostID: 456}
	mq := &ModelQuery{
		model: &post,
	}

	pk, pkValue := mq.getPrimaryKey()
	assert.Equal(t, "post_id", pk, "Should find primary key by db:\"*_id\" tag")
	assert.Equal(t, 456, pkValue, "Should return primary key value")
}

func TestModelQuery_GetPrimaryKey_WithIDField(t *testing.T) {
	comment := TestComment{ID: 789}
	mq := &ModelQuery{
		model: &comment,
	}

	pk, pkValue := mq.getPrimaryKey()
	// Should find by "ID" field name, but use db tag value "comment_id".
	assert.Equal(t, "comment_id", pk, "Should use db tag value")
	assert.Equal(t, 789, pkValue, "Should return primary key value")
}

func TestModelQuery_GetPrimaryKey_NotFound(t *testing.T) {
	type NoID struct {
		Name string `db:"name"`
	}
	noID := NoID{Name: "test"}
	mq := &ModelQuery{
		model: &noID,
	}

	pk, pkValue := mq.getPrimaryKey()
	assert.Equal(t, "", pk, "Should return empty string for missing PK")
	assert.Nil(t, pkValue, "Should return nil for missing PK")
}

func TestModelQuery_GetPrimaryKey_MultipleIDFields(t *testing.T) {
	// First db:"id" or db:"*_id" should win.
	type Multi struct {
		UserID int `db:"user_id"`
		ID     int `db:"id"`
	}
	multi := Multi{UserID: 111, ID: 222}
	mq := &ModelQuery{
		model: &multi,
	}

	pk, pkValue := mq.getPrimaryKey()
	// Should find first *_id tag.
	assert.Equal(t, "user_id", pk, "Should find first *_id tag")
	assert.Equal(t, 111, pkValue)
}
