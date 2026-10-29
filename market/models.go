// Package market implements the public REST v2 endpoints
// (api-pub.bitfinex.com): platform status, tickers, trades,
// books, stats, candles, configs, derivatives status,
// liquidations, rankings, funding stats, VASPs and calc
// endpoints. Every method is idempotent and safe to retry.
package market

import (
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Ticker is a trading or funding ticker snapshot.
type Ticker struct {
	Symbol             types.Symbol  `json:"symbol"`
	FRR                types.Decimal `json:"frr,omitempty"` // funding only
	Bid                types.Decimal `json:"bid"`
	BidPeriod          int64         `json:"bid_period,omitempty"` // funding only
	BidSize            types.Decimal `json:"bid_size"`
	Ask                types.Decimal `json:"ask"`
	AskPeriod          int64         `json:"ask_period,omitempty"` // funding only
	AskSize            types.Decimal `json:"ask_size"`
	DailyChange        types.Decimal `json:"daily_change"`
	DailyChangeRel     types.Decimal `json:"daily_change_relative"`
	LastPrice          types.Decimal `json:"last_price"`
	Volume             types.Decimal `json:"volume"`
	High               types.Decimal `json:"high"`
	Low                types.Decimal `json:"low"`
	FRRAmountAvailable types.Decimal `json:"frr_amount_available,omitempty"` // funding only
	MTS                types.Millis  `json:"mts,omitempty"`
}

// Trade is a public trade (or funding public trade) execution.
type Trade struct {
	ID     int64         `json:"id"`
	Symbol types.Symbol  `json:"symbol,omitempty"`
	MTS    types.Millis  `json:"mts"`
	Amount types.Decimal `json:"amount"`
	Price  types.Decimal `json:"price,omitempty"`  // trading
	Rate   types.Decimal `json:"rate,omitempty"`   // funding
	Period int64         `json:"period,omitempty"` // funding
}

// Candle is an OHLCV candle.
type Candle struct {
	MTS    types.Millis  `json:"mts"`
	Open   types.Decimal `json:"open"`
	Close  types.Decimal `json:"close"`
	High   types.Decimal `json:"high"`
	Low    types.Decimal `json:"low"`
	Volume types.Decimal `json:"volume"`
}

// BookEntry is one level of an aggregated (P0–P4) trading book.
type BookEntry struct {
	Price  types.Decimal `json:"price"`
	Count  int64         `json:"count"`
	Amount types.Decimal `json:"amount"` // >0 bid, <0 ask
}

// RawBookEntry is one order in a raw (R0) book.
type RawBookEntry struct {
	OrderID int64         `json:"order_id"`
	Price   types.Decimal `json:"price"`
	Amount  types.Decimal `json:"amount"` // >0 bid, <0 ask
}

// RawFundingBookEntry is an R0 funding offer: ID, period, rate, amount.
type RawFundingBookEntry struct {
	OfferID int64         `json:"offer_id"`
	Period  int64         `json:"period"`
	Rate    types.Decimal `json:"rate"`
	Amount  types.Decimal `json:"amount"`
}

// RawFundingBookFromWire decodes a raw funding book row.
func RawFundingBookFromWire(a []any) (RawFundingBookEntry, error) {
	var e RawFundingBookEntry
	if err := wire.AssertMinLength(a, 4, "raw funding book"); err != nil {
		return e, err
	}
	var err error
	if e.OfferID, err = wire.MustInt(a, 0, "raw funding book"); err != nil {
		return e, err
	}
	if e.Period, err = wire.MustInt(a, 1, "raw funding book"); err != nil {
		return e, err
	}
	if e.Rate, err = wire.RequiredDecimal(a, 2); err != nil {
		return e, err
	}
	e.Amount, err = wire.RequiredDecimal(a, 3)
	return e, err
}

// FundingBookEntry is one level of an aggregated funding book.
type FundingBookEntry struct {
	Rate   types.Decimal `json:"rate"`
	Period int64         `json:"period"`
	Count  int64         `json:"count"`
	Amount types.Decimal `json:"amount"` // >0 bid, <0 ask
}

// StatEntry is a [MTS, VALUE] stats pair.
type StatEntry struct {
	MTS   types.Millis  `json:"mts"`
	Value types.Decimal `json:"value"`
}

// DerivativeStatus is a derivative pair status snapshot.
type DerivativeStatus struct {
	Key                  types.Symbol  `json:"key"`
	MTS                  types.Millis  `json:"mts"`
	DerivPrice           types.Decimal `json:"deriv_price"`
	SpotPrice            types.Decimal `json:"spot_price"`
	InsuranceFundBalance types.Decimal `json:"insurance_fund_balance"`
	NextFundingEventMTS  types.Millis  `json:"next_funding_event_mts"`
	NextFundingAccrued   types.Decimal `json:"next_funding_accrued"`
	NextFundingStep      types.Decimal `json:"next_funding_step"`
	CurrentFunding       types.Decimal `json:"current_funding"`
	MarkPrice            types.Decimal `json:"mark_price"`
	OpenInterest         types.Decimal `json:"open_interest"`
	ClampMin             types.Decimal `json:"clamp_min"`
	ClampMax             types.Decimal `json:"clamp_max"`
}

// Liquidation is a liquidation event entry.
type Liquidation struct {
	PositionID    int64         `json:"position_id"`
	MTS           types.Millis  `json:"mts"`
	Symbol        types.Symbol  `json:"symbol"`
	Amount        types.Decimal `json:"amount"`
	BasePrice     types.Decimal `json:"base_price"`
	IsMatch       int64         `json:"is_match"`
	IsMarketSold  int64         `json:"is_market_sold"`
	PriceAcquired types.Decimal `json:"price_acquired"`
}

// FundingStatsEntry is a funding statistics snapshot.
type FundingStatsEntry struct {
	MTS             types.Millis  `json:"mts"`
	FRR             types.Decimal `json:"frr"`
	AverageRate     types.Decimal `json:"average_rate"`
	AveragePeriod   types.Decimal `json:"average_period"`
	FundingUsed     types.Decimal `json:"funding_used"`
	FundingProvided types.Decimal `json:"funding_provided"`
}

// RankingRow is a best-effort leaderboard row; Extra preserves
// the undocumented tail of the positional array.
type RankingRow struct {
	MTS      types.Millis `json:"mts"`
	Username string       `json:"username"`
	Extra    []any        `json:"extra,omitempty"`
}

// MarketAvgPrice is the hypothetical average execution price.
type MarketAvgPrice struct {
	PriceAvg types.Decimal `json:"price_avg"`
	Amount   types.Decimal `json:"amount"`
}

// ---------- positional decoders ----------

func tickerFromWire(a []any, sym types.Symbol) (Ticker, error) {
	t := Ticker{Symbol: sym}
	var err error
	if sym.IsFunding() {
		if err = wire.AssertMinLength(a, 13, "funding ticker"); err != nil {
			return t, err
		}
		if t.FRR, err = wire.DecOr(a, 0, types.Decimal{}); err != nil {
			return t, err
		}
		if t.Bid, err = wire.DecOr(a, 1, types.Decimal{}); err != nil {
			return t, err
		}
		t.BidPeriod = wire.IntOr(a, 2, 0)
		if t.BidSize, err = wire.DecOr(a, 3, types.Decimal{}); err != nil {
			return t, err
		}
		if t.Ask, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
			return t, err
		}
		t.AskPeriod = wire.IntOr(a, 5, 0)
		if t.AskSize, err = wire.DecOr(a, 6, types.Decimal{}); err != nil {
			return t, err
		}
		if t.DailyChange, err = wire.DecOr(a, 7, types.Decimal{}); err != nil {
			return t, err
		}
		if t.DailyChangeRel, err = wire.DecOr(a, 8, types.Decimal{}); err != nil {
			return t, err
		}
		if t.LastPrice, err = wire.DecOr(a, 9, types.Decimal{}); err != nil {
			return t, err
		}
		if t.Volume, err = wire.DecOr(a, 10, types.Decimal{}); err != nil {
			return t, err
		}
		if t.High, err = wire.DecOr(a, 11, types.Decimal{}); err != nil {
			return t, err
		}
		if t.Low, err = wire.DecOr(a, 12, types.Decimal{}); err != nil {
			return t, err
		}
		if t.FRRAmountAvailable, err = wire.DecOr(a, 15, types.Decimal{}); err != nil {
			return t, err
		}
		if mts, ok, merr := wire.Mts(a, 16); merr != nil {
			return t, merr
		} else if ok {
			t.MTS = mts
		}
		return t, nil
	}

	if err = wire.AssertMinLength(a, 10, "trading ticker"); err != nil {
		return t, err
	}
	if t.Bid, err = wire.DecOr(a, 0, types.Decimal{}); err != nil {
		return t, err
	}
	if t.BidSize, err = wire.DecOr(a, 1, types.Decimal{}); err != nil {
		return t, err
	}
	if t.Ask, err = wire.DecOr(a, 2, types.Decimal{}); err != nil {
		return t, err
	}
	if t.AskSize, err = wire.DecOr(a, 3, types.Decimal{}); err != nil {
		return t, err
	}
	if t.DailyChange, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return t, err
	}
	if t.DailyChangeRel, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return t, err
	}
	if t.LastPrice, err = wire.DecOr(a, 6, types.Decimal{}); err != nil {
		return t, err
	}
	if t.Volume, err = wire.DecOr(a, 7, types.Decimal{}); err != nil {
		return t, err
	}
	if t.High, err = wire.DecOr(a, 8, types.Decimal{}); err != nil {
		return t, err
	}
	if t.Low, err = wire.DecOr(a, 9, types.Decimal{}); err != nil {
		return t, err
	}
	if mts, ok, merr := wire.Mts(a, 12); merr != nil {
		return t, merr
	} else if ok {
		t.MTS = mts
	}
	return t, nil
}

