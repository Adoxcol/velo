package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Limiter is a shared bandwidth budget for all downloads (a global
// speed limit). The rate can be changed at any time; 0 means unlimited.
type Limiter struct {
	rate atomic.Int64 // bytes per second
	mu   sync.Mutex
	next time.Time // when the budget is next free
}

// burst is how far behind "now" the budget may fall, allowing short bursts.
const burst = 250 * time.Millisecond

func (l *Limiter) SetRate(bytesPerSec int64) { l.rate.Store(max(bytesPerSec, 0)) }
func (l *Limiter) Rate() int64               { return l.rate.Load() }

// chunk is the read size to use, so limited downloads move in small steps
// and stay responsive to pause.
func (l *Limiter) chunk(bufLen int) int {
	if l == nil {
		return bufLen
	}
	if r := l.rate.Load(); r > 0 {
		return int(min(int64(bufLen), max(r/10, 4096)))
	}
	return bufLen
}

// Wait blocks until n bytes fit in the budget or ctx ends.
func (l *Limiter) Wait(ctx context.Context, n int) error {
	if l == nil {
		return nil
	}
	r := l.rate.Load()
	if r <= 0 || n <= 0 {
		return nil
	}
	l.mu.Lock()
	now := time.Now()
	if floor := now.Add(-burst); l.next.Before(floor) {
		l.next = floor
	}
	l.next = l.next.Add(time.Duration(int64(n) * int64(time.Second) / r))
	wait := l.next.Sub(now)
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
