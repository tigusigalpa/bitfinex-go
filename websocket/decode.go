package websocket

import (
	"github.com/tigusigalpa/bitfinex-go/funding"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/market"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/types"
	"github.com/tigusigalpa/bitfinex-go/wallets"
	"strings"
)

func (c *Client) dispatchTagged(id int64, tag string, payload any) {
	if id == 0 {
		c.dispatchAuthStream(tag, payload)
		return
	}
	c.mu.Lock()
	sub := c.subs[id]
	c.mu.Unlock()
	if sub == nil {
		return
	}
	ev := Event{Kind: KindUpdate, ChanID: id, Channel: sub.req.Channel, Symbol: sub.req.Symbol, Raw: payload, Action: tag}
	row, err := wire.AsArr(payload, "trade update")
	if err == nil {
		tr, e := market.TradeFromWire(row, types.Symbol(sub.req.Symbol))
		err = e
		if err == nil {
			ev.Trades = []market.Trade{tr}
		}
	}
	if err != nil {
		c.deliverError(id, err)
		return
	}
	c.deliverTo(sub, ev)
}

func (c *Client) dispatchUntagged(id int64, payload any) {
	c.mu.Lock()
	sub := c.subs[id]
	c.mu.Unlock()
	if sub == nil {
		return
	}
	arr, err := wire.AsArr(payload, "channel data")
	if err != nil {
		c.deliverError(id, err)
		return
	}
	ev := Event{Kind: KindUpdate, ChanID: id, Channel: sub.req.Channel, Symbol: sub.req.Symbol, Raw: payload}
	switch sub.req.Channel {
	case "ticker":
		tk, e := market.TickerFromWire(arr, types.Symbol(sub.req.Symbol))
		err = e
		ev.Ticker = &tk
	case "trades":
		ev.Kind = KindSnapshot
		ev.Trades, err = decodeEntityList(payload, func(a []any) (market.Trade, error) { return market.TradeFromWire(a, types.Symbol(sub.req.Symbol)) })
	case "book":
		c.handleBookData(sub, id, arr, payload)
		return
	case "candles":
		ev.Candles, err = decodeEntityList(payload, market.CandleFromWire)
		if len(arr) == 0 {
			ev.Kind = KindSnapshot
		} else if _, nested := arr[0].([]any); nested {
			ev.Kind = KindSnapshot
		}
	case "status":
		ev.Status = payload // retained legacy escape hatch
		if strings.HasPrefix(sub.req.Key, "deriv:") {
			v, e := market.DerivativeStatusHistoryFromWire(arr, types.Symbol(strings.TrimPrefix(sub.req.Key, "deriv:")))
			err = e
			ev.DerivativeStatus = &v
		} else if sub.req.Key == "liq:global" {
			ev.Liquidations, err = decodeEntityList(payload, market.LiquidationFromWire)
		} else {
			err = &types.ProtocolError{Message: "unsupported status key"}
		}
	default:
		ev.Status = payload
	}
	if err != nil {
		c.deliverError(id, err)
		return
	}
	c.deliverTo(sub, ev)
}

func (c *Client) handleBookData(sub *Subscription, id int64, arr []any, payload any) {
	c.mu.Lock()
	book := c.books[id]
	c.mu.Unlock()
	isRaw := sub.req.Prec == "R0"
	isFunding := types.Symbol(sub.req.Symbol).IsFunding()
	_, nested := wire.At(arr, 0).([]any)
	snapshot := len(arr) == 0 || nested
	rows := [][]any{arr}
	if snapshot {
		rows = nil
		for _, v := range arr {
			row, ok := v.([]any)
			if !ok {
				c.deliverError(id, &types.ProtocolError{Message: "invalid book snapshot row"})
				c.resyncChannel(id)
				return
			}
			rows = append(rows, row)
		}
	}
	levels := make([]market.BookLevel, 0, len(rows))
	for _, row := range rows {
		lvl, err := decodeBookLevel(row, isRaw, isFunding)
		if err != nil {
			c.deliverError(id, err)
			c.resyncChannel(id)
			return
		}
		levels = append(levels, lvl)
	}
	if book != nil {
		if snapshot {
			book.clear()
		}
		for _, row := range rows {
			applyLevel(book, row, isRaw, isFunding)
		}
	}
	ev := Event{Kind: KindUpdate, ChanID: id, Channel: "book", Symbol: sub.req.Symbol, Raw: payload}
	if snapshot {
		ev.Kind = KindSnapshot
		ev.BookSnapshot = levels
	} else {
		ev.BookLevel = &levels[0]
	}
	c.deliverTo(sub, ev)
}

