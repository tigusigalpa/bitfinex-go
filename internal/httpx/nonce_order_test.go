package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
)

type orderedDoer func(*http.Request) (*http.Response, error)

func (f orderedDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }

type blockedBody struct {
	io.Reader
	release <-chan struct{}
	ctx     context.Context
}

func (b *blockedBody) Read(p []byte) (int, error) {
	select {
	case <-b.release:
		return b.Reader.Read(p)
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}
func (*blockedBody) Close() error { return nil }

// A second client on the same key must not generate/send its nonce until the
// first response BODY has been consumed. Holding only through Do is insufficient.
func TestAuthenticatedExchangesOrderedAcrossTransports(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var generated, sent atomic.Int64
	nonces := auth.FuncNonceProvider(func(context.Context) (string, error) {
		return strconv.FormatInt(generated.Add(1), 10), nil
	})
	cfg := testCfg(&testutil.FakeDoer{})
	cfg.NonceProvider = nonces
	cfg.Doer = orderedDoer(func(r *http.Request) (*http.Response, error) {
		n := sent.Add(1)
		if r.Header.Get("bfx-nonce") != strconv.FormatInt(n, 10) {
			t.Error("nonce reached transport out of order")
		}
		resp := testutil.JSONResponse(200, `[1]`, nil)
		if n == 1 {
			resp.Body = &blockedBody{Reader: strings.NewReader(`[1]`), release: release, ctx: ctx}
			close(entered)
		}
		return resp, nil
	})
	first, second := New(cfg), New(cfg)
	call := func(tr *Transport) error {
		_, e := tr.PostAuth(ctx, "v2/auth/r/wallets", nil, RequestContext{})
		return e
	}
	done := make(chan error, 2)
	go func() { done <- call(first) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Cancellation while queued must neither consume a nonce nor reach Do.
	queued, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	defer stop()
	_, err := second.PostAuth(queued, "v2/auth/r/wallets", nil, RequestContext{})
	if !errors.Is(err, context.DeadlineExceeded) || generated.Load() != 1 || sent.Load() != 1 {
		t.Fatalf("queued exchange escaped gate: %v, generated=%d sent=%d", err, generated.Load(), sent.Load())
	}
	// Public calls and unrelated keys must continue while this key is busy.
	pub := testCfg(&testutil.FakeDoer{})
	if _, e := New(pub).GetPublic(ctx, "platform/status", nil, RequestContext{}); e != nil {
		t.Fatal(e)
	}
	creds, _ := auth.NewCredentials("unrelated-key", testSecret)
	pub.Signer = auth.NewSigner(creds)
	if _, e := New(pub).PostAuth(ctx, "v2/auth/r/wallets", nil, RequestContext{}); e != nil {
		t.Fatal(e)
	}
	go func() { done <- call(second) }()
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case e := <-done:
			if e != nil {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if generated.Load() != 2 || sent.Load() != 2 {
		t.Fatal("lost exchange")
	}
}

func TestAuthenticatedGateReleasedAfterFailures(t *testing.T) {
	for _, failure := range []string{"nonce", "network", "empty", "oversize", "invalid-json"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cfg := testCfg(&testutil.FakeDoer{})
			var nonceCalls, doCalls int
			cfg.NonceProvider = auth.FuncNonceProvider(func(context.Context) (string, error) {
				nonceCalls++
				if failure == "nonce" && nonceCalls == 1 {
					return "", errors.New("fixture failure")
				}
				return strconv.Itoa(nonceCalls), nil
			})
			cfg.Doer = orderedDoer(func(*http.Request) (*http.Response, error) {
				doCalls++
				if doCalls == 1 && failure != "nonce" {
					switch failure {
					case "network":
						return nil, errors.New("fixture failure")
					case "empty":
						return nil, nil
					case "oversize":
						return testutil.JSONResponse(200, strings.Repeat(" ", (4<<20)+1), nil), nil
					case "invalid-json":
						return testutil.JSONResponse(200, `invalid`, nil), nil
					}
				}
				return testutil.JSONResponse(200, `[1]`, nil), nil
			})
			tr := New(cfg)
			if _, e := tr.PostAuth(ctx, "v2/auth/w/order/submit", nil, RequestContext{}); e == nil {
				t.Fatal("failure not reported")
			}
			if nonceCalls != 1 {
				t.Fatal("write was retried")
			}
			if _, e := tr.PostAuth(ctx, "v2/auth/r/wallets", nil, RequestContext{}); e != nil {
				t.Fatalf("gate leaked: %v", e)
			}
		})
	}
}
