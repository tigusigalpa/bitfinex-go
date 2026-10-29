package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestUntrustedResponsesAreRedacted(t *testing.T) {
	for _, status := range []int{200, 400, 401, 429, 500} {
		d := &testutil.FakeDoer{}
		cfg := testCfg(d)
		cfg.Retry.MaxAttempts = 1
		d.Responses = append(d.Responses, testutil.JSONResponse(status, `["error",10020,"sensitive-echo-from-server"]`, nil))
		_, err := New(cfg).PostAuth(context.Background(), "v2/auth/r/wallets", nil, RequestContext{})
		if err == nil || strings.Contains(err.Error(), "sensitive-echo") {
			t.Fatalf("unsafe error: %v", err)
		}
		var api *types.APIError
		if !errors.As(err, &api) || strings.Contains(api.Message+api.BodySnippet, "sensitive-echo") {
			t.Fatal("unsafe API error metadata")
		}
	}
	resp := testutil.JSONResponse(200, `[]`, map[string]string{"Set-Cookie": "session=private", "Authorization": "private", "Bfx-Apikey": "private", "X-Debug": "private", "Retry-After": "60"})
	h := safeHeaders(resp)
	if len(h) != 1 || h["retry-after"] != "60" {
		t.Fatalf("unsafe headers: %v", h)
	}
}

func TestMalformedAndOversizedResponse(t *testing.T) {
	for _, body := range []string{`[] []`, `["error",0,"failure"]`, strings.Repeat(" ", 4<<20) + `[]`} {
		d := &testutil.FakeDoer{Responses: []*http.Response{testutil.JSONResponse(200, body, nil)}}
		cfg := testCfg(d)
		cfg.Retry.MaxAttempts = 1
		if _, err := New(cfg).GetPublic(context.Background(), "test", nil, RequestContext{}); err == nil {
			t.Fatal("accepted invalid response")
		}
	}
}

func TestRetryAfterDatesAndInvalidNumbers(t *testing.T) {
	for _, s := range []string{"NaN", "+Inf", "-1", "garbage"} {
		if retryAfterSeconds(s) != 0 {
			t.Fatalf("accepted %q", s)
		}
	}
	got := retryAfterSeconds(time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat))
	if got < 118 || got > 121 {
		t.Fatalf("HTTP date retry-after=%v", got)
	}
}

func TestRetryBudgetIncludesLimiterWait(t *testing.T) {
	d := &testutil.FakeDoer{}
	cfg := testCfg(d)
	cfg.Retry.MaxElapsed = 20 * time.Millisecond
	cfg.Limiter = blockingLimiter{}
	start := time.Now()
	_, err := New(cfg).GetPublic(context.Background(), "test", nil, RequestContext{Bucket: "test", Idempotent: true})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second || len(d.Requests) != 0 {
		t.Fatal("retry elapsed budget did not bound limiter wait")
	}
}

type blockingLimiter struct{}

func (blockingLimiter) Wait(ctx context.Context, _ string) error { <-ctx.Done(); return ctx.Err() }
func (blockingLimiter) Penalize(string, float64)                 {}

func TestNoRetryForPermanentTransportError(t *testing.T) {
	d := &testutil.FakeDoer{FailErr: errors.New("permanent failure containing private data")}
	_, err := New(testCfg(d)).GetPublic(context.Background(), "test", nil, RequestContext{Idempotent: true})
	if len(d.Requests) != 1 || strings.Contains(err.Error(), "private") {
		t.Fatal("permanent transport error retried or exposed")
	}
	if !errors.Is(err, d.FailErr) {
		t.Fatal("underlying error not available via errors.Is")
	}
}

func FuzzErrorEnvelope(f *testing.F) {
	for _, s := range []string{`["error",10010,"ERR_RATE_LIMIT"]`, `[]`, `null`, `{`, `["error",0,"text"]`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		cfg := testCfg(&testutil.FakeDoer{})
		cfg.Limiter = nil
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
		_, _ = New(cfg).classify(resp, []byte(body), "fuzz", 1, RequestContext{})
	})
}