func decodeBookLevel(row []any, raw, fund bool) (market.BookLevel, error) {
	var l market.BookLevel
	switch {
	case raw && fund:
		e, err := market.RawFundingBookFromWire(row)
		l.RawFunding = &e
		return l, err
	case raw:
		e, err := market.RawBookEntryFromWire(row)
		l.Raw = &e
		return l, err
	case fund:
		e, err := market.FundingBookEntryFromWire(row)
		l.Funding = &e
		return l, err
	default:
		e, err := market.BookEntryFromWire(row)
		l.Entry = &e
		return l, err
	}
}

func (c *Client) dispatchAuthStream(tag string, payload any) {
	a := &AuthEvent{Stream: tag, Raw: payload}
	kind := KindUpdate
	var err error
	switch tag {
	case "os", "on", "ou", "oc":
		a.Orders, err = decodeEntityList(payload, orders.FromWire)
	case "ps", "pn", "pu", "pc":
		a.Positions, err = decodeEntityList(payload, positions.FromWire)
	case "te", "tu":
		a.Trades, err = decodeEntityList(payload, orders.TradeExecutionFromWire)
	case "fos", "fon", "fou", "foc":
		a.Offers, err = decodeEntityList(payload, funding.OfferFromWire)
	case "fcs", "fcn", "fcu", "fcc":
		a.Credits, err = decodeEntityList(payload, funding.CreditFromWire)
	case "fls", "fln", "flu", "flc":
		a.Loans, err = decodeEntityList(payload, funding.LoanFromWire)
	case "ws", "wu":
		a.Wallets, err = decodeEntityList(payload, wallets.FromWire)
	case "fte", "ftu":
		a.FundingTrades, err = decodeEntityList(payload, funding.TradeFromWire)
	case "n":
		row, e := wire.AsArr(payload, "notification")
		err = e
		if err == nil {
			n, e := orders.NotificationFromWire(row)
			err = e
			a.Notification = &n
		}
	case "bu":
		row, e := wire.AsArr(payload, "balance info")
		err = e
		if err == nil {
			var b types.BalanceInfo
			b.Assets, err = wire.RequiredDecimal(row, 0)
			if err == nil {
				b.NetAssets, err = wire.RequiredDecimal(row, 1)
			}
			a.Balance = &b
		}
		a.Info = payload
	case "miu":
		m, e := wire.MarginInfo(payload)
		err = e
		a.Margin = &m
		a.Info = payload
	case "fiu":
		v, e := wire.FundingInfo(payload)
		err = e
		a.Funding = &v
		a.Info = payload
	default:
		a.Info = payload
	}
	switch tag {
	case "os", "ps", "fos", "fcs", "fls", "ws":
		kind = KindSnapshot
	}
	ev := Event{Kind: kind, ChanID: 0, Channel: "auth", Auth: a}
	if err != nil {
		ev = Event{Kind: KindError, ChanID: 0, Err: err}
	}
	c.mu.Lock()
	sub := c.subs[0]
	c.mu.Unlock()
	if sub != nil {
		c.deliverTo(sub, ev)
	} else {
		c.emitLifecycle(ev)
	}
}
