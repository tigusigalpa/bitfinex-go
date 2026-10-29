package auth

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// MaxNonce is the largest nonce Bitfinex accepts (2^53-1 —
// Number.MAX_SAFE_INTEGER, per the official requirements doc).
const MaxNonce = 9007199254740991

// NonceProvider yields a strictly increasing numeric nonce for
// each signed request on a given API key. Nonces must never
// repeat or decrease for the same key, even across restarts.
type NonceProvider interface {
	Next(ctx context.Context) (string, error)
}

// Clock supplies the current time. It exists so tests can pin
// time deterministically; production uses SystemClock.
type Clock interface {
	Now() time.Time
}

// SystemClock is the wall-clock Clock.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now() }

// MillisecondNonceProvider is the default process-local,
// concurrency-safe provider:
//
//	next = max(nowMillis, last+1)
//
// It satisfies single-process monotonicity under concurrency,
// but it does NOT coordinate across processes, machines, or
// restarts. If you run multiple services or horizontal replicas
// on one API key, inject a persistent/distributed NonceProvider
// (e.g. backed by a DB sequence or Redis INCR) — or, as the
// official docs recommend, issue a separate API key per
// independent client.
type MillisecondNonceProvider struct {
	clock Clock

	mu   sync.Mutex
	last int64
}

// NewMillisecondNonceProvider returns the default provider.
// If clock is nil, SystemClock is used.
func NewMillisecondNonceProvider(clock Clock) *MillisecondNonceProvider {
	if clock == nil {
		clock = SystemClock{}
	}
	return &MillisecondNonceProvider{clock: clock}
}

// Next returns a strictly increasing nonce string.
func (p *MillisecondNonceProvider) Next(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	now := p.clock.Now().UnixMilli()
	next := now
	if next <= p.last {
		next = p.last + 1
	}
	if next > MaxNonce {
		return "", &types.NonceError{Message: "nonce exhausted (>2^53-1); generate a new API key"}
	}
	p.last = next
	return itoa(next), nil
}

// FuncNonceProvider adapts a function to NonceProvider — handy
// for tests and for distributed implementations.
type FuncNonceProvider func(ctx context.Context) (string, error)

// Next implements NonceProvider.
func (f FuncNonceProvider) Next(ctx context.Context) (string, error) { return f(ctx) }

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