func tradeFromWire(a []any, sym types.Symbol) (Trade, error) {
	t := Trade{Symbol: sym}
	var err error
	if sym.IsFunding() {
		if err = wire.AssertMinLength(a, 5, "funding trade"); err != nil {
			return t, err
		}
		t.ID = wire.IntOr(a, 0, 0)
		if mts, _, merr := wire.Mts(a, 1); merr != nil {
			return t, merr
		} else {
			t.MTS = mts
		}
		if t.Amount, err = wire.RequiredDecimal(a, 2); err != nil {
			return t, err
		}
		if t.Rate, err = wire.RequiredDecimal(a, 3); err != nil {
			return t, err
		}
		t.Period = wire.IntOr(a, 4, 0)
		return t, nil
	}
	if err = wire.AssertMinLength(a, 4, "trade"); err != nil {
		return t, err
	}
	t.ID = wire.IntOr(a, 0, 0)
	if mts, _, merr := wire.Mts(a, 1); merr != nil {
		return t, merr
	} else {
		t.MTS = mts
	}
	if t.Amount, err = wire.RequiredDecimal(a, 2); err != nil {
		return t, err
	}
	if t.Price, err = wire.RequiredDecimal(a, 3); err != nil {
		return t, err
	}
	return t, nil
}

func candleFromWire(a []any) (Candle, error) {
	c := Candle{}
	if err := wire.AssertMinLength(a, 6, "candle"); err != nil {
		return c, err
	}
	var err error
	if mts, _, merr := wire.Mts(a, 0); merr != nil {
		return c, merr
	} else {
		c.MTS = mts
	}
	if c.Open, err = wire.RequiredDecimal(a, 1); err != nil {
		return c, err
	}
	if c.Close, err = wire.RequiredDecimal(a, 2); err != nil {
		return c, err
	}
	if c.High, err = wire.RequiredDecimal(a, 3); err != nil {
		return c, err
	}
	if c.Low, err = wire.RequiredDecimal(a, 4); err != nil {
		return c, err
	}
	if c.Volume, err = wire.RequiredDecimal(a, 5); err != nil {
		return c, err
	}
	return c, nil
}

