// Package funding implements the margin-funding endpoints:
// offers (submit/cancel/cancel-all/history), close, auto-renew,
// keep, loans, credits and funding trades/info.
//
// Write endpoints are never automatically retried.
package funding

import (
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Funding offer/credit/loan type strings.
const (
	TypeLend = "lend"
	TypeLoan = "loan"
	// TypeLimit uses a fixed daily rate.
	TypeLimit = "LIMIT"
	// TypeFRRDeltaFix uses a fixed offset from FRR.
	TypeFRRDeltaFix = "FRRDELTAFIX"
	// TypeFRRDeltaVar uses a dynamic non-negative offset from FRR.
	TypeFRRDeltaVar = "FRRDELTAVAR"
)

// FundingOffer is one funding offer row:
// [ID, SYMBOL, MTS_CREATE, MTS_UPDATE, AMOUNT, AMOUNT_ORIG, TYPE,
//
//	_, _, FLAGS, STATUS, _, _, _, RATE, PERIOD, NOTIFY, HIDDEN, _,
//	RENEW].
type FundingOffer struct {
	ID         int64         `json:"id"`
	Symbol     types.Symbol  `json:"symbol"`
	MTSCreate  types.Millis  `json:"mts_create"`
	MTSUpdate  types.Millis  `json:"mts_update"`
	Amount     types.Decimal `json:"amount"`
	AmountOrig types.Decimal `json:"amount_orig"`
	Type       string        `json:"type"`
	Flags      int64         `json:"flags"`
	Status     string        `json:"status"`
	Rate       types.Decimal `json:"rate"`
	Period     int64         `json:"period"`
	Notify     bool          `json:"notify"`
	Hidden     bool          `json:"hidden"`
	Renew      int64         `json:"renew"`
}

// OfferFromWire decodes a funding offer positional array.
// Exported for the WebSocket account stream (fos/fon/fou/foc).
func OfferFromWire(a []any) (FundingOffer, error) {
	o := FundingOffer{}
	if err := wire.AssertMinLength(a, 16, "funding offer"); err != nil {
		return o, err
	}
	var err error
	if o.ID, err = wire.RequiredID(a, 0); err != nil {
		return o, err
	}
	if sym, serr := wire.Str(a, 1); serr != nil {
		return o, serr
	} else {
		o.Symbol = types.Symbol(sym)
	}
	if mts, _, merr := wire.Mts(a, 2); merr != nil {
		return o, merr
	} else {
		o.MTSCreate = mts
	}
	if mts, _, merr := wire.Mts(a, 3); merr != nil {
		return o, merr
	} else {
		o.MTSUpdate = mts
	}
	if o.Amount, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return o, err
	}
	if o.AmountOrig, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return o, err
	}
	if o.Type, err = wire.Str(a, 6); err != nil {
		return o, err
	}
	o.Flags = wire.IntOr(a, 9, 0)
	if o.Status, err = wire.Str(a, 10); err != nil {
		return o, err
	}
	if o.Rate, err = wire.DecOr(a, 14, types.Decimal{}); err != nil {
		return o, err
	}
	o.Period = wire.IntOr(a, 15, 0)
	if o.Notify, err = wire.Bool(a, 16); err != nil {
		return o, err
	}
	if o.Hidden, err = wire.Bool(a, 17); err != nil {
		return o, err
	}
	o.Renew = wire.IntOr(a, 19, 0)
	return o, nil
}

// FundingCredit is one credit row (funding provided and used):
// [ID, SYMBOL, SIDE, MTS_CREATE, MTS_UPDATE, AMOUNT, FLAGS,
//
//	STATUS, _, _, _, RATE, PERIOD, MTS_OPENING, MTS_LAST_PAYOUT, _,
//	_, _, RENEW, RATE_REAL, ...].
type FundingCredit struct {
	ID            int64         `json:"id"`
	Symbol        types.Symbol  `json:"symbol"`
	Side          int64         `json:"side"`
	MTSCreate     types.Millis  `json:"mts_create"`
	MTSUpdate     types.Millis  `json:"mts_update"`
	Amount        types.Decimal `json:"amount"`
	Flags         int64         `json:"flags"`
	Status        string        `json:"status"`
	Rate          types.Decimal `json:"rate"`
	Period        int64         `json:"period"`
	MTSOpening    types.Millis  `json:"mts_opening"`
	MTSLastPayout types.Millis  `json:"mts_last_payout"`
	Renew         int64         `json:"renew"`
	RateReal      types.Decimal `json:"rate_real"`
}

