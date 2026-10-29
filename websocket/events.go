package websocket

import (
	"github.com/tigusigalpa/bitfinex-go/funding"
	"github.com/tigusigalpa/bitfinex-go/market"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/types"
	"github.com/tigusigalpa/bitfinex-go/wallets"
	"strconv"
)

// Kind classifies a subscription event.
type Kind int

const (
	// KindSnapshot is the initial state image after subscribing
	// (book levels, trade batch, auth snapshot sets). For book
	// channels the local integrity book is rebuilt from it.
	KindSnapshot Kind = iota
	// KindUpdate is an incremental update/delta.
	KindUpdate
	// KindHeartbeat is a server "hb" frame.
	KindHeartbeat
	// KindInfo carries lifecycle facts: subscribed,
	// unsubscribed, conf result, resubscribed, paused, pong.
	KindInfo
	// KindChecksum confirms a verified order-book CRC32.
	KindChecksum
	// KindError delivers a typed error (ErrConsumerTooSlow,
	// SequenceGapError, ChecksumError, WebSocketError, server
	// error events). After ErrConsumerTooSlow the subscription
	// channel is closed.
	KindError
)

// String describes the event kind.
func (k Kind) String() string {
	switch k {
	case KindSnapshot:
		return "snapshot"
	case KindUpdate:
		return "update"
	case KindHeartbeat:
		return "heartbeat"
	case KindInfo:
		return "info"
	case KindChecksum:
		return "checksum"
	case KindError:
		return "error"
	}
	return "unknown"
}

// Event is one typed message on a subscription. Exactly one of
// the payload fields is populated for data events; Raw always
// preserves the decoded wire payload as an escape hatch.
type Event struct {
	Kind    Kind   `json:"kind"`
	ChanID  int64  `json:"chan_id"`
	Channel string `json:"channel,omitempty"` // "ticker","trades","book","candles","status","auth"
	Symbol  string `json:"symbol,omitempty"`

	// Action details KindInfo events ("subscribed",
	// "unsubscribed", "conf", "resubscribed", "paused",
	// "resumed", "pong").
	Action string `json:"action,omitempty"`

	// Public channel payloads.
	Ticker        *market.Ticker         `json:"ticker,omitempty"`
	Trades        []market.Trade         `json:"trades,omitempty"`
	FundingTrades []funding.FundingTrade `json:"funding_trades,omitempty"` // fte/ftu
	BookLevel     *market.BookLevel      `json:"book_level,omitempty"`
	BookSnapshot  []market.BookLevel     `json:"book_snapshot,omitempty"`
	Candles       []market.Candle        `json:"candles,omitempty"`
	Status        any                    `json:"status,omitempty"`

	// Auth channel payload (chanID 0).
	Auth *AuthEvent `json:"auth,omitempty"`

	// Err is set on KindError events.
	Err error `json:"-"`

	// Raw is the decoded wire payload (escape hatch for fields
	// the typed layer does not model yet).
	Raw any `json:"raw,omitempty"`
}

// AuthEvent is a typed account-stream record on chanID 0.
// Stream is the two/three-letter stream tag; exactly one of the
// typed fields below is populated based on it.
type AuthEvent struct {
	Stream string `json:"stream"`

	Orders        []orders.Order          `json:"orders,omitempty"`         // os/on/ou/oc
	Positions     []positions.Position    `json:"positions,omitempty"`      // ps/pn/pu/pc
	Trades        []orders.TradeExecution `json:"trades,omitempty"`         // te/tu
	Offers        []funding.FundingOffer  `json:"offers,omitempty"`         // fos/fon/fou/foc
	Credits       []funding.FundingCredit `json:"credits,omitempty"`        // fcs/fcn/fcu/fcc
	Loans         []funding.FundingLoan   `json:"loans,omitempty"`          // fls/fln/flu/flc
	Wallets       []wallets.Wallet        `json:"wallets,omitempty"`        // ws/wu
	FundingTrades []funding.FundingTrade  `json:"funding_trades,omitempty"` // fte/ftu
	Notification  *orders.Notification    `json:"notification,omitempty"`   // n

	// Info carries the bu/miu/fiu payloads (shape varies by
	// key; preserved untyped).
	Info any `json:"info,omitempty"`

	// Raw is the inner payload as decoded.
	Raw any `json:"raw,omitempty"`
}