// BookLevel is a sum-type for REST book results: exactly one of
// Entry/Raw/Funding is non-nil.
type BookLevel struct {
	Entry      *BookEntry           `json:"entry,omitempty"`
	Raw        *RawBookEntry        `json:"raw,omitempty"`
	Funding    *FundingBookEntry    `json:"funding,omitempty"`
	RawFunding *RawFundingBookEntry `json:"raw_funding,omitempty"`
}

func bookEntryFromWire(a []any) (BookEntry, error) {
	e := BookEntry{}
	if err := wire.AssertMinLength(a, 3, "book entry"); err != nil {
		return e, err
	}
	var err error
	if e.Price, err = wire.RequiredDecimal(a, 0); err != nil {
		return e, err
	}
	if e.Count, err = wire.MustInt(a, 1, "book entry"); err != nil {
		return e, err
	}
	if e.Amount, err = wire.RequiredDecimal(a, 2); err != nil {
		return e, err
	}
	return e, nil
}

func rawBookEntryFromWire(a []any) (RawBookEntry, error) {
	e := RawBookEntry{}
	if err := wire.AssertMinLength(a, 3, "raw book entry"); err != nil {
		return e, err
	}
	var err error
	if e.OrderID, err = wire.MustInt(a, 0, "raw book entry"); err != nil {
		return e, err
	}
	if e.Price, err = wire.RequiredDecimal(a, 1); err != nil {
		return e, err
	}
	if e.Amount, err = wire.RequiredDecimal(a, 2); err != nil {
		return e, err
	}
	return e, nil
}

