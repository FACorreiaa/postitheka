// Package ratelimit bounds how often one caller may do one thing.
//
// The state is in this process's memory. That is right while one replica
// serves the API: a second replica would give each caller a second allowance,
// and the limits would then belong in Postgres or at the ingress.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Limiter is a token bucket per key. A key starts with a full bucket of
// `limit` tokens and regains them evenly over `per`, so a caller may spend its
// whole allowance at once and is then held to the average rate. That fits
// people: someone attaching ten photos sends ten requests in a second, and
// should not be refused for it.
type Limiter struct {
	mu      sync.Mutex
	burst   float64
	perSec  float64
	buckets map[string]*bucket
	swept   time.Time
	now     func() time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// New allows `limit` actions per `per` for each key. A limit below one would
// refuse everything, so it is raised to one.
func New(limit int, per time.Duration) *Limiter {
	if limit < 1 {
		limit = 1
	}
	return &Limiter{
		burst:   float64(limit),
		perSec:  float64(limit) / per.Seconds(),
		buckets: map[string]*bucket{},
		now:     time.Now,
	}
}

// Allow spends one token for key. When there is none, it reports how long
// until there is: always at least a second, rounded up, because that is what
// goes in a Retry-After header and "retry in 0 seconds" invites a tight loop.
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst}
		l.buckets[key] = b
	} else {
		b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.perSec)
	}
	b.seen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration(math.Ceil((1-b.tokens)/l.perSec)) * time.Second
	return false, max(wait, time.Second)
}

// sweep forgets keys whose bucket has had time to refill: they would be
// treated as new anyway. Without it, every address that ever sent a request
// with a bad token would be remembered until the process restarted.
func (l *Limiter) sweep(now time.Time) {
	full := time.Duration(l.burst / l.perSec * float64(time.Second))
	if now.Sub(l.swept) < full {
		return
	}
	l.swept = now
	for key, b := range l.buckets {
		if now.Sub(b.seen) >= full {
			delete(l.buckets, key)
		}
	}
}
