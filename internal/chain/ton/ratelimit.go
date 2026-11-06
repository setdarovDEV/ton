package ton

import (
	"context"
	"sync"
	"time"
)

// rateLimiter is a small token bucket: tonapi.io allows ~1 rps without a key
// and more with one, so the client paces itself instead of hitting 429s.
type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	burst    int
	tokens   float64
	last     time.Time
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &rateLimiter{interval: time.Duration(float64(time.Second) / rps), burst: burst, tokens: float64(burst), last: time.Now()}
}

// Wait blocks until a token is available or ctx is done.
func (r *rateLimiter) Wait(ctx context.Context) error {
	for {
		r.mu.Lock()
		now := time.Now()
		r.tokens += float64(now.Sub(r.last)) / float64(r.interval)
		if r.tokens > float64(r.burst) {
			r.tokens = float64(r.burst)
		}
		r.last = now
		if r.tokens >= 1 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}
		wait := time.Duration((1 - r.tokens) * float64(r.interval))
		r.mu.Unlock()

		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
