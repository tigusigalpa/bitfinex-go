package bitfinex

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/retry"
	"github.com/tigusigalpa/bitfinex-go/types"
	"github.com/tigusigalpa/bitfinex-go/websocket"
)

// Default endpoint bases. Override ONLY via the explicit options
// below (tests/proxy) — production defaults are HTTPS/WSS.
const (
	DefaultPublicRESTBase  = "https://api-pub.bitfinex.com/v2"
	DefaultPrivateRESTBase = "https://api.bitfinex.com/v2"
	DefaultPublicWSURL     = "wss://api-pub.bitfinex.com/ws/2"
	DefaultPrivateWSURL    = "wss://api.bitfinex.com/ws/2"
	DefaultUserAgent       = "bitfinex-go/1.2.0 (+https://github.com/tigusigalpa/bitfinex-go)"
)

// HTTPDoer is the narrow transport dependency — *http.Client
// satisfies it.
type HTTPDoer = httpx.HTTPDoer

// NonceProvider yields strictly increasing numeric nonces for
// signed requests. The default is process-local; inject a
// persistent/distributed provider for multi-process use.
type NonceProvider = auth.NonceProvider

// RateLimiter is the throttling contract. The default is an
// endpoint-aware token bucket; inject your own for shared or
// distributed limits.
type RateLimiter interface {
	Wait(ctx context.Context, bucket string) error
	Penalize(bucket string, retryAfterSeconds float64)
}

// Clock supplies the current time (nonce provider, tests).
type Clock = auth.Clock

// WebSocketDialer creates WebSocket connections. Inject a fake
// in tests or a custom dialer for proxies.
type WebSocketDialer = websocket.Dialer

type options struct {
	creds       *auth.Credentials
	httpDoer    HTTPDoer
	publicREST  string
	privateREST string
	publicWS    string
	privateWS   string
	timeout     time.Duration
	userAgent   string
	retry       retry.Policy
	limiter     RateLimiter
	limiterSet  bool
	rateLimits  map[string]float64
	nonce       NonceProvider
	clock       Clock
	logger      *slog.Logger
	wsDialer    WebSocketDialer
}

// Option configures a Client. Invalid options fail at
// construction — a misconfigured client is never built.
type Option func(*options) error

func defaultOptions() options {
	return options{
		publicREST:  DefaultPublicRESTBase,
		privateREST: DefaultPrivateRESTBase,
		publicWS:    DefaultPublicWSURL,
		privateWS:   DefaultPrivateWSURL,
		timeout:     30 * time.Second,
		userAgent:   DefaultUserAgent,
		retry:       retry.DefaultPolicy(),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// WithCredentials binds an API key/secret pair. Without it the
// client is public-only: private methods return
// ErrCredentialsMissing.
func WithCredentials(key, secret string) Option {
	return func(o *options) error {
		creds, err := auth.NewCredentials(key, secret)
		if err != nil {
			return err
		}
		o.creds = &creds
		return nil
	}
}

// WithHTTPClient injects an *http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) error {
		if c == nil {
			return &types.ValidationError{Field: "http_client", Message: "must not be nil"}
		}
		o.httpDoer = c
		return nil
	}
}

// WithHTTPDoer injects any narrow HTTPDoer (tests, proxies,
// custom instrumentation).
func WithHTTPDoer(d HTTPDoer) Option {
	return func(o *options) error {
		if nilDependency(d) {
			return &types.ValidationError{Field: "http_doer", Message: "must not be nil"}
		}
		o.httpDoer = d
		return nil
	}
}

// WithPublicRESTBase overrides the public REST base URL.
// For tests/proxy only — the default is the official
// https://api-pub.bitfinex.com/v2.
func WithPublicRESTBase(u string) Option {
	return func(o *options) error {
		if err := validateHTTPBase(u, "public_rest_base"); err != nil {
			return err
		}
		o.publicREST = u
		return nil
	}
}

// WithPrivateRESTBase overrides the private REST base URL.
func WithPrivateRESTBase(u string) Option {
	return func(o *options) error {
		if err := validateHTTPBase(u, "private_rest_base"); err != nil {
			return err
		}
		o.privateREST = u
		return nil
	}
}

// WithPublicWSURL overrides the public WebSocket URL.
func WithPublicWSURL(u string) Option {
	return func(o *options) error {
		if err := validateWSBase(u, "public_ws_url"); err != nil {
			return err
		}
		o.publicWS = u
		return nil
	}
}

