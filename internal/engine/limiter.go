package engine

import (
	"context"
	"math"
	"sync"
	"time"
)

// Limiter is a bandwidth limiter whose rate can change at runtime (0 = unlimited).
//
// It shares its rate fairly between "shares" (one per download): every
// rebalance (~250 ms) the rate is split with a weighted max-min (water-filling)
// allocation, so a download that cannot use its part (slow server) leaves the
// rest to the others, and a download with 16 connections does not get more
// than one with 2. Calls to Limiter.WaitN use a default share common to every
// caller that has no share of its own (FTP, torrent...).
//
// Changing the rate wakes the goroutines already waiting, so raising the limit
// takes effect immediately.
type Limiter struct {
	mu      sync.Mutex
	rate    float64 // bytes/s, 0 = unlimited
	changed chan struct{}
	shares  map[*Share]struct{}
	def     *Share
	lastBal time.Time
}

// Share is one participant of a Limiter (typically one download).
type Share struct {
	l       *Limiter
	weight  float64
	alloc   float64 // allocated bytes/s
	tokens  float64
	last    time.Time
	used    int64   // bytes taken since the last rebalance
	waited  bool    // had to wait since the last rebalance (wants more)
	demand  float64 // smoothed consumption (bytes/s)
	lastUse time.Time
	closed  bool
}

const (
	rebalanceEvery = 250 * time.Millisecond
	shareIdle      = 2 * time.Second
	maxBurst       = 1 << 20
	minBurst       = 16 << 10
)

// NewLimiter creates a limiter in bytes/second (0 = unlimited).
func NewLimiter(bytesPerSec int64) *Limiter {
	l := &Limiter{changed: make(chan struct{}), shares: map[*Share]struct{}{}}
	l.def = &Share{l: l, weight: 1}
	l.shares[l.def] = struct{}{}
	l.SetRate(bytesPerSec)
	return l
}

// SetRate changes the rate in bytes/second (0 = unlimited) and wakes waiters.
func (l *Limiter) SetRate(bytesPerSec int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r := float64(bytesPerSec)
	if r < 0 {
		r = 0
	}
	if r == l.rate {
		return
	}
	now := time.Now()
	l.rate = r
	for s := range l.shares {
		s.tokens = math.Min(s.tokens, s.burst())
		s.last = now
	}
	l.rebalanceLocked(now)
}

// Rate returns the current rate (0 = unlimited).
func (l *Limiter) Rate() int64 {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return int64(l.rate)
}

