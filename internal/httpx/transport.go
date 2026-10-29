// Package httpx is the internal HTTP transport: request
// construction, HMAC-SHA384 signing, endpoint-aware throttling,
// bounded retries for idempotent requests, and response/error
// classification.
//
// Signing contract: the JSON body is serialized exactly once;
// the identical bytes are fed to HMAC-SHA384 and sent as the
// request body. No secret, signature, nonce or bfx-* header is
// ever logged or embedded in an error.
package httpx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/retry"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// HTTPDoer is the narrow transport dependency — *http.Client
// satisfies it.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Limiter is the rate-limiter contract used by the transport.
// *ratelimit.Limiter satisfies it; inject your own for shared
// or distributed throttling.
type Limiter interface {
	Wait(ctx context.Context, bucket string) error
	Penalize(bucket string, retryAfterSeconds float64)
}

// RequestContext carries per-call routing metadata.
type RequestContext struct {
	Bucket     string // rate-limit bucket name
	Idempotent bool   // safe to retry after transient failure
}

// Result is the transport-level outcome of a call.
type Result struct {
	Data       any
	StatusCode int
	Headers    map[string]string
	Attempts   int
}

// Config bundles transport dependencies.
type Config struct {
	Doer          HTTPDoer
	PublicBase    string // e.g. https://api-pub.bitfinex.com/v2
	PrivateBase   string // e.g. https://api.bitfinex.com/v2
	Signer        *auth.Signer
	NonceProvider auth.NonceProvider
	Limiter       Limiter
	Retry         retry.Policy
	UserAgent     string
	Logger        *slog.Logger
}

// Transport executes REST calls against the v2 API.
type Transport struct {
	cfg Config
}

// New builds a Transport.
func New(cfg Config) *Transport {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Transport{cfg: cfg}
}

// Meta converts a Result to the public metadata type.
func Meta(r *Result) types.ResponseMeta {
	return types.ResponseMeta{StatusCode: r.StatusCode, Headers: r.Headers, Attempts: r.Attempts}
}

// GetPublic performs an unauthenticated GET on the public base.
func (t *Transport) GetPublic(ctx context.Context, path string, query map[string]string, rc RequestContext) (*Result, error) {
	u := strings.TrimRight(t.cfg.PublicBase, "/") + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			q.Set(k, v)
		}
		u += "?" + q.Encode()
	}
	return t.send(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	}, path, rc)
}

// PostPublic performs an unauthenticated JSON POST on the public
// base (calc endpoints).
func (t *Transport) PostPublic(ctx context.Context, path string, body any, rc RequestContext) (*Result, error) {
	u := strings.TrimRight(t.cfg.PublicBase, "/") + "/" + strings.TrimLeft(path, "/")
	rawBody, err := wire.Encode(body)
	if err != nil {
		return nil, err
	}
	return t.send(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(rawBody)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, path, rc)
}

// PostAuth performs a signed POST on the private base. apiPath is
// relative to /api/ including the version prefix, e.g.
// "v2/auth/r/wallets". The JSON body is serialized ONCE — the
// same bytes are signed and transmitted.
func (t *Transport) PostAuth(ctx context.Context, apiPath string, body any, rc RequestContext) (*Result, error) {
	if t.cfg.Signer == nil || t.cfg.NonceProvider == nil {
		return nil, types.ErrCredentialsMissing
	}
	base := strings.TrimRight(t.cfg.PrivateBase, "/")

	var rawBody []byte
	var err error
	if body == nil {
		rawBody = []byte("{}")
	} else {
		rawBody, err = wire.Encode(body)
		if err != nil {
			return nil, err
		}
	}

	// The request URL strips the "v2/" prefix: the private base
	// already ends in /v2.
	urlPath := strings.TrimPrefix(apiPath, "v2/")
	u := base + "/" + urlPath

	return t.send(ctx, func() (*http.Request, error) {
		// Fresh nonce per attempt: the signature input must match
		// the transmitted nonce exactly.
		nonce, err := t.cfg.NonceProvider.Next(ctx)
		if err != nil {
			return nil, &types.NonceError{Message: "nonce provider failed", Err: err}
		}
		sig, err := t.cfg.Signer.Sign(apiPath, nonce, rawBody)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(rawBody)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("bfx-nonce", sig.Nonce)
		req.Header.Set("bfx-apikey", sig.Key)
		req.Header.Set("bfx-signature", sig.Signature)
		return req, nil
	}, apiPath, rc)
}

