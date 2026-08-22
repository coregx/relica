package core

import (
	"testing"
	"time"
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
	if name != "test_users" {
		t.Errorf("Should use TableName() method: got %v, want %v", name, "test_users")
	}
}

func TestInferTableName_DefaultPluralization(t *testing.T) {
	product := TestProduct{}
	name := inferTableName(&product)
	if name != "testproducts" {
		t.Errorf("Should lowercase struct name + 's': got %v, want %v", name, "testproducts")
	}
}

func TestInferTableName_AlreadyPlural(t *testing.T) {
	type News struct {
		ID int `db:"id"`
	}
	news := News{}
	name := inferTableName(&news)
	if name != "news" {
		t.Errorf("Should keep unchanged if already ends with 's': got %v, want %v", name, "news")
	}
}

func TestModelQuery_Table_Override(t *testing.T) {
	user := ModelTestUser{}
	mq := &ModelQuery{
		model:   &user,
		table:   "test_users",
		exclude: make(map[string]bool),
	}

	mq.Table("archived_users")
	if mq.table != "archived_users" {
		t.Errorf("Should override table name: got %v, want %v", mq.table, "archived_users")
	}
}

func TestModelQuery_Exclude(t *testing.T) {
	user := ModelTestUser{}
	mq := &ModelQuery{
		model:   &user,
		table:   "test_users",
		exclude: make(map[string]bool),
	}

	mq.Exclude("created_at", "status")

	if !mq.exclude["created_at"] {
		t.Error("Should exclude created_at: expected true")
	}
	if !mq.exclude["status"] {
		t.Error("Should exclude status: expected true")
	}
	if mq.exclude["name"] {
		t.Error("Should not exclude name: expected false")
	}
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

	if len(result) != 2 {
		t.Errorf("Should have 2 fields: got %v, want %v", len(result), 2)
	}
	if result["name"] != "Alice" {
		t.Errorf("got %v, want %v", result["name"], "Alice")
	}
	if result["email"] != "alice@example.com" {
		t.Errorf("got %v, want %v", result["email"], "alice@example.com")
	}
	if result["id"] != nil {
		t.Errorf("Should not include id: expected nil, got %v", result["id"])
	}
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

	if len(result) != 3 {
		t.Errorf("Should have 3 fields: got %v, want %v", len(result), 3)
	}
	if result["id"] != 1 {
		t.Errorf("got %v, want %v", result["id"], 1)
	}
	if result["name"] != "Alice" {
		t.Errorf("got %v, want %v", result["name"], "Alice")
	}
	if result["email"] != "alice@example.com" {
		t.Errorf("got %v, want %v", result["email"], "alice@example.com")
	}
	if result["status"] != nil {
		t.Errorf("Should exclude status: expected nil, got %v", result["status"])
	}
	if result["created_at"] != nil {
		t.Errorf("Should exclude created_at: expected nil, got %v", result["created_at"])
	}
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

	if len(result) != 1 {
		t.Errorf("Should have 1 field: got %v, want %v", len(result), 1)
	}
	if result["name"] != "Alice" {
		t.Errorf("got %v, want %v", result["name"], "Alice")
	}
	if result["email"] != nil {
		t.Errorf("Should exclude even if in only list: expected nil, got %v", result["email"])
	}
}

func TestModelQuery_GetPrimaryKeys_SinglePK_IDField(t *testing.T) {
	// ModelTestUser has ID field (int) with db:"id" tag - found by field name fallback
	user := ModelTestUser{ID: 123}
	mq := &ModelQuery{
		model: &user,
	}

	cols, vals, err := mq.getPrimaryKeys()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(cols) != 1 || cols[0] != "id" {
		t.Errorf("Should find primary key by ID field name: got %v, want %v", cols, []string{"id"})
	}
	// ID is int in ModelTestUser
	if len(vals) != 1 {
		t.Errorf("expected length %d, got %d", 1, len(vals))
	}
	if vals[0] != 123 {
		t.Errorf("Should return primary key value: got %v, want %v", vals[0], 123)
	}
}

func TestModelQuery_GetPrimaryKeys_SinglePK_ExplicitTag(t *testing.T) {
	// TestPost has explicit db:"post_id,pk" tag
	post := TestPost{PostID: 456}
	mq := &ModelQuery{
		model: &post,
	}

	cols, vals, err := mq.getPrimaryKeys()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(cols) != 1 || cols[0] != "post_id" {
		t.Errorf(`Should find primary key by db:"column,pk" tag: got %v, want %v`, cols, []string{"post_id"})
	}
	want := []interface{}{456}
	if len(vals) != len(want) || vals[0] != want[0] {
		t.Errorf("Should return primary key value: got %v, want %v", vals, want)
	}
}

func TestModelQuery_GetPrimaryKeys_CompositePK(t *testing.T) {
	// TestOrderItem has composite PK: order_id + product_id
	item := TestOrderItem{OrderID: 100, ProductID: 200, Quantity: 5}
	mq := &ModelQuery{
		model: &item,
	}

	cols, vals, err := mq.getPrimaryKeys()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	wantCols := []string{"order_id", "product_id"}
	if len(cols) != len(wantCols) || cols[0] != wantCols[0] || cols[1] != wantCols[1] {
		t.Errorf("Should find both PK columns: got %v, want %v", cols, wantCols)
	}
	wantVals := []interface{}{100, 200}
	if len(vals) != len(wantVals) || vals[0] != wantVals[0] || vals[1] != wantVals[1] {
		t.Errorf("Should return both PK values: got %v, want %v", vals, wantVals)
	}
}

func TestModelQuery_GetPrimaryKeys_IDFieldWithDbTag(t *testing.T) {
	// TestComment has ID field with db:"comment_id" tag
	comment := TestComment{ID: 789}
	mq := &ModelQuery{
		model: &comment,
	}

	cols, vals, err := mq.getPrimaryKeys()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Should find by "ID" field name, but use db tag value "comment_id"
	if len(cols) != 1 || cols[0] != "comment_id" {
		t.Errorf("Should use db tag value for column name: got %v, want %v", cols, []string{"comment_id"})
	}
	want := []interface{}{789}
	if len(vals) != len(want) || vals[0] != want[0] {
		t.Errorf("Should return primary key value: got %v, want %v", vals, want)
	}
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
	if err == nil {
		t.Error("Should return error for missing PK: expected error")
	}
	if cols != nil {
		t.Errorf("Should return nil columns: expected nil, got %v", cols)
	}
	if vals != nil {
		t.Errorf("Should return nil values: expected nil, got %v", vals)
	}
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
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Explicit pk tag takes precedence
	if len(cols) != 1 || cols[0] != "tenant_id" {
		t.Errorf("Explicit pk tag should take precedence: got %v, want %v", cols, []string{"tenant_id"})
	}
	want := []interface{}{222}
	if len(vals) != len(want) || vals[0] != want[0] {
		t.Errorf("got %v, want %v", vals, want)
	}
}