// WithPrivateWSURL overrides the private WebSocket URL.
func WithPrivateWSURL(u string) Option {
	return func(o *options) error {
		if err := validateWSBase(u, "private_ws_url"); err != nil {
			return err
		}
		o.privateWS = u
		return nil
	}
}

// WithTimeout sets the default request timeout applied to the
// built-in HTTP client. Ignored when a custom Doer is injected
// (configure timeouts there). Must be positive.
func WithTimeout(d time.Duration) Option {
	return func(o *options) error {
		if d <= 0 {
			return &types.ValidationError{Field: "timeout", Message: "must be positive"}
		}
		o.timeout = d
		return nil
	}
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(o *options) error {
		if ua == "" || strings.ContainsAny(ua, "\r\n") {
			return &types.ValidationError{Field: "user_agent", Message: "must not be empty"}
		}
		o.userAgent = ua
		return nil
	}
}

// WithRetryPolicy overrides the retry policy. Retries apply ONLY
// to classified idempotent reads — writes are never retried.
func WithRetryPolicy(maxAttempts int, base, maxDelay, maxElapsed time.Duration) Option {
	return func(o *options) error {
		if maxAttempts < 1 || base < 0 || maxDelay < 0 || maxElapsed < 0 {
			return &types.ValidationError{Field: "retry_policy", Message: "attempts>=1 and non-negative delays required"}
		}
		o.retry = retry.NewPolicy(maxAttempts, base, maxDelay, maxElapsed)
		return nil
	}
}

// WithRateLimiter injects a custom limiter (shared or
// distributed). Pass nil-aware: nil disables local throttling —
// NOT recommended for production.
func WithRateLimiter(l RateLimiter) Option {
	return func(o *options) error {
		if l != nil && nilDependency(l) {
			return &types.ValidationError{Field: "rate_limiter", Message: "typed nil dependency"}
		}
		o.limiter = l
		o.limiterSet = true
		return nil
	}
}

// WithRateLimitProfile overrides per-bucket requests/minute
// budgets of the default limiter (e.g. {"book": 45}).
func WithRateLimitProfile(overrides map[string]float64) Option {
	return func(o *options) error {
		for k, v := range overrides {
			if v < 1 || math.IsNaN(v) || math.IsInf(v, 0) {
				return &types.ValidationError{Field: "rate_limits", Message: fmt.Sprintf("bucket %q must be >0", k)}
			}
		}
		o.rateLimits = make(map[string]float64, len(overrides))
		for k, v := range overrides {
			o.rateLimits[k] = v
		}
		return nil
	}
}

// WithNonceProvider injects the nonce provider. REQUIRED for
// multi-process deployments on one API key — the default is
// process-local only.
func WithNonceProvider(p NonceProvider) Option {
	return func(o *options) error {
		if nilDependency(p) {
			return &types.ValidationError{Field: "nonce_provider", Message: "must not be nil"}
		}
		o.nonce = p
		return nil
	}
}

// WithClock injects the clock used by the default nonce
// provider. Tests only.
func WithClock(c Clock) Option {
	return func(o *options) error {
		if nilDependency(c) {
			return &types.ValidationError{Field: "clock", Message: "must not be nil"}
		}
		o.clock = c
		return nil
	}
}

// WithLogger injects a *slog.Logger. The client never logs
// credentials, signatures, nonces or request bodies — only
// endpoint names, statuses and retry metadata.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) error {
		if l == nil {
			return &types.ValidationError{Field: "logger", Message: "must not be nil"}
		}
		o.logger = l
		return nil
	}
}

// WithWSDialer injects a custom WebSocket dialer (tests/proxy).
func WithWSDialer(d WebSocketDialer) Option {
	return func(o *options) error {
		if nilDependency(d) {
			return &types.ValidationError{Field: "ws_dialer", Message: "must not be nil"}
		}
		o.wsDialer = d
		return nil
	}
}

func nilDependency(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}

func validateHTTPBase(u, field string) error {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return &types.ValidationError{Field: field, Message: "invalid base URL (userinfo, query and fragment are forbidden)"}
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return &types.ValidationError{Field: field, Message: fmt.Sprintf("scheme %q not allowed (want https; http only for tests/proxy)", parsed.Scheme)}
	}
	return nil
}

func validateWSBase(u, field string) error {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return &types.ValidationError{Field: field, Message: "invalid WS URL (userinfo, query and fragment are forbidden)"}
	}
	if parsed.Scheme != "wss" && parsed.Scheme != "ws" {
		return &types.ValidationError{Field: field, Message: fmt.Sprintf("scheme %q not allowed (want wss; ws only for tests/proxy)", parsed.Scheme)}
	}
	return nil
}