// send runs the attempt loop: limiter wait → build request
// (signing happens inside build for signed calls) → do →
// classify → maybe retry with bounded jittered backoff.
func (t *Transport) send(ctx context.Context, build func() (*http.Request, error), endpoint string, rc RequestContext) (*Result, error) {
	startedAt := time.Now()
	var lastErr error

	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if rc.Bucket != "" && t.cfg.Limiter != nil {
			if err := t.cfg.Limiter.Wait(ctx, rc.Bucket); err != nil {
				return nil, err
			}
		}

		req, err := build()
		if err != nil {
			// Signing/nonce/validation failures are never retried.
			return nil, err
		}
		if t.cfg.UserAgent != "" {
			req.Header.Set("User-Agent", t.cfg.UserAgent)
		}

		resp, err := t.cfg.Doer.Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			lastErr = &types.TransportError{Op: req.Method, Endpoint: endpoint, Err: err}
		} else {
			raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			if readErr != nil {
				lastErr = &types.TransportError{Op: "read", Endpoint: endpoint, Err: readErr}
			} else {
				var res *Result
				res, lastErr = t.classify(resp, raw, endpoint, attempt, rc)
				if res != nil {
					return res, nil
				}
			}
		}

		if attempt >= t.cfg.Retry.MaxAttempts ||
			!t.cfg.Retry.Retryable(lastErr, rc.Idempotent) ||
			t.cfg.Retry.ElapsedExceeded(startedAt) {
			return nil, lastErr
		}

		delay := t.cfg.Retry.DelayForAttempt(attempt)
		t.cfg.Logger.Debug("retrying request",
			"endpoint", endpoint, "attempt", attempt+1, "delay", delay.String())
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// classify maps an HTTP response to a Result or to the typed
// error it must raise. The Bitfinex ["error",code,text] envelope
// is detected even on 2xx statuses.
func (t *Transport) classify(resp *http.Response, raw []byte, endpoint string, attempt int, rc RequestContext) (*Result, error) {
	status := resp.StatusCode
	summary := summarize(raw)
	retryAfter := retryAfterSeconds(resp.Header.Get("Retry-After"))

	var decoded any
	decodeErr := error(nil)
	decoded, decErr := wire.Decode(raw)
	if decErr != nil {
		decodeErr = decErr
	}

	// Bitfinex error envelope: ["error", <code:int>, "<text>"]
	var bfxCode int
	var bfxText string
	if arr, ok := decoded.([]any); ok && len(arr) >= 2 {
		if tag, _ := arr[0].(string); tag == "error" {
			if n, ok := arr[1].(interface{ String() string }); ok {
				if iv, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
					bfxCode = int(iv)
				}
			}
			if len(arr) >= 3 {
				bfxText, _ = arr[2].(string)
			}
			if bfxText == "" {
				bfxText = "Bitfinex API error"
			}
		}
	}

	if status >= 200 && status < 300 && bfxCode == 0 && decodeErr == nil {
		return &Result{
			Data:       decoded,
			StatusCode: status,
			Headers:    safeHeaders(resp),
			Attempts:   attempt,
		}, nil
	}

	message := bfxText
	if message == "" {
		if decodeErr != nil && status >= 200 && status < 300 {
			message = "undecodable 2xx body"
		} else {
			message = fmt.Sprintf("HTTP %d", status)
		}
	}
	base := types.APIError{
		StatusCode:  status,
		Code:        bfxCode,
		Message:     message,
		Endpoint:    endpoint,
		Attempts:    attempt,
		BodySnippet: summary,
		RetryAfter:  time.Duration(retryAfter * float64(time.Second)),
	}

	isRateLimit := status == 429 || bfxCode == 10010 || strings.Contains(bfxText, "ERR_RATE_LIMIT")
	if isRateLimit {
		if rc.Bucket != "" && t.cfg.Limiter != nil {
			t.cfg.Limiter.Penalize(rc.Bucket, retryAfter)
		}
		return nil, &types.RateLimitError{APIError: base}
	}

	if status == 401 || status == 403 || bfxCode == 10100 {
		return nil, &types.AuthenticationError{APIError: base}
	}

	if decodeErr != nil {
		if status >= 200 && status < 300 {
			return nil, &types.ProtocolError{Message: "undecodable 2xx body on " + endpoint + ": " + decodeErr.Error()}
		}
		return nil, &types.APIError{StatusCode: status, Message: message, Endpoint: endpoint, Attempts: attempt, BodySnippet: summary}
	}

	return nil, &base
}

// safeHeaders keeps only response headers (server-generated;
// cannot contain request secrets).
func safeHeaders(resp *http.Response) map[string]string {
	out := make(map[string]string, len(resp.Header))
	for name, values := range resp.Header {
		out[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	return out
}

func retryAfterSeconds(h string) float64 {
	if h == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(h), 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

// summarize returns a bounded single-line digest of a response
// body for errors. Response bodies cannot contain request
// secrets by construction.
func summarize(raw []byte) string {
	one := strings.Join(strings.Fields(string(raw)), " ")
	if len(one) > 300 {
		return one[:300] + "…"
	}
	return one
}