func fundingBookEntryFromWire(a []any) (FundingBookEntry, error) {
	e := FundingBookEntry{}
	if err := wire.AssertMinLength(a, 4, "funding book entry"); err != nil {
		return e, err
	}
	var err error
	if e.Rate, err = wire.RequiredDecimal(a, 0); err != nil {
		return e, err
	}
	e.Period = wire.IntOr(a, 1, 0)
	e.Count = wire.IntOr(a, 2, 0)
	if e.Amount, err = wire.RequiredDecimal(a, 3); err != nil {
		return e, err
	}
	return e, nil
}

func statEntryFromWire(a []any) (StatEntry, error) {
	e := StatEntry{}
	if err := wire.AssertMinLength(a, 2, "stat entry"); err != nil {
		return e, err
	}
	var err error
	if mts, _, merr := wire.Mts(a, 0); merr != nil {
		return e, merr
	} else {
		e.MTS = mts
	}
	if e.Value, err = wire.DecOr(a, 1, types.Decimal{}); err != nil {
		return e, err
	}
	return e, nil
}

func derivStatusFromWire(a []any) (DerivativeStatus, error) {
	d := DerivativeStatus{}
	if err := wire.AssertMinLength(a, 4, "derivative status"); err != nil {
		return d, err
	}
	var err error
	key, _ := wire.Str(a, 0)
	d.Key = types.Symbol(key)
	if mts, _, merr := wire.Mts(a, 1); merr != nil {
		return d, merr
	} else {
		d.MTS = mts
	}
	if d.DerivPrice, err = wire.DecOr(a, 3, types.Decimal{}); err != nil {
		return d, err
	}
	if d.SpotPrice, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return d, err
	}
	if d.InsuranceFundBalance, err = wire.DecOr(a, 6, types.Decimal{}); err != nil {
		return d, err
	}
	if mts, _, merr := wire.Mts(a, 9); merr != nil {
		return d, merr
	} else {
		d.NextFundingEventMTS = mts
	}
	if d.NextFundingAccrued, err = wire.DecOr(a, 10, types.Decimal{}); err != nil {
		return d, err
	}
	if d.NextFundingStep, err = wire.DecOr(a, 11, types.Decimal{}); err != nil {
		return d, err
	}
	if d.CurrentFunding, err = wire.DecOr(a, 13, types.Decimal{}); err != nil {
		return d, err
	}
	if d.MarkPrice, err = wire.DecOr(a, 16, types.Decimal{}); err != nil {
		return d, err
	}
	if d.OpenInterest, err = wire.DecOr(a, 19, types.Decimal{}); err != nil {
		return d, err
	}
	if d.ClampMin, err = wire.DecOr(a, 22, types.Decimal{}); err != nil {
		return d, err
	}
	if d.ClampMax, err = wire.DecOr(a, 23, types.Decimal{}); err != nil {
		return d, err
	}
	return d, nil
}

