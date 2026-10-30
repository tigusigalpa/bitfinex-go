package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/ratelimit"
	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// For a non-idempotent call with available rate tokens, Done is first consulted
// by authflow.Acquire, after it registered the waiter. This barrier makes the
// request queue deterministic both before and after the cooldown fix.
type queuedContextV122 struct {
	context.Context
	queued chan struct{}
	once   sync.Once
}

func (c *queuedContextV122) Done() <-chan struct{} {
	c.once.Do(func() { close(c.queued) })
	return c.Context.Done()
}

func TestQueuedSignedRequestObservesNewCooldownBeforeNonce(t *testing.T) {
	for _, status := range []int{429, 200} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var clock atomic.Int64
			clock.Store(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC).UnixNano())
			lim := ratelimit.New(map[string]float64{"*": 60000}, time.Second)
			lim.SetNow(func() time.Time { return time.Unix(0, clock.Load()) })
			entered, release := make(chan struct{}), make(chan struct{})
			var sent, nonces atomic.Int64
			cfg := testCfg(&testutil.FakeDoer{})
			cfg.Limiter = lim
			cfg.NonceProvider = auth.FuncNonceProvider(func(context.Context) (string, error) { return strconv.FormatInt(nonces.Add(1), 10), nil })
			cfg.Doer = orderedDoer(func(r *http.Request) (*http.Response, error) {
				if sent.Add(1) == 1 {
					close(entered)
					select {
					case <-release:
					case <-r.Context().Done():
						return nil, r.Context().Err()
					}
					return testutil.JSONResponse(status, `["error",10010,"ERR_RATE_LIMIT"]`, map[string]string{"Retry-After": "90"}), nil
				}
				return testutil.JSONResponse(200, `[1]`, nil), nil
			})
			tr := New(cfg)
			call := func(ctx context.Context, bucket string) error {
				_, e := tr.PostAuth(ctx, "v2/auth/w/order/submit", nil, RequestContext{Bucket: bucket})
				return e
			}
			first, second := make(chan error, 1), make(chan error, 1)
			go func() { first <- call(ctx, "orders-write") }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			queued := &queuedContextV122{Context: ctx, queued: make(chan struct{})}
			// A different endpoint on the same key must also observe the IP penalty.
			go func() { second <- call(queued, "other-write") }()
			select {
			case <-queued.queued:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			close(release)
			for _, result := range []<-chan error{first, second} {
				select {
				case e := <-result:
					var rle *types.RateLimitError
					if !errors.As(e, &rle) {
						t.Fatalf("expected cooldown error: %v", e)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if sent.Load() != 1 || nonces.Load() != 1 {
				t.Fatalf("queued request bypassed cooldown: sent=%d nonces=%d", sent.Load(), nonces.Load())
			}
			if d := lim.AvailabilityDelay("other-write"); d != 90*time.Second {
				t.Fatalf("Retry-After/global penalty lost: %v", d)
			}
			// A limiter failure must release the key gate; no real 90-second sleep.
			clock.Add(int64(91 * time.Second))
			if e := call(ctx, "other-write"); e != nil {
				t.Fatalf("gate leaked after cooldown rejection: %v", e)
			}
			if sent.Load() != 2 || nonces.Load() != 2 {
				t.Fatal("request did not resume with a fresh nonce")
			}
		})
	}
}

type failingLimiterV122 struct {
	err   error
	calls int
}

func (l *failingLimiterV122) Wait(context.Context, string) error {
	l.calls++
	if l.calls == 1 {
		return l.err
	}
	return nil
}
func (*failingLimiterV122) Penalize(string, float64) {}

func TestLimiterFailureNeverRetriesAndReleasesAuthGate(t *testing.T) {
	for _, signed := range []bool{false, true} {
		t.Run(strconv.FormatBool(signed), func(t *testing.T) {
			original := &types.TransportError{Op: "limiter", Err: io.EOF} // retryable if mistaken for a sent read
			lim := &failingLimiterV122{err: original}
			doer := &testutil.FakeDoer{}
			cfg := testCfg(doer)
			cfg.Limiter = lim
			var nonces atomic.Int64
			cfg.NonceProvider = auth.FuncNonceProvider(func(context.Context) (string, error) { return strconv.FormatInt(nonces.Add(1), 10), nil })
			tr := New(cfg)
			call := func() error {
				rc := RequestContext{Bucket: "fixture", Idempotent: true}
				if signed {
					_, e := tr.PostAuth(context.Background(), "v2/auth/r/wallets", nil, rc)
					return e
				}
				_, e := tr.GetPublic(context.Background(), "platform/status", nil, rc)
				return e
			}
			if e := call(); !errors.Is(e, original) {
				t.Fatalf("limiter error lost: %v", e)
			}
			if lim.calls != 1 || nonces.Load() != 0 || len(doer.Requests) != 0 {
				t.Fatal("limiter failure caused a retry, nonce allocation or HTTP request")
			}
			if e := call(); e != nil {
				t.Fatal(e)
			}
			if lim.calls != 2 || len(doer.Requests) != 1 {
				t.Fatal("limiter failure leaked gate or double-charged token")
			}
		})
	}
}
