package database

import (
	"container/list"
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// ConnectionPool is an LRU pool of per-user SQLite connections.
type ConnectionPool struct {
	mu      sync.Mutex
	maxSize int
	items   map[string]*poolEntry
	order   *list.List // front = most recently used
	timeout time.Duration
	logger  *slog.Logger
}

type poolEntry struct {
	db      *sql.DB
	userID  string
	lastUsed time.Time
	element *list.Element
}

// NewConnectionPool creates a new LRU connection pool.
func NewConnectionPool(maxSize int, timeout time.Duration, logger *slog.Logger) *ConnectionPool {
	return &ConnectionPool{
		maxSize: maxSize,
		items:   make(map[string]*poolEntry),
		order:   list.New(),
		timeout: timeout,
		logger:  logger,
	}
}

// Get retrieves a connection from the pool and updates the LRU order.
func (p *ConnectionPool) Get(userID string) (*sql.DB, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.items[userID]
	if !ok {
		return nil, false
	}

	entry.lastUsed = time.Now()
	p.order.MoveToFront(entry.element)
	return entry.db, true
}

// Put adds a connection to the pool. If the pool is at capacity,
// the least recently used connection is evicted and closed.
func (p *ConnectionPool) Put(userID string, db *sql.DB) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If already in pool, update
	if entry, ok := p.items[userID]; ok {
		entry.db = db
		entry.lastUsed = time.Now()
		p.order.MoveToFront(entry.element)
		return
	}

	// Evict LRU if at capacity
	if p.order.Len() >= p.maxSize {
		p.evictLRU()
	}

	entry := &poolEntry{
		db:       db,
		userID:   userID,
		lastUsed: time.Now(),
	}
	entry.element = p.order.PushFront(entry)
	p.items[userID] = entry
}

// Remove removes and closes a specific connection from the pool.
func (p *ConnectionPool) Remove(userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.items[userID]
	if !ok {
		return
	}

	p.order.Remove(entry.element)
	delete(p.items, userID)
	if err := entry.db.Close(); err != nil {
		p.logger.Error("failed to close evicted connection", "user_id", userID, "error", err)
	}
}

// Close closes all connections in the pool.
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for userID, entry := range p.items {
		if err := entry.db.Close(); err != nil {
			p.logger.Error("failed to close connection", "user_id", userID, "error", err)
		}
	}
	p.items = make(map[string]*poolEntry)
	p.order.Init()
}

// StartCleanup runs a background goroutine that periodically closes
// connections that have been idle longer than the configured timeout.
func (p *ConnectionPool) StartCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.cleanup()
			}
		}
	}()
}

func (p *ConnectionPool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	var toRemove []string

	for userID, entry := range p.items {
		if now.Sub(entry.lastUsed) > p.timeout {
			toRemove = append(toRemove, userID)
		}
	}

	for _, userID := range toRemove {
		entry := p.items[userID]
		p.order.Remove(entry.element)
		delete(p.items, userID)
		if err := entry.db.Close(); err != nil {
			p.logger.Error("failed to close idle connection", "user_id", userID, "error", err)
		}
		p.logger.Debug("closed idle connection", "user_id", userID)
	}
}

// evictLRU removes and closes the least recently used connection.
// Must be called with p.mu held.
func (p *ConnectionPool) evictLRU() {
	back := p.order.Back()
	if back == nil {
		return
	}

	entry := back.Value.(*poolEntry)
	p.order.Remove(back)
	delete(p.items, entry.userID)
	if err := entry.db.Close(); err != nil {
		p.logger.Error("failed to close evicted connection", "user_id", entry.userID, "error", err)
	}
	p.logger.Debug("evicted LRU connection", "user_id", entry.userID)
}
