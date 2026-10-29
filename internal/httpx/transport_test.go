package httpx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/ratelimit"
	"github.com/tigusigalpa/bitfinex-go/internal/retry"
	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func testCfg(doer *testutil.FakeDoer) Config {
	creds, _ := auth.NewCredentials("k", testSecret)
	return Config{
		Doer:          doer,
		PublicBase:    "https://pub.example/v2",
		PrivateBase:   "https://api.example/v2",
		Signer:        auth.NewSigner(creds),
		NonceProvider: auth.NewMillisecondNonceProvider(nil),
		Limiter:       ratelimit.New(map[string]float64{"*": 60000}, 0),
		Retry:         retry.NewPolicy(3, time.Millisecond, 5*time.Millisecond, time.Second),
	}
}

const testSecret = "s3cr3t"

func TestPostAuthSignsExactBytes(t *testing.T) {
	doer := &testutil.FakeDoer{Responses: nil}
	tr := New(testCfg(doer))
	r, err := tr.PostAuth(context.Background(), "v2/auth/r/wallets", map[string]any{"x": 1}, RequestContext{Bucket: "wallets", Idempotent: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = r
	req := doer.Requests[0]
	if req.Method != "POST" || req.URL != "https://api.example/v2/auth/r/wallets" {
		t.Fatalf("bad request: %s %s", req.Method, req.URL)
	}
	for _, h := range []string{"bfx-nonce", "bfx-apikey", "bfx-signature", "Content-Type"} {
		if req.Header.Get(h) == "" {
			t.Fatalf("missing header %s", h)
		}
	}
	// Signature must match the exact body bytes sent.
	sig, err := tr.cfg.Signer.Sign("v2/auth/r/wallets", req.Header.Get("bfx-nonce"), req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Signature != req.Header.Get("bfx-signature") {
		t.Fatal("signature does not cover transmitted bytes")
	}
}

func TestPublicGet(t *testing.T) {
	doer := &testutil.FakeDoer{}
	tr := New(testCfg(doer))
	r, err := tr.GetPublic(context.Background(), "platform/status", nil, RequestContext{Bucket: "public", Idempotent: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != 200 {
		t.Fatal("bad status")
	}
}

func TestErrorEnvelope(t *testing.T) {
	doer := &testutil.FakeDoer{}
	tr := New(testCfg(doer))
	// Bitfinex ["error", code, msg] on HTTP 200.
	doer.Responses = append(doer.Responses, testutil.JSONResponse(200, `["error",10100,"apikey: invalid"]`, nil))
	_, err := tr.PostAuth(context.Background(), "v2/auth/r/wallets", nil, RequestContext{Bucket: "w", Idempotent: true})
	var ae *types.AuthenticationError
	if !errors.As(err, &ae) {
		t.Fatalf("want AuthenticationError, got %T %v", err, err)
	}
	if ae.Code != 10100 {
		t.Fatal("code lost")
	}
}

func TestRateLimitErrorAndCooldown(t *testing.T) {
	doer := &testutil.FakeDoer{}
	lim := ratelimit.New(nil, time.Hour)
	cfg := testCfg(doer)
	cfg.Limiter = lim
	tr := New(cfg)
	doer.Responses = append(doer.Responses, testutil.JSONResponse(429, `["error",10010,"ERR_RATE_LIMIT"]`, map[string]string{"Retry-After": "5"}))
	_, err := tr.GetPublic(context.Background(), "ticker/tBTCUSD", nil, RequestContext{Bucket: "ticker", Idempotent: true})
	var rle *types.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("want RateLimitError, got %T", err)
	}
	// Bucket must now be in >=60s cooldown.
	if d := lim.AvailabilityDelay("ticker"); d < 50*time.Second {
		t.Fatalf("cooldown too short: %v", d)
	}
}

func TestNoRetryOnWrite(t *testing.T) {
	doer := &testutil.FakeDoer{FailErr: errConnReset}
	tr := New(testCfg(doer))
	// Non-idempotent write: transport error → exactly ONE attempt.
	_, err := tr.PostAuth(context.Background(), "v2/auth/w/order/submit", map[string]any{"a": 1}, RequestContext{Bucket: "orders-write", Idempotent: false})
	var te *types.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("want TransportError, got %T", err)
	}
	if len(doer.Requests) != 1 {
		t.Fatalf("write was retried: %d attempts", len(doer.Requests))
	}
}

func TestRetryOnIdempotentRead(t *testing.T) {
	doer := &testutil.FakeDoer{}
	tr := New(testCfg(doer))
	doer.Responses = append(doer.Responses,
		testutil.JSONResponse(503, `maintenance`, nil),
		testutil.JSONResponse(503, `maintenance`, nil),
		testutil.JSONResponse(200, `[1]`, nil),
	)
	r, err := tr.GetPublic(context.Background(), "platform/status", nil, RequestContext{Bucket: "public", Idempotent: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Attempts != 3 {
		t.Fatalf("attempts = %d", r.Attempts)
	}
}

func TestNoRetryOnAuthError(t *testing.T) {
	doer := &testutil.FakeDoer{}
	tr := New(testCfg(doer))
	doer.Responses = append(doer.Responses, testutil.JSONResponse(401, `["error",10100,"apikey: invalid"]`, nil))
	_, err := tr.GetPublic(context.Background(), "x", nil, RequestContext{Bucket: "p", Idempotent: true})
	if err == nil {
		t.Fatal("want error")
	}
	if len(doer.Requests) != 1 {
		t.Fatal("auth error retried")
	}
}

func TestMissingCredentials(t *testing.T) {
	doer := &testutil.FakeDoer{}
	cfg := testCfg(doer)
	cfg.Signer = nil
	cfg.NonceProvider = nil
	tr := New(cfg)
	_, err := tr.PostAuth(context.Background(), "v2/auth/r/wallets", nil, RequestContext{})
	if !errors.Is(err, types.ErrCredentialsMissing) {
		t.Fatalf("want ErrCredentialsMissing, got %v", err)
	}
}

func TestCtxCancelDuringLimiterWait(t *testing.T) {
	lim := ratelimit.New(map[string]float64{"*": 0.001}, time.Hour)
	lim.Penalize("book", 60)
	doer := &testutil.FakeDoer{}
	cfg := testCfg(doer)
	cfg.Limiter = lim
	tr := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := tr.GetPublic(ctx, "book/x/P0", nil, RequestContext{Bucket: "book", Idempotent: true})
	if !errors.Is(err, context.DeadlineExceeded) && err == nil {
		t.Fatalf("want ctx error, got %v", err)
	}
}

// errConnReset simulates a transient transport failure.
var errConnReset = &fakeNetErr{}

type fakeNetErr struct{}

func (e *fakeNetErr) Error() string   { return "connection reset by peer" }
func (e *fakeNetErr) Timeout() bool   { return false }
func (e *fakeNetErr) Temporary() bool { return true }

func TestBodySnippetBounded(t *testing.T) {
	long := strings.Repeat("x", 1000)
	doer := &testutil.FakeDoer{}
	tr := New(testCfg(doer))
	doer.Responses = append(doer.Responses, testutil.JSONResponse(500, long, nil))
	_, err := tr.GetPublic(context.Background(), "x", nil, RequestContext{Bucket: "p", Idempotent: true})
	var ae *types.APIError
	if errors.As(err, &ae) && len(ae.BodySnippet) > 400 {
		t.Fatalf("body snippet not bounded: %d", len(ae.BodySnippet))
	}
}
