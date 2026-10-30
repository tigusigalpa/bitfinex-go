// Package orders implements the authenticated order endpoints:
// active orders, submit/update/cancel (single, multi, multi-op),
// order history, order trades, your trades, OTC order history.
//
// Write endpoints are NEVER automatically retried — an ambiguous
// outcome may have created/changed server-side state. Reconcile
// through ClientOrderID (cid/cid_date) instead.
package orders

import (
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Order flags (bitmask) per the official flag-values reference.
const (
	FlagHidden     = 64
	FlagClose      = 512
	FlagReduceOnly = 1024
	FlagPostOnly   = 4096
	FlagOCO        = 16384
	FlagNoVarRates = 524288
)

// Order type strings. The API may add types, so this is a
// constant bag, not a closed enum — unknown server values are
// preserved as strings.
const (
	TypeLimit                = "LIMIT"
	TypeExchangeLimit        = "EXCHANGE LIMIT"
	TypeMarket               = "MARKET"
	TypeExchangeMarket       = "EXCHANGE MARKET"
	TypeStop                 = "STOP"
	TypeExchangeStop         = "EXCHANGE STOP"
	TypeStopLimit            = "STOP LIMIT"
	TypeExchangeStopLimit    = "EXCHANGE STOP LIMIT"
	TypeTrailingStop         = "TRAILING STOP"
	TypeExchangeTrailingStop = "EXCHANGE TRAILING STOP"
	TypeFOK                  = "FOK"
	TypeExchangeFOK          = "EXCHANGE FOK"
	TypeIOC                  = "IOC"
	TypeExchangeIOC          = "EXCHANGE IOC"
)

// Order is one order positional row (25+ documented indices):
// [ID, GID, CID, SYMBOL, MTS_CREATE, MTS_UPDATE, AMOUNT,
//
//	AMOUNT_ORIG, TYPE, TYPE_PREV, MTS_TIF, _, FLAGS, STATUS, _, _,
//	PRICE, PRICE_AVG, PRICE_TRAILING, PRICE_AUX_LIMIT, _, _, _,
//	NOTIFY, HIDDEN, PLACED_ID, _, _, ROUTING, _, _, META].
type Order struct {
	ID            int64         `json:"id"`
	GID           int64         `json:"gid"`
	CID           int64         `json:"cid"`
	Symbol        types.Symbol  `json:"symbol"`
	MTSCreate     types.Millis  `json:"mts_create"`
	MTSUpdate     types.Millis  `json:"mts_update"`
	Amount        types.Decimal `json:"amount"`
	AmountOrig    types.Decimal `json:"amount_orig"`
	Type          string        `json:"type"`
	TypePrev      string        `json:"type_prev,omitempty"`
	MTSTif        types.Millis  `json:"mts_tif,omitempty"`
	Flags         int64         `json:"flags"`
	Status        string        `json:"status"`
	Price         types.Decimal `json:"price"`
	PriceAvg      types.Decimal `json:"price_avg"`
	PriceTrailing types.Decimal `json:"price_trailing"`
	PriceAuxLimit types.Decimal `json:"price_aux_limit"`
	Notify        bool          `json:"notify"`
	Hidden        bool          `json:"hidden"`
	PlacedID      int64         `json:"placed_id"`
	Routing       string        `json:"routing,omitempty"`
	// Meta preserves the untyped trailing metadata object/array.
	Meta any `json:"meta,omitempty"`
}

// FromWire decodes an order positional array. Exported so the
// WebSocket account stream (os/on/ou/oc events) can reuse it.
func FromWire(a []any) (Order, error) {
	o := Order{}
	if err := wire.AssertMinLength(a, 25, "order"); err != nil {
		return o, err
	}
	var err error
	if o.ID, err = wire.RequiredID(a, 0); err != nil {
		return o, err
	}
	if o.GID, err = wire.OptionalInt(a, 1); err != nil {
		return o, err
	}
	if o.CID, err = wire.OptionalInt(a, 2); err != nil {
		return o, err
	}
	if sym, serr := wire.Str(a, 3); serr != nil {
		return o, serr
	} else {
		o.Symbol = types.Symbol(sym)
	}
	if mts, _, merr := wire.Mts(a, 4); merr != nil {
		return o, merr
	} else {
		o.MTSCreate = mts
	}
	if mts, _, merr := wire.Mts(a, 5); merr != nil {
		return o, merr
	} else {
		o.MTSUpdate = mts
	}
	if o.Amount, err = wire.DecOr(a, 6, types.Decimal{}); err != nil {
		return o, err
	}
	if o.AmountOrig, err = wire.DecOr(a, 7, types.Decimal{}); err != nil {
		return o, err
	}
	if o.Type, err = wire.Str(a, 8); err != nil {
		return o, err
	}
	if o.TypePrev, err = wire.Str(a, 9); err != nil {
		return o, err
	}
	if mts, _, merr := wire.Mts(a, 10); merr != nil {
		return o, merr
	} else {
		o.MTSTif = mts
	}
	o.Flags = wire.IntOr(a, 12, 0)
	if o.Status, err = wire.Str(a, 13); err != nil {
		return o, err
	}
	if o.Price, err = wire.DecOr(a, 16, types.Decimal{}); err != nil {
		return o, err
	}
	if o.PriceAvg, err = wire.DecOr(a, 17, types.Decimal{}); err != nil {
		return o, err
	}
	if o.PriceTrailing, err = wire.DecOr(a, 18, types.Decimal{}); err != nil {
		return o, err
	}
	if o.PriceAuxLimit, err = wire.DecOr(a, 19, types.Decimal{}); err != nil {
		return o, err
	}
	if o.Notify, err = wire.Bool(a, 23); err != nil {
		return o, err
	}
	if o.Hidden, err = wire.Bool(a, 24); err != nil {
		return o, err
	}
	if o.PlacedID, err = wire.OptionalInt(a, 25); err != nil {
		return o, err
	}
	if o.Routing, err = wire.Str(a, 28); err != nil {
		return o, err
	}
	if v := wire.At(a, 31); v != nil {
		o.Meta = v
	}
	return o, nil
}

// TradeExecution is one of your own trades ("te"/"tu" stream
// records and /auth/r/trades rows):
// [ID, PAIR, MTS_CREATE, ORDER_ID, EXEC_AMOUNT, EXEC_PRICE,
//
//	ORDER_TYPE, ORDER_PRICE, MAKER, FEE, FEE_CURRENCY].
type TradeExecution struct {
	ID          int64         `json:"id"`
	Pair        string        `json:"pair"`
	MTSCreate   types.Millis  `json:"mts_create"`
	OrderID     int64         `json:"order_id"`
	ExecAmount  types.Decimal `json:"exec_amount"`
	ExecPrice   types.Decimal `json:"exec_price"`
	OrderType   string        `json:"order_type"`
	OrderPrice  types.Decimal `json:"order_price"`
	Maker       int64         `json:"maker"`
	Fee         types.Decimal `json:"fee"`
	FeeCurrency string        `json:"fee_currency"`
}

// TradeExecutionFromWire decodes a trade-execution positional
// array. Exported for the WebSocket account stream.
func TradeExecutionFromWire(a []any) (TradeExecution, error) {
	t := TradeExecution{}
	if err := wire.AssertMinLength(a, 6, "trade execution"); err != nil {
		return t, err
	}
	var err error
	if t.ID, err = wire.RequiredID(a, 0); err != nil {
		return t, err
	}
	if t.Pair, err = wire.Str(a, 1); err != nil {
		return t, err
	}
	if mts, _, merr := wire.Mts(a, 2); merr != nil {
		return t, merr
	} else {
		t.MTSCreate = mts
	}
	if t.OrderID, err = wire.RequiredID(a, 3); err != nil {
		return t, err
	}
	if t.ExecAmount, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return t, err
	}
	if t.ExecPrice, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return t, err
	}
	if t.OrderType, err = wire.Str(a, 6); err != nil {
		return t, err
	}
	if t.OrderPrice, err = wire.DecOr(a, 7, types.Decimal{}); err != nil {
		return t, err
	}
	t.Maker = wire.IntOr(a, 8, 0)
	if t.Fee, err = wire.DecOr(a, 9, types.Decimal{}); err != nil {
		return t, err
	}
	if t.FeeCurrency, err = wire.Str(a, 10); err != nil {
		return t, err
	}
	return t, nil
}

