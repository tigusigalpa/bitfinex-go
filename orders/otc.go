package orders

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// OTCOrder is the separate documented OTC order schema (not a regular Order row).
type OTCOrder struct {
	ID                   int64         `json:"id"`
	Symbol               types.Symbol  `json:"symbol"`
	MTSCreate            types.Millis  `json:"mts_create"`
	MTSUpdate            types.Millis  `json:"mts_update"`
	Initiator            bool          `json:"initiator"`
	InitiatorNickname    string        `json:"initiator_nickname"`
	CounterpartyNickname string        `json:"counterparty_nickname"`
	Amount               types.Decimal `json:"amount"`
	Price                types.Decimal `json:"price"`
	Status               string        `json:"status"`
}

// OTCHistoryTyped retrieves one OTC history page using its native positional schema.
func (s *Service) OTCHistoryTyped(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]OTCOrder], error) {
	if e := q.ValidateFor(500, false); e != nil {
		return nil, e
	}
	path := "v2/auth/r/orders/otc"
	if symbol != "" {
		sym, e := types.ParseSymbol(symbol)
		if e != nil {
			return nil, e
		}
		path += "/" + sym.String()
	}
	path += "/hist"
	r, e := s.t.PostAuth(ctx, path, historyBody(q), readBucket())
	if e != nil {
		return nil, e
	}
	rows, e := wire.Rows(r.Data, "OTC history")
	if e != nil {
		return nil, e
	}
	out := make([]OTCOrder, 0, len(rows))
	for _, a := range rows {
		if e = wire.AssertMinLength(a, 13, "OTC order"); e != nil {
			return nil, e
		}
		var o OTCOrder
		o.ID, e = wire.MustInt(a, 0, "OTC id")
		if e != nil {
			return nil, e
		}
		sym, e := wire.RequiredString(a, 1)
		if e != nil {
			return nil, e
		}
		o.Symbol = types.Symbol(sym)
		o.MTSCreate, _, e = wire.Mts(a, 2)
		if e != nil {
			return nil, e
		}
		o.MTSUpdate, _, e = wire.Mts(a, 3)
		if e != nil {
			return nil, e
		}
		o.Initiator, e = wire.Bool(a, 5)
		if e != nil {
			return nil, e
		}
		o.InitiatorNickname, e = wire.Str(a, 6)
		if e != nil {
			return nil, e
		}
		o.CounterpartyNickname, e = wire.Str(a, 7)
		if e != nil {
			return nil, e
		}
		o.Amount, e = wire.RequiredDecimal(a, 9)
		if e != nil {
			return nil, e
		}
		o.Price, e = wire.RequiredDecimal(a, 10)
		if e != nil {
			return nil, e
		}
		o.Status, e = wire.RequiredString(a, 12)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}
