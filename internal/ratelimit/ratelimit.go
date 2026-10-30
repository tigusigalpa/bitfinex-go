// Package ratelimit implements an endpoint-aware token bucket.
//
// Each endpoint maps to a named bucket with its own requests-per-
// minute budget (Bitfinex documents per-endpoint limits in the
// 10–90 req/min range; unknown buckets get a conservative
// default). State is process-local unless you inject shared
// storage through the RateLimiter interface.
//
// After a server-side ERR_RATE_LIMIT / HTTP 429 the bucket is
// cooled down for at least CooldownSeconds (60s), extended by
// Retry-After when the server sends one.
package ratelimit

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// CooldownSeconds is the minimum post-rate-limit cooldown —
// the documented IP-block window after ERR_RATE_LIMIT.
const CooldownSeconds = 60.0

// DefaultLimits are conservative per-bucket budgets
// (requests/minute). "*" is the fallback for undocumented
// endpoints. Budgets follow the official limits reference; where
// docs conflict we pick the lower published value.
func DefaultLimits() map[string]float64 {
	return map[string]float64{
		"*":               10,
		"platform-status": 30,
		"tickers":         90,
		"ticker":          30,
		"trades":          15,
		"book":            90,
		"stats":           15,
		"candles":         30,
		"liquidations":    3,
		"withdraw":        10,
	}
}

type bucket struct {
	tokens      float64
	refilledAt  time.Time
	cooldownEnd time.Time
	started     bool
}

// Limiter is a process-local, concurrency-safe token bucket per
// endpoint bucket. It blocks (honoring ctx) until a token is
// available, and applies a >=60s cooldown after server-side
// rate-limit rejection.
type Limiter struct {
	mu          sync.Mutex
	limits      map[string]float64
	buckets     map[string]*bucket
	now         func() time.Time
	maxWait     time.Duration
	cooldownEnd time.Time
}

// New builds a Limiter. overrides add or replace limits.
// maxWait bounds a single Wait call; <=0 uses 65s.
func New(overrides map[string]float64, maxWait time.Duration) *Limiter {
	limits := DefaultLimits()
	for k, v := range overrides {
		if v >= 1 && !math.IsInf(v, 0) && !math.IsNaN(v) {
			limits[k] = v
		}
	}
	if maxWait <= 0 {
		maxWait = 65 * time.Second
	}
	return &Limiter{
		limits:  limits,
		buckets: map[string]*bucket{},
		now:     time.Now,
		maxWait: maxWait,
	}
}

// SetNow overrides the clock (tests only).
func (l *Limiter) SetNow(f func() time.Time) { l.now = f }

// Wait blocks until a token for bucket is available or ctx
// ends. Returns *types.RateLimitError when the required wait
// would exceed the configured max wait.
func (l *Limiter) Wait(ctx context.Context, bucketName string) error {
	started := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		limit := l.limits[bucketName]
		if limit < 1 {
			limit = l.limits["*"]
		}
		b := l.bucketLocked(bucketName)
		now := l.now()
		l.refillLocked(b, limit, now)
		wait := b.cooldownEnd.Sub(now)
		if global := l.cooldownEnd.Sub(now); global > wait {
			wait = global
		}
		if b.tokens < 1 {
			need := time.Duration((1 - b.tokens) / (limit / 60) * float64(time.Second))
			if need < time.Nanosecond {
				need = time.Nanosecond
			}
			if need > wait {
				wait = need
			}
		}
		if wait <= 0 {
			b.tokens--
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		if wait > l.maxWait-time.Since(started) {
			return &types.RateLimitError{APIError: types.APIError{Message: "local rate limiter wait would exceed maximum", Endpoint: bucketName, RetryAfter: wait}}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		// Recheck under lock: another waiter or a new server penalty may
		// have changed availability while this caller was asleep.
	}
}

// Penalize applies a cooldown of at least CooldownSeconds after
// the server rejects with ERR_RATE_LIMIT/429. retryAfter
// (seconds) extends the cooldown when the server asks for more.
func (l *Limiter) Penalize(bucketName string, retryAfterSeconds float64) {
	if math.IsNaN(retryAfterSeconds) || math.IsInf(retryAfterSeconds, 0) {
		retryAfterSeconds = 0
	}
	if retryAfterSeconds > 86400 {
		retryAfterSeconds = 86400
	}
	cd := CooldownSeconds
	if retryAfterSeconds > cd {
		cd = retryAfterSeconds
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucketLocked(bucketName)
	until := l.now().Add(time.Duration(cd * float64(time.Second)))
	if until.After(b.cooldownEnd) {
		b.cooldownEnd = until
	}
	if until.After(l.cooldownEnd) {
		l.cooldownEnd = until
	}
	b.tokens = 0
}

// AvailabilityDelay reports how long until the bucket can serve
// a request — for diagnostics. Does not mutate state.
func (l *Limiter) AvailabilityDelay(bucketName string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucketLocked(bucketName)
	d := b.cooldownEnd.Sub(l.now())
	if global := l.cooldownEnd.Sub(l.now()); global > d {
		d = global
	}
	if d < 0 {
		return 0
	}
	return d
}

func (l *Limiter) bucketLocked(name string) *bucket {
	b := l.buckets[name]
	if b == nil {
		b = &bucket{}
		l.buckets[name] = b
	}
	return b
}

func (l *Limiter) refillLocked(b *bucket, limit float64, now time.Time) {
	if !b.started {
		b.started = true
		b.tokens = limit
		b.refilledAt = now
		return
	}
	elapsed := now.Sub(b.refilledAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	b.tokens += elapsed * (limit / 60.0)
	if b.tokens > limit {
		b.tokens = limit
	}
	b.refilledAt = now
}
