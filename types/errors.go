package types

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors. Match with errors.Is.
var (
	// ErrCredentialsMissing is returned by private endpoints on a
	// client constructed without credentials.
	ErrCredentialsMissing = errors.New("bitfinex: credentials required for authenticated endpoint")

	// ErrConsumerTooSlow is delivered on a subscription when the
	// consumer cannot drain events and the configured
	// BackpressurePolicy is Stop — no message is silently dropped.
	ErrConsumerTooSlow = errors.New("bitfinex: subscription consumer too slow")

	// ErrClosed is returned when using a closed client/connection.
	ErrClosed = errors.New("bitfinex: closed")
)

// ValidationError reports a client-side request that failed
// fail-closed validation before any network I/O. Never retried.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("bitfinex: invalid %s: %s", e.Field, e.Message)
	}
	return "bitfinex: validation error: " + e.Message
}

// APIError is a non-2xx response or a Bitfinex ["error",code,msg]
// envelope. BodySnippet is bounded and contains only response
// bytes — request secrets can never appear here.
type APIError struct {
	StatusCode  int           // HTTP status (0 if not applicable)
	Code        int           // Bitfinex numeric error code (0 if none)
	Message     string        // server text (never request data)
	Endpoint    string        // safe endpoint path, e.g. "v2/auth/r/wallets"
	Attempts    int           // attempts made before this error surfaced
	BodySnippet string        // size-limited one-line response digest
	RetryAfter  time.Duration // parsed Retry-After, if present
}

func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString("bitfinex: api error")
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, " status=%d", e.StatusCode)
	}
	if e.Code != 0 {
		fmt.Fprintf(&b, " code=%d", e.Code)
	}
	if e.Endpoint != "" {
		fmt.Fprintf(&b, " endpoint=%s", e.Endpoint)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	return b.String()
}

// RateLimitError is an APIError raised for HTTP 429 / Bitfinex
// code 10010 (ERR_RATE_LIMIT). The client applies at least a
// 60-second cooldown to the affected bucket after this error.
type RateLimitError struct {
	APIError
}

// AuthenticationError is an APIError for 401/403 or Bitfinex
// auth failures (e.g. invalid key, bad signature, stale nonce).
// Never retried automatically.
type AuthenticationError struct {
	APIError
}

// TransportError wraps a network-layer failure (DNS, connect,
// TLS, read/write timeout). Unwrap yields the underlying error.
type TransportError struct {
	Op       string // e.g. "POST", "dial", "read"
	Endpoint string // safe endpoint identifier
	Err      error  // underlying error; never contains secrets
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("bitfinex: transport error on %s %s: %v", e.Op, e.Endpoint, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// Temporary reports whether the failure looks transient
// (timeout / temporary network error) and is safe to retry for
// idempotent requests.
func (e *TransportError) Temporary() bool {
	var ne interface{ Temporary() bool }
	if errors.As(e.Err, &ne) {
		return ne.Temporary()
	}
	var te interface{ Timeout() bool }
	if errors.As(e.Err, &te) {
		return te.Timeout()
	}
	// Connection refused/reset and DNS failures are treated as
	// transient for idempotent reads.
	return true
}

// ProtocolError reports a wire-contract violation: malformed
// positional arrays, unexpected types, undecodable bodies.
// Never retried; indicates a server change or a library bug.
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string { return "bitfinex: protocol error: " + e.Message }

// NonceError reports a nonce provider failure (non-monotonic
// output, out-of-range value, backend error). Never retried.
type NonceError struct {
	Message string
	Err     error
}

func (e *NonceError) Error() string {
	if e.Err != nil {
		return "bitfinex: nonce error: " + e.Message + ": " + e.Err.Error()
	}
	return "bitfinex: nonce error: " + e.Message
}

func (e *NonceError) Unwrap() error { return e.Err }

// WebSocketError reports a WebSocket-layer failure (dial, read,
// write, unexpected close, server error event).
type WebSocketError struct {
	Op      string // "dial", "read", "write", "close", "event"
	Message string
	Err     error
}

func (e *WebSocketError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("bitfinex: websocket %s error: %s: %v", e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("bitfinex: websocket %s error: %s", e.Op, e.Message)
}

func (e *WebSocketError) Unwrap() error { return e.Err }

// SequenceGapError reports a missed WebSocket sequence number
// (with SEQ_ALL enabled). The affected channel must be resynced.
type SequenceGapError struct {
	ChanID   int64
	Expected int64
	Got      int64
}

func (e *SequenceGapError) Error() string {
	return fmt.Sprintf("bitfinex: sequence gap on channel %d: expected seq %d, got %d",
		e.ChanID, e.Expected, e.Got)
}

// ChecksumError reports a failed order-book CRC32 checksum. The
// book must be discarded and resynced via re-subscription.
type ChecksumError struct {
	ChanID   int64
	Expected int64 // locally computed (signed CRC32)
	Got      int64 // server 'cs' value
}

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("bitfinex: book checksum mismatch on channel %d: computed %d, server %d",
		e.ChanID, e.Expected, e.Got)
}
