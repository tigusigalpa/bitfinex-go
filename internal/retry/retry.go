// Package retry implements capped exponential backoff with full
// jitter for explicitly idempotent requests only.
//
// NEVER retried: validation errors, auth errors, nonce errors,
// protocol errors, or ANY write operation (order submit/update/
// cancel, funding ops, transfers, withdrawals, invoices, alerts,
// settings). A write whose result is uncertain must be reconciled
// by the application (e.g. via cid), never repeated blindly.
package retry

import (
	"errors"
	"math/rand"
	"time"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// retryableStatus are HTTP statuses safe to retry when transient.
var retryableStatus = map[int]bool{408: true, 429: true, 500: true, 502: true, 503: true, 504: true}

// Policy bounds retries: max attempts, per-attempt delay cap,
// and a total elapsed cap.
type Policy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	MaxElapsed  time.Duration

	// randSource is injectable for deterministic tests.
	randSource *rand.Rand
	now        func() time.Time
}

// DefaultPolicy is the conservative production default.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts: 3,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    4 * time.Second,
		MaxElapsed:  30 * time.Second,
		now:         time.Now,
	}
}

// NewPolicy builds a Policy with explicit bounds for tests.
func NewPolicy(maxAttempts int, base, maxDelay, maxElapsed time.Duration) Policy {
	p := DefaultPolicy()
	if maxAttempts > 0 {
		p.MaxAttempts = maxAttempts
	}
	if base > 0 {
		p.BaseDelay = base
	}
	if maxDelay > 0 {
		p.MaxDelay = maxDelay
	}
	if maxElapsed > 0 {
		p.MaxElapsed = maxElapsed
	}
	return p
}

// SetRand overrides the jitter source (tests only).
func (p *Policy) SetRand(r *rand.Rand) { p.randSource = r }

// SetNow overrides the clock (tests only).
func (p *Policy) SetNow(f func() time.Time) { p.now = f }

// Retryable classifies whether err may be retried for a request
// whose idempotent flag is set. Non-idempotent (write) requests
// are never retried — an ambiguous outcome may have mutated
// server state already.
func (p Policy) Retryable(err error, idempotent bool) bool {
	if !idempotent || err == nil {
		return false
	}
	var ve *types.ValidationError
	var ae *types.AuthenticationError
	var ne *types.NonceError
	var pe *types.ProtocolError
	if errors.As(err, &ve) || errors.As(err, &ae) || errors.As(err, &ne) || errors.As(err, &pe) {
		return false
	}
	var te *types.TransportError
	if errors.As(err, &te) {
		return te.Temporary()
	}
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		return retryableStatus[apiErr.StatusCode]
	}
	return false
}

// DelayForAttempt returns the backoff before the next send —
// uniform jitter in [0, min(MaxDelay, Base*2^(attempt-1))].
// attempt is 1-based (delay before the SECOND send is attempt=1).
func (p Policy) DelayForAttempt(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	cap := p.BaseDelay << (attempt - 1)
	if cap <= 0 || cap > p.MaxDelay {
		cap = p.MaxDelay
	}
	if cap <= 0 {
		return 0
	}
	if p.randSource != nil {
		return time.Duration(p.randSource.Int63n(int64(cap) + 1))
	}
	return time.Duration(rand.Int63n(int64(cap) + 1))
}

// ElapsedExceeded reports whether the elapsed budget is spent.
func (p Policy) ElapsedExceeded(startedAt time.Time) bool {
	now := time.Now()
	if p.now != nil {
		now = p.now()
	}
	return now.Sub(startedAt) >= p.MaxElapsed
}