// CreditFromWire decodes a funding credit positional array.
// Exported for the WebSocket account stream (fcs/fcn/fcu/fcc).
func CreditFromWire(a []any) (FundingCredit, error) {
	c := FundingCredit{}
	if err := wire.AssertMinLength(a, 13, "funding credit"); err != nil {
		return c, err
	}
	var err error
	if c.ID, err = wire.RequiredID(a, 0); err != nil {
		return c, err
	}
	if sym, serr := wire.Str(a, 1); serr != nil {
		return c, serr
	} else {
		c.Symbol = types.Symbol(sym)
	}
	c.Side = wire.IntOr(a, 2, 0)
	if mts, _, merr := wire.Mts(a, 3); merr != nil {
		return c, merr
	} else {
		c.MTSCreate = mts
	}
	if mts, _, merr := wire.Mts(a, 4); merr != nil {
		return c, merr
	} else {
		c.MTSUpdate = mts
	}
	if c.Amount, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return c, err
	}
	c.Flags = wire.IntOr(a, 6, 0)
	if c.Status, err = wire.Str(a, 7); err != nil {
		return c, err
	}
	if c.Rate, err = wire.DecOr(a, 11, types.Decimal{}); err != nil {
		return c, err
	}
	c.Period = wire.IntOr(a, 12, 0)
	if mts, _, merr := wire.Mts(a, 13); merr != nil {
		return c, merr
	} else {
		c.MTSOpening = mts
	}
	if mts, _, merr := wire.Mts(a, 14); merr != nil {
		return c, merr
	} else {
		c.MTSLastPayout = mts
	}
	c.Renew = wire.IntOr(a, 18, 0)
	if c.RateReal, err = wire.DecOr(a, 19, types.Decimal{}); err != nil {
		return c, err
	}
	return c, nil
}

// FundingLoan is one loan row (funding taken): same layout as
// FundingCredit.
type FundingLoan = FundingCredit

// LoanFromWire decodes a funding loan positional array. Exported
// for the WebSocket account stream (fls/fln/flu/flc).
func LoanFromWire(a []any) (FundingLoan, error) { return CreditFromWire(a) }

// FundingTrade is one funding trade row:
// [ID, SYMBOL, MTS_CREATE, OFFER_ID, AMOUNT, RATE, PERIOD, MAKER].
type FundingTrade struct {
	ID        int64         `json:"id"`
	Symbol    types.Symbol  `json:"symbol"`
	MTSCreate types.Millis  `json:"mts_create"`
	OfferID   int64         `json:"offer_id"`
	Amount    types.Decimal `json:"amount"`
	Rate      types.Decimal `json:"rate"`
	Period    int64         `json:"period"`
	Maker     int64         `json:"maker"`
}

// TradeFromWire decodes a funding trade positional array.
// Exported for the WebSocket account stream (fte/ftu).
func TradeFromWire(a []any) (FundingTrade, error) {
	t := FundingTrade{}
	if err := wire.AssertMinLength(a, 6, "funding trade"); err != nil {
		return t, err
	}
	var err error
	if t.ID, err = wire.RequiredID(a, 0); err != nil {
		return t, err
	}
	if sym, serr := wire.Str(a, 1); serr != nil {
		return t, serr
	} else {
		t.Symbol = types.Symbol(sym)
	}
	if mts, _, merr := wire.Mts(a, 2); merr != nil {
		return t, merr
	} else {
		t.MTSCreate = mts
	}
	if t.OfferID, err = wire.RequiredID(a, 3); err != nil {
		return t, err
	}
	if t.Amount, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return t, err
	}
	if t.Rate, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return t, err
	}
	t.Period = wire.IntOr(a, 6, 0)
	t.Maker = wire.IntOr(a, 7, 0)
	return t, nil
}
