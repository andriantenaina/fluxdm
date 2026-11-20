package engine

import (
	"context"
	"strings"
	"sync"
)

// HostGate limits the number of simultaneous connections to one host across
// all downloads (a server that sees too many connections from the same client
// throttles or bans it). A nil gate never blocks.
type HostGate struct {
	mu      sync.Mutex
	limit   int // 0 = unlimited
	inUse   map[string]int
	changed chan struct{}
}

// NewHostGate creates a gate allowing limit connections per host (0 = unlimited).
func NewHostGate(limit int) *HostGate {
	return &HostGate{limit: limit, inUse: map[string]int{}, changed: make(chan struct{})}
}

// SetLimit changes the per-host limit (0 = unlimited) and wakes waiters.
func (g *HostGate) SetLimit(n int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.limit = max(n, 0)
	g.notifyLocked()
	g.mu.Unlock()
}

// Limit returns the per-host limit (0 = unlimited).
func (g *HostGate) Limit() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.limit
}

// InUse returns the number of connections currently held for host.
func (g *HostGate) InUse(host string) int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inUse[strings.ToLower(host)]
}

// Saturated reports whether host has no free connection slot.
func (g *HostGate) Saturated(host string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.limit > 0 && g.inUse[strings.ToLower(host)] >= g.limit
}

func (g *HostGate) notifyLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}

func (g *HostGate) releaser(host string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			if g.inUse[host] <= 1 {
				delete(g.inUse, host)
			} else {
				g.inUse[host]--
			}
			g.notifyLocked()
			g.mu.Unlock()
		})
	}
}

func noop() {}

// TryAcquire takes a connection slot for host without blocking.
func (g *HostGate) TryAcquire(host string) (release func(), ok bool) {
	if g == nil || host == "" {
		return noop, true
	}
	host = strings.ToLower(host)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.limit > 0 && g.inUse[host] >= g.limit {
		return nil, false
	}
	g.inUse[host]++
	return g.releaser(host), true
}

// Acquire takes a connection slot for host, waiting until one is free.
func (g *HostGate) Acquire(ctx context.Context, host string) (release func(), err error) {
	if g == nil || host == "" {
		return noop, nil
	}
	host = strings.ToLower(host)
	for {
		g.mu.Lock()
		if g.limit <= 0 || g.inUse[host] < g.limit {
			g.inUse[host]++
			g.mu.Unlock()
			return g.releaser(host), nil
		}
		ch := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}