// IsSnapshot reports whether the stream tag is a snapshot
// (os, ps, ws, fos, fcs, fls) rather than a delta.
func (e *AuthEvent) IsSnapshot() bool {
	switch e.Stream {
	case "os", "ps", "ws", "fos", "fcs", "fls":
		return true
	}
	return false
}

// SubscriptionRequest describes one public channel subscription.
type SubscriptionRequest struct {
	Channel string // "ticker","trades","book","candles","status"
	Symbol  string // e.g. "tBTCUSD" (ticker/trades/book)
	Prec    string // book precision P0..P4 or R0
	Freq    string // book frequency F0/F1
	Len     int    // book length hint (e.g. 25, 100)
	Key     string // candles/status key, e.g. "trade:1m:tBTCUSD"
}

// SubscribeTicker builds a ticker subscription request.
func SubscribeTicker(symbol string) SubscriptionRequest {
	return SubscriptionRequest{Channel: "ticker", Symbol: symbol}
}

// SubscribeTrades builds a trades subscription request.
func SubscribeTrades(symbol string) SubscriptionRequest {
	return SubscriptionRequest{Channel: "trades", Symbol: symbol}
}

// SubscribeBook builds a book subscription request.
// prec: "P0"–"P4" aggregated or "R0" raw; freq: "F0"/"F1";
// length: e.g. 25 or 100 (0 = server default).
func SubscribeBook(symbol, prec, freq string, length int) SubscriptionRequest {
	return SubscriptionRequest{Channel: "book", Symbol: symbol, Prec: prec, Freq: freq, Len: length}
}

// SubscribeCandles builds a candles subscription request.
// key e.g. "trade:1m:tBTCUSD".
func SubscribeCandles(key string) SubscriptionRequest {
	return SubscriptionRequest{Channel: "candles", Key: key}
}

// SubscribeStatus builds a status subscription request.
// key e.g. "deriv:tBTCF0:USTF0" or "liq:global".
func SubscribeStatus(key string) SubscriptionRequest {
	return SubscriptionRequest{Channel: "status", Key: key}
}

// validate enforces the local contract before sending.
func (r SubscriptionRequest) validate() error {
	switch r.Channel {
	case "ticker", "trades":
		if _, err := types.ParseSymbol(r.Symbol); err != nil {
			return err
		}
	case "book":
		switch r.Len {
		case 0, 1, 25, 100, 250:
		default:
			return &types.ValidationError{Field: "len", Message: "book length must be 1, 25, 100 or 250"}
		}
		sym, err := types.ParseSymbol(r.Symbol)
		if err != nil {
			return err
		}
		_ = sym
		switch r.Prec {
		case "P0", "P1", "P2", "P3", "P4", "R0":
		default:
			return &types.ValidationError{Field: "prec", Message: "book precision must be P0–P4 or R0"}
		}
		freq := r.Freq
		if freq == "" {
			freq = "F0" // server default when omitted
		}
		switch freq {
		case "F0", "F1":
		default:
			return &types.ValidationError{Field: "freq", Message: "book frequency must be F0 or F1"}
		}
	case "candles", "status":
		if r.Key == "" {
			return &types.ValidationError{Field: "key", Message: "subscription key required"}
		}
	default:
		return &types.ValidationError{Field: "channel", Message: "unknown channel " + r.Channel}
	}
	return nil
}

// message renders the subscribe wire payload.
func (r SubscriptionRequest) message() map[string]any {
	msg := map[string]any{"event": "subscribe", "channel": r.Channel}
	if r.Symbol != "" {
		msg["symbol"] = r.Symbol
	}
	if r.Prec != "" {
		msg["prec"] = r.Prec
	}
	if r.Freq != "" {
		msg["freq"] = r.Freq
	}
	if r.Len > 0 {
		msg["len"] = itoa64(int64(r.Len))
	}
	if r.Key != "" {
		msg["key"] = r.Key
	}
	return msg
}

// cacheKey identifies the request for ack matching and
// resubscribe bookkeeping.
func (r SubscriptionRequest) cacheKey() string {
	return r.Channel + "|" + r.Symbol + "|" + r.Prec + "|" + r.Freq + "|" + r.Key + "|" + strconv.Itoa(r.Len)
}

func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
