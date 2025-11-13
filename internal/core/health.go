package core

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/coregx/relica/internal/logger"
)

// healthChecker performs periodic health checks on database connections.
// It pings the database at regular intervals to detect dead connections early.
type healthChecker struct {
	db       *sql.DB
	logger   logger.Logger
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	lastErr  error
	lastPing time.Time
}

// newHealthChecker creates a new health checker that pings the database at the specified interval.
func newHealthChecker(db *sql.DB, log logger.Logger, interval time.Duration) *healthChecker {
	return &healthChecker{
		db:       db,
		logger:   log,
		interval: interval,
		stop:     make(chan struct{}),
	}
}

// start begins the health check loop in a background goroutine.
func (h *healthChecker) start() {
	h.wg.Add(1)
	go h.run()
}

// run is the main health check loop.
func (h *healthChecker) run() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.ping()
		case <-h.stop:
			return
		}
	}
}

// ping performs a single health check.
func (h *healthChecker) ping() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := h.db.PingContext(ctx)

	h.mu.Lock()
	h.lastErr = err
	h.lastPing = time.Now()
	h.mu.Unlock()

	if err != nil {
		h.logger.Warn("database health check failed",
			"error", err,
			"interval", h.interval)
	} else {
		h.logger.Debug("database health check passed",
			"interval", h.interval)
	}
}

// shutdown halts the health checker and waits for it to finish.
func (h *healthChecker) shutdown() {
	close(h.stop)
	h.wg.Wait()
}

// isHealthy returns true if the last health check was successful.
func (h *healthChecker) isHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastErr == nil
}

// lastError returns the error from the most recent health check.
//nolint:unused // May be used for debugging/monitoring
func (h *healthChecker) lastError() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastErr
}

// lastCheck returns the time of the most recent health check.
func (h *healthChecker) lastCheck() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastPing
}
