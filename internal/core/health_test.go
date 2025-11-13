package core

import (
	"database/sql"
	"testing"
	"time"

	"github.com/coregx/relica/internal/logger"
	_ "modernc.org/sqlite"
)

func TestHealthChecker_Basic(t *testing.T) {
	// Create test database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create health checker
	log := &logger.NoopLogger{}
	hc := newHealthChecker(db, log, 100*time.Millisecond)

	// Start health checks
	hc.start()
	defer hc.shutdown()

	// Wait for at least one health check
	time.Sleep(150 * time.Millisecond)

	// Check health status
	if !hc.isHealthy() {
		t.Error("Health check should pass for valid database")
	}

	// Check last check time
	lastCheck := hc.lastCheck()
	if lastCheck.IsZero() {
		t.Error("Last check time should not be zero")
	}
}

func TestHealthChecker_Shutdown(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	log := &logger.NoopLogger{}
	hc := newHealthChecker(db, log, 50*time.Millisecond)

	hc.start()

	// Wait a bit
	time.Sleep(75 * time.Millisecond)

	// Shutdown should not hang
	done := make(chan struct{})
	go func() {
		hc.shutdown()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Error("Shutdown took too long")
	}
}

func TestDB_Stats(t *testing.T) {
	// Create test DB
	coreDB, err := NewDB("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer coreDB.Close()

	// Get stats
	stats := coreDB.Stats()

	// Check basic fields are present
	if stats.MaxOpenConnections < 0 {
		t.Error("MaxOpenConnections should be non-negative")
	}

	// Initially should be healthy (no health checker)
	if !stats.Healthy {
		t.Error("DB without health checker should be healthy by default")
	}

	// LastHealthCheck should be zero (no health checker)
	if !stats.LastHealthCheck.IsZero() {
		t.Error("LastHealthCheck should be zero when health checks disabled")
	}
}

func TestDB_WithHealthCheck(t *testing.T) {
	// Create DB with health check
	coreDB, err := Open("sqlite", ":memory:",
		WithHealthCheck(100*time.Millisecond))
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer coreDB.Close()

	// Wait for health check
	time.Sleep(150 * time.Millisecond)

	// Check health
	if !coreDB.IsHealthy() {
		t.Error("DB should be healthy")
	}

	// Get stats
	stats := coreDB.Stats()
	if !stats.Healthy {
		t.Error("Stats should show healthy DB")
	}

	if stats.LastHealthCheck.IsZero() {
		t.Error("LastHealthCheck should not be zero when health checks enabled")
	}
}

func TestDB_ConnectionPoolOptions(t *testing.T) {
	// Create DB with all pool options
	coreDB, err := Open("sqlite", ":memory:",
		WithMaxOpenConns(10),
		WithMaxIdleConns(5),
		WithConnMaxLifetime(5*time.Minute),
		WithConnMaxIdleTime(1*time.Minute))
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer coreDB.Close()

	// Get stats
	stats := coreDB.Stats()

	// Check max open connections was set
	if stats.MaxOpenConnections != 10 {
		t.Errorf("Expected MaxOpenConnections=10, got %d", stats.MaxOpenConnections)
	}
}