// Notification is the auth write/ack envelope:
// [MTS, TYPE, MSG_ID, _, DATA, _, CODE, STATUS, TEXT].
type Notification struct {
	MTS       types.Millis `json:"mts"`
	Type      string       `json:"type"`
	MessageID int64        `json:"message_id"`
	// Data holds the inner payload (decoded entity array or
	// list of entity arrays), kept untyped at this level.
	Data   any    `json:"data,omitempty"`
	Code   int64  `json:"code"`
	Status string `json:"status"`
	Text   string `json:"text"`
}

// Success reports whether the notification status is SUCCESS.
func (n Notification) Success() bool { return n.Status == "SUCCESS" }

// NotificationFromWire decodes the notification envelope.
func NotificationFromWire(a []any) (Notification, error) {
	n := Notification{}
	if err := wire.AssertMinLength(a, 8, "notification"); err != nil {
		return n, err
	}
	var err error
	if mts, _, merr := wire.Mts(a, 0); merr != nil {
		return n, merr
	} else {
		n.MTS = mts
	}
	if n.Type, err = wire.Str(a, 1); err != nil {
		return n, err
	}
	n.MessageID = wire.IntOr(a, 2, 0)
	n.Data = wire.At(a, 4)
	n.Code = wire.IntOr(a, 5, 0)
	if n.Status, err = wire.Str(a, 6); err != nil {
		return n, err
	}
	if n.Text, err = wire.Str(a, 7); err != nil {
		return n, err
	}
	return n, nil
}

// WriteResult is the decoded result of an order write op: the
// notification envelope plus every order array it carried.
type WriteResult struct {
	Notification Notification `json:"notification"`
	Orders       []Order      `json:"orders"`
}

// Success reports whether the write acknowledged SUCCESS.
func (r WriteResult) Success() bool { return r.Notification.Success() }

// FirstOrder returns the first order in the payload, if any.
func (r WriteResult) FirstOrder() (Order, bool) {
	if len(r.Orders) == 0 {
		return Order{}, false
	}
	return r.Orders[0], true
}

// writeResultFromNotification decodes the notification plus the
// order row(s) inside its DATA field. DATA is either one order
// array or a list of order arrays.
func writeResultFromNotification(raw []any) (WriteResult, error) {
	n, err := NotificationFromWire(raw)
	if err != nil {
		return WriteResult{}, err
	}
	res := WriteResult{Notification: n}
	data, ok := n.Data.([]any)
	if !ok || len(data) == 0 {
		return res, nil
	}
	// Single order array vs list of order arrays: a single order
	// starts with a numeric ID, a list starts with an array.
	if _, firstIsArr := data[0].([]any); firstIsArr {
		for _, item := range data {
			arr, ok := item.([]any)
			if !ok {
				continue
			}
			o, derr := FromWire(arr)
			if derr != nil {
				return res, derr
			}
			res.Orders = append(res.Orders, o)
		}
		return res, nil
	}
	o, derr := FromWire(data)
	if derr != nil {
		return res, derr
	}
	res.Orders = append(res.Orders, o)
	return res, nil
}
