package websocket

import (
	"log/slog"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
)

// Integrity flags sent via {"event":"conf","flags":N} after
// connecting to the public endpoint.
const (
	// FlagSeqAll enables per-connection sequence numbers — every
	// message gains a trailing seq the client checks for gaps.
	FlagSeqAll = 65536
	// FlagChecksum enables order-book CRC32 checksum messages
	// ("cs") on book channels.
	FlagChecksum = 131072
)

// DefaultMaxSubscriptions is the conservative channel cap.
// The official docs currently disagree (25 vs 30); we use the
// lower published value and make it configurable.
const DefaultMaxSubscriptions = 25

// BackpressurePolicy decides what happens when a subscription's
// event buffer is full. Messages are never silently dropped.
type BackpressurePolicy int

const (
	// BackpressureStop (default) emits a final
	// ErrConsumerTooSlow error event and closes the
	// subscription — the consumer must resubscribe to continue.
	BackpressureStop BackpressurePolicy = iota
	// BackpressureBlock makes the read loop block until the
	// consumer drains. Simple, but a stalled subscription
	// stalls the whole connection (all channels share one
	// read loop); use only with fast consumers.
	BackpressureBlock
)

// AuthOptions controls the authenticated connection.
type AuthOptions struct {
	// Filter limits which account streams are delivered,
	// e.g. ["trading", "funding"] or ["trading-tBTCUSD"].
	Filter []string
	// DeadManSwitch sends dms:4 — the server cancels all
	// orders if the socket dies. OFF unless explicitly set.
	DeadManSwitch bool
}

// Config bundles WebSocket client dependencies.
type Config struct {
	// PublicURL/PrivateURL override the official endpoints
	// (tests/proxy only).
	PublicURL  string
	PrivateURL string

	// Signer and NonceProvider enable ConnectAuth. A fresh
	// nonce is pulled on EVERY connect/reconnect — the
	// provider must remain strictly monotonic for the key.
	Signer        *auth.Signer
	NonceProvider auth.NonceProvider

	// Dialer creates connections; nil → GorillaDialer{}.
	Dialer Dialer

	// Logger receives safe diagnostics (no secrets ever).
	Logger *slog.Logger

	// MaxSubscriptions caps active channels; zero →
	// DefaultMaxSubscriptions (25).
	MaxSubscriptions int

	// EventBuffer is each subscription's channel capacity;
	// zero → 256.
	EventBuffer int

	// Backpressure chooses the full-buffer behavior; zero →
	// BackpressureStop.
	Backpressure BackpressurePolicy

	// ReconnectBase/ReconnectCap bound the capped full-jitter
	// backoff between reconnect attempts.
	ReconnectBase time.Duration // zero → 500ms
	ReconnectCap  time.Duration // zero → 30s
}

func (c *Config) withDefaults() {
	if c.Dialer == nil {
		c.Dialer = GorillaDialer{}
	}
	if c.MaxSubscriptions <= 0 {
		c.MaxSubscriptions = DefaultMaxSubscriptions
	}
	if c.EventBuffer <= 0 {
		c.EventBuffer = 256
	}
	if c.ReconnectBase <= 0 {
		c.ReconnectBase = 500 * time.Millisecond
	}
	if c.ReconnectCap <= 0 {
		c.ReconnectCap = 30 * time.Second
	}
}