// NewShare registers a participant with a weight (1 = normal, 2 = high
// priority, 0.5 = low). Close it when the download stops.
func (l *Limiter) NewShare(weight float64) *Share {
	if l == nil {
		return nil
	}
	if weight <= 0 {
		weight = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	s := &Share{l: l, weight: weight, last: now, lastUse: now}
	l.shares[s] = struct{}{}
	l.rebalanceLocked(now)
	s.tokens = s.burst() / 4 // small head start, like a fresh token bucket
	return s
}

// Close unregisters the share; its bandwidth goes to the others.
func (s *Share) Close() {
	if s == nil || s == s.l.def {
		return
	}
	l := s.l
	l.mu.Lock()
	defer l.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	delete(l.shares, s)
	l.rebalanceLocked(time.Now())
}

// SetWeight changes the weight (priority) of the share.
func (s *Share) SetWeight(w float64) {
	if s == nil || w <= 0 {
		return
	}
	s.l.mu.Lock()
	s.weight = w
	s.l.rebalanceLocked(time.Now())
	s.l.mu.Unlock()
}

// Alloc returns the bandwidth currently allocated to the share (0 = unlimited).
func (s *Share) Alloc() int64 {
	if s == nil {
		return 0
	}
	s.l.mu.Lock()
	defer s.l.mu.Unlock()
	if s.l.rate <= 0 {
		return 0
	}
	return int64(s.alloc)
}

func (s *Share) burst() float64 {
	b := s.alloc / 4
	if b > maxBurst {
		b = maxBurst
	}
	if b < minBurst {
		b = minBurst
	}
	return b
}

// rebalanceLocked computes a weighted max-min fair allocation of the rate.
func (l *Limiter) rebalanceLocked(now time.Time) {
	dt := now.Sub(l.lastBal).Seconds()
	l.lastBal = now
	type cand struct {
		s   *Share
		cap float64
	}
	var act []cand
	for s := range l.shares {
		if dt > 0.05 {
			inst := float64(s.used) / dt
			if s.demand == 0 {
				s.demand = inst
			} else {
				s.demand = 0.5*s.demand + 0.5*inst
			}
			s.used = 0
		}
		if now.Sub(s.lastUse) > shareIdle {
			s.alloc = 0
			s.waited = false
			continue
		}
		c := math.Inf(1)
		if !s.waited && s.demand > 0 {
			// not limited by us: it needs about what it consumed, plus headroom
			c = s.demand*1.25 + 32<<10
		}
		act = append(act, cand{s, c})
		s.waited = false
	}
	if l.rate > 0 && len(act) > 0 {
		rem := l.rate
		open := act
		for len(open) > 0 {
			var sumW float64
			for _, c := range open {
				sumW += c.s.weight
			}
			var next []cand
			progressed := false
			for _, c := range open {
				fair := rem * c.s.weight / sumW
				if c.cap < fair {
					c.s.alloc = c.cap
					progressed = true
				} else {
					next = append(next, c)
				}
			}
			if !progressed {
				for _, c := range open {
					c.s.alloc = rem * c.s.weight / sumW
				}
				rem = 0
				break
			}
			rem = l.rate
			for _, c := range act {
				isOpen := false
				for _, n := range next {
					if n.s == c.s {
						isOpen = true
						break
					}
				}
				if !isOpen {
					rem -= c.s.alloc
				}
			}
			open = next
		}
		if rem > 0 { // everyone is satisfied: spread the leftover as headroom
			var sumW float64
			for _, c := range act {
				sumW += c.s.weight
			}
			for _, c := range act {
				c.s.alloc += rem * c.s.weight / sumW
			}
		}
	}
	close(l.changed)
	l.changed = make(chan struct{})
}

// WaitN blocks until n bytes may be consumed through the default share.
func (l *Limiter) WaitN(ctx context.Context, n int) error {
	if l == nil {
		return nil
	}
	return l.def.WaitN(ctx, n)
}

// WaitN blocks until n bytes may be consumed by this share.
func (s *Share) WaitN(ctx context.Context, n int) error {
	if s == nil {
		return nil
	}
	l := s.l
	for n > 0 {
		l.mu.Lock()
		now := time.Now()
		if l.rate <= 0 {
			s.used += int64(n)
			s.lastUse = now
			l.mu.Unlock()
			return nil
		}
		wasIdle := now.Sub(s.lastUse) > shareIdle
		s.lastUse = now
		if wasIdle || now.Sub(l.lastBal) >= rebalanceEvery {
			if wasIdle {
				s.last = now
				s.demand = 0 // unknown again: let it claim a full share
			}
			l.rebalanceLocked(now)
		}
		b := s.burst()
		if s.alloc > 0 {
			s.tokens = math.Min(b, s.tokens+s.alloc*now.Sub(s.last).Seconds())
		}
		s.last = now
		chunk := float64(n)
		if chunk > b {
			chunk = b
		}
		if s.tokens >= chunk {
			s.tokens -= chunk
			s.used += int64(chunk)
			n -= int(chunk)
			l.mu.Unlock()
			continue
		}
		s.waited = true
		wait := rebalanceEvery
		if s.alloc > 0 {
			if w := time.Duration((chunk - s.tokens) / s.alloc * float64(time.Second)); w < wait {
				wait = w
			}
		}
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		ch := l.changed
		l.mu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		case <-ch:
			t.Stop()
		}
	}
	return nil
}
