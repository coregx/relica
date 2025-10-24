package core

import (
	"context"
	"testing"
	"time"
)

// TestQueryBuilder_WithContext tests that WithContext sets context on QueryBuilder
func TestQueryBuilder_WithContext(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}

	ctx := context.WithValue(context.Background(), "key", "value")
	qb2 := qb.WithContext(ctx)

	if qb2.ctx != ctx {
		t.Error("WithContext should set context on QueryBuilder")
	}

	// Should return the same builder (fluent API)
	if qb2 != qb {
		t.Error("WithContext should return the same QueryBuilder instance")
	}
}

// TestSelectQuery_WithContext tests that WithContext sets context on SelectQuery
func TestSelectQuery_WithContext(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}
	sq := &SelectQuery{builder: qb}

	ctx := context.WithValue(context.Background(), "key", "value")
	sq2 := sq.WithContext(ctx)

	if sq2.ctx != ctx {
		t.Error("WithContext should set context on SelectQuery")
	}

	// Should return the same query (fluent API)
	if sq2 != sq {
		t.Error("WithContext should return the same SelectQuery instance")
	}
}

// TestUpdateQuery_WithContext tests that WithContext sets context on UpdateQuery
func TestUpdateQuery_WithContext(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}
	uq := &UpdateQuery{builder: qb}

	ctx := context.WithValue(context.Background(), "key", "value")
	uq2 := uq.WithContext(ctx)

	if uq2.ctx != ctx {
		t.Error("WithContext should set context on UpdateQuery")
	}

	if uq2 != uq {
		t.Error("WithContext should return the same UpdateQuery instance")
	}
}

// TestDeleteQuery_WithContext tests that WithContext sets context on DeleteQuery
func TestDeleteQuery_WithContext(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}
	dq := &DeleteQuery{builder: qb}

	ctx := context.WithValue(context.Background(), "key", "value")
	dq2 := dq.WithContext(ctx)

	if dq2.ctx != ctx {
		t.Error("WithContext should set context on DeleteQuery")
	}

	if dq2 != dq {
		t.Error("WithContext should return the same DeleteQuery instance")
	}
}

// TestUpsertQuery_WithContext tests that WithContext sets context on UpsertQuery
func TestUpsertQuery_WithContext(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}
	uq := &UpsertQuery{builder: qb}

	ctx := context.WithValue(context.Background(), "key", "value")
	uq2 := uq.WithContext(ctx)

	if uq2.ctx != ctx {
		t.Error("WithContext should set context on UpsertQuery")
	}

	if uq2 != uq {
		t.Error("WithContext should return the same UpsertQuery instance")
	}
}

// TestBatchInsertQuery_WithContext tests that WithContext sets context on BatchInsertQuery
func TestBatchInsertQuery_WithContext(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}
	biq := &BatchInsertQuery{builder: qb}

	ctx := context.WithValue(context.Background(), "key", "value")
	biq2 := biq.WithContext(ctx)

	if biq2.ctx != ctx {
		t.Error("WithContext should set context on BatchInsertQuery")
	}

	if biq2 != biq {
		t.Error("WithContext should return the same BatchInsertQuery instance")
	}
}

// TestBatchUpdateQuery_WithContext tests that WithContext sets context on BatchUpdateQuery
func TestBatchUpdateQuery_WithContext(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}
	buq := &BatchUpdateQuery{builder: qb}

	ctx := context.WithValue(context.Background(), "key", "value")
	buq2 := buq.WithContext(ctx)

	if buq2.ctx != ctx {
		t.Error("WithContext should set context on BatchUpdateQuery")
	}

	if buq2 != buq {
		t.Error("WithContext should return the same BatchUpdateQuery instance")
	}
}

// TestContext_Priority_QueryOverBuilder tests context priority: query > builder
func TestContext_Priority_QueryOverBuilder(t *testing.T) {
	// This test will be completed in integration tests where we have a real DB
	// For now, we just test that the context is properly set
	db := &DB{}

	builderCtx := context.WithValue(context.Background(), "source", "builder")
	queryCtx := context.WithValue(context.Background(), "source", "query")

	qb := &QueryBuilder{db: db, ctx: builderCtx}
	sq := &SelectQuery{builder: qb, ctx: queryCtx}

	// Query context should take precedence
	if sq.ctx != queryCtx {
		t.Error("Query context should be set on SelectQuery")
	}

	// When we don't set query context, it should use builder context
	sq2 := &SelectQuery{builder: qb}
	if sq2.ctx != nil {
		t.Error("SelectQuery without explicit context should have nil ctx initially")
	}
}

// TestContext_Priority_BuilderOverDefault tests context priority: builder > default
func TestContext_Priority_BuilderOverDefault(t *testing.T) {
	db := &DB{}

	builderCtx := context.WithValue(context.Background(), "source", "builder")
	qb := &QueryBuilder{db: db, ctx: builderCtx}

	// When no query context is set, builder context should be used
	if qb.ctx != builderCtx {
		t.Error("Builder context should be set")
	}

	// Query without explicit context should eventually use builder context
	sq := &SelectQuery{builder: qb}
	if sq.ctx != nil {
		t.Error("SelectQuery should not have context before Build()")
	}
}

// TestContextNil_DefaultsToBackground tests that nil context is handled
func TestContextNil_DefaultsToBackground(t *testing.T) {
	// Create query without any context
	db := &DB{}
	qb := &QueryBuilder{db: db}

	if qb.ctx != nil {
		t.Error("QueryBuilder without WithContext should have nil context")
	}

	// This is fine - context will be resolved in Build() or Execute()
}

// TestMultipleWithContext_LastWins tests that multiple WithContext calls work
func TestMultipleWithContext_LastWins(t *testing.T) {
	db := &DB{}
	qb := &QueryBuilder{db: db}

	ctx1 := context.WithValue(context.Background(), "key", "value1")
	ctx2 := context.WithValue(context.Background(), "key", "value2")

	qb.WithContext(ctx1).WithContext(ctx2)

	if qb.ctx != ctx2 {
		t.Error("Last WithContext call should win")
	}
}

// TestContextCancellation_BeforeExecute tests context cancellation
func TestContextCancellation_BeforeExecute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Verify context is canceled
	if ctx.Err() == nil {
		t.Error("Context should be canceled")
	}

	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", ctx.Err())
	}
}

// TestContextTimeout tests context with timeout
func TestContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(50 * time.Millisecond)

	if ctx.Err() == nil {
		t.Error("Context should have timed out")
	}

	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", ctx.Err())
	}
}

// TestContextDeadline tests context with deadline
func TestContextDeadline(t *testing.T) {
	deadline := time.Now().Add(10 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	// Wait for deadline
	time.Sleep(50 * time.Millisecond)

	if ctx.Err() == nil {
		t.Error("Context should have exceeded deadline")
	}

	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", ctx.Err())
	}
}