func liquidationFromWire(a []any) (Liquidation, error) {
	l := Liquidation{}
	if err := wire.AssertMinLength(a, 8, "liquidation"); err != nil {
		return l, err
	}
	var err error
	l.PositionID = wire.IntOr(a, 0, 0)
	if mts, _, merr := wire.Mts(a, 1); merr != nil {
		return l, merr
	} else {
		l.MTS = mts
	}
	sym, _ := wire.Str(a, 3)
	l.Symbol = types.Symbol(sym)
	if l.Amount, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return l, err
	}
	if l.BasePrice, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return l, err
	}
	l.IsMatch = wire.IntOr(a, 6, 0)
	l.IsMarketSold = wire.IntOr(a, 7, 0)
	if l.PriceAcquired, err = wire.DecOr(a, 9, types.Decimal{}); err != nil {
		return l, err
	}
	return l, nil
}

func fundingStatsFromWire(a []any) (FundingStatsEntry, error) {
	e := FundingStatsEntry{}
	if err := wire.AssertMinLength(a, 4, "funding stats entry"); err != nil {
		return e, err
	}
	var err error
	if mts, _, merr := wire.Mts(a, 0); merr != nil {
		return e, merr
	} else {
		e.MTS = mts
	}
	if e.FRR, err = wire.DecOr(a, 3, types.Decimal{}); err != nil {
		return e, err
	}
	if e.AverageRate, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return e, err
	}
	if e.AveragePeriod, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return e, err
	}
	if e.FundingUsed, err = wire.DecOr(a, 6, types.Decimal{}); err != nil {
		return e, err
	}
	if e.FundingProvided, err = wire.DecOr(a, 7, types.Decimal{}); err != nil {
		return e, err
	}
	return e, nil
}

// ---------- exported decoders (shared with the websocket package) ----------

// TickerFromWire decodes a ticker positional array (REST and WS).
func TickerFromWire(a []any, sym types.Symbol) (Ticker, error) {
	return tickerFromWire(a, sym)
}

// TradeFromWire decodes a trade positional array (REST and WS).
func TradeFromWire(a []any, sym types.Symbol) (Trade, error) {
	return tradeFromWire(a, sym)
}

// CandleFromWire decodes a candle positional array (REST and WS).
func CandleFromWire(a []any) (Candle, error) { return candleFromWire(a) }

// BookEntryFromWire decodes an aggregated book level (REST/WS).
func BookEntryFromWire(a []any) (BookEntry, error) { return bookEntryFromWire(a) }

// RawBookEntryFromWire decodes a raw R0 book order (REST/WS).
func RawBookEntryFromWire(a []any) (RawBookEntry, error) { return rawBookEntryFromWire(a) }

// FundingBookEntryFromWire decodes a funding book level.
func FundingBookEntryFromWire(a []any) (FundingBookEntry, error) {
	return fundingBookEntryFromWire(a)
}
