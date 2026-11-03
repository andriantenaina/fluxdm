package engine

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// Limiter is a token-bucket bandwidth limiter whose rate can change at runtime.
// A rate of 0 means unlimited.
type Limiter struct {
	mu  sync.RWMutex
	lim *rate.Limiter
}

const limiterBurst = 1 << 20

// NewLimiter creates a limiter in bytes/second (0 = unlimited).
func NewLimiter(bytesPerSec int64) *Limiter {
	l := &Limiter{}
	l.SetRate(bytesPerSec)
	return l
}

// SetRate changes the rate in bytes/second (0 = unlimited).
func (l *Limiter) SetRate(bytesPerSec int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if bytesPerSec <= 0 {
		l.lim = nil
		return
	}
	burst := limiterBurst
	if bytesPerSec < int64(burst) {
		burst = int(bytesPerSec)
		if burst < 16<<10 {
			burst = 16 << 10
		}
	}
	l.lim = rate.NewLimiter(rate.Limit(bytesPerSec), burst)
}

// Rate returns the current rate (0 = unlimited).
func (l *Limiter) Rate() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.lim == nil {
		return 0
	}
	return int64(l.lim.Limit())
}

// WaitN blocks until n bytes may be consumed.
func (l *Limiter) WaitN(ctx context.Context, n int) error {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	lim := l.lim
	l.mu.RUnlock()
	if lim == nil {
		return nil
	}
	for n > 0 {
		chunk := n
		if chunk > lim.Burst() {
			chunk = lim.Burst()
		}
		if err := lim.WaitN(ctx, chunk); err != nil {
			return err
		}
		n -= chunk
	}
	return nil
}
