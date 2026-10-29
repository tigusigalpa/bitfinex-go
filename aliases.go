package bitfinex

import (
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Convenience re-exports — the canonical definitions live in
// the types package; these aliases keep the common surface in
// the root package.
type (
	// Decimal is the exact money/price/amount/rate type.
	Decimal = types.Decimal
	// Symbol is a validated market symbol ("tBTCUSD"/"fUSD").
	Symbol = types.Symbol
	// Millis is a Unix-milliseconds timestamp.
	Millis = types.Millis
	// ResponseMeta carries safe HTTP/rate-limit metadata. The
	// generic Response[T] container lives in types (generic
	// aliases need Go 1.23+; this module targets Go 1.22).
	ResponseMeta = types.ResponseMeta
	// HistoryQuery is the shared start/end/limit/sort window.
	HistoryQuery = types.HistoryQuery
	// KeyValueInfo is the loosely-typed /auth/r/info envelope.
	KeyValueInfo = types.KeyValueInfo

	ValidationError     = types.ValidationError
	APIError            = types.APIError
	RateLimitError      = types.RateLimitError
	AuthenticationError = types.AuthenticationError
	TransportError      = types.TransportError
	ProtocolError       = types.ProtocolError
	NonceError          = types.NonceError
	WebSocketError      = types.WebSocketError
	SequenceGapError    = types.SequenceGapError
	ChecksumError       = types.ChecksumError
)

// Sentinel errors (match with errors.Is).
var (
	ErrCredentialsMissing = types.ErrCredentialsMissing
	ErrConsumerTooSlow    = types.ErrConsumerTooSlow
	ErrClosed             = types.ErrClosed
)

// Decimal/symbol/timestamp constructors re-exported for
// convenience.
var (
	NewDecimal       = types.NewDecimal
	MustDecimal      = types.MustDecimal
	DecimalFromInt64 = types.DecimalFromInt64
	DecimalZero      = types.DecimalZero
	ParseSymbol      = types.ParseSymbol
	MustSymbol       = types.MustSymbol
	MillisFromTime   = types.MillisFromTime
	MillisNow        = types.MillisNow
)

// History sort orders.
const (
	SortAsc  = types.SortAsc
	SortDesc = types.SortDesc
)
