package market

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strconv"
)

// VASP is a virtual asset service provider's decentralised ID and name.
type VASP struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VASPsTyped retrieves concrete provider objects.
func (s *Service) VASPsTyped(ctx context.Context) (*types.Response[[]VASP], error) {
	r, e := s.t.GetPublic(ctx, "ext/vasps", nil, readBucket("public"))
	if e != nil {
		return nil, e
	}
	a, e := wire.AsArr(r.Data, "VASPs")
	if e != nil {
		return nil, e
	}
	out := make([]VASP, 0, len(a))
	for _, v := range a {
		if _, e = wire.Object(v, "id", "name"); e != nil {
			return nil, e
		}
		x, e := wire.Into[VASP](v)
		if e != nil {
			return nil, e
		}
		if x.ID == "" || x.Name == "" {
			return nil, &types.ProtocolError{Message: "empty VASP identity"}
		}
		out = append(out, x)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// AveragePriceRequest is one documented trading/funding average-price calculation.
type AveragePriceRequest struct {
	Symbol    string
	Amount    types.Decimal
	Period    int
	RateLimit *types.Decimal
}

// Validate checks the symbol, amount, and funding-only period before I/O.
func (r AveragePriceRequest) Validate() error {
	sym, e := types.ParseSymbol(r.Symbol)
	if e != nil {
		return e
	}
	if !r.Amount.IsValid() || r.Amount.IsZero() {
		return &types.ValidationError{Field: "amount", Message: "non-zero calculation amount required"}
	}
	if sym.IsFunding() {
		if r.Period < 2 || r.Period > 120 {
			return &types.ValidationError{Field: "period", Message: "funding calculation requires period 2..120"}
		}
	} else if r.Period != 0 {
		return &types.ValidationError{Field: "period", Message: "period is only valid for funding calculations"}
	}
	if r.RateLimit != nil && (!r.RateLimit.IsValid() || r.RateLimit.Sign() <= 0) {
		return &types.ValidationError{Field: "rate_limit", Message: "positive rate/price limit required"}
	}
	return nil
}

// MarketAveragePriceTyped sends documented POST query parameters; it is a read calculation.
func (s *Service) MarketAveragePriceTyped(ctx context.Context, q AveragePriceRequest) (*types.Response[MarketAvgPrice], error) {
	if e := q.Validate(); e != nil {
		return nil, e
	}
	params := map[string]string{"symbol": q.Symbol, "amount": q.Amount.CanonicalString()}
	if q.Period != 0 {
		params["period"] = itoaMarket(int64(q.Period))
	}
	if q.RateLimit != nil {
		params["rate_limit"] = q.RateLimit.CanonicalString()
	}
	r, e := s.t.PostPublicQuery(ctx, "calc/trade/avg", params, httpx.RequestContext{Bucket: "calc", Idempotent: true})
	if e != nil {
		return nil, e
	}
	a, e := wire.AsArr(r.Data, "average price")
	if e != nil {
		return nil, e
	}
	avg, e := wire.RequiredDecimal(a, 0)
	if e != nil {
		return nil, e
	}
	amount, e := wire.RequiredDecimal(a, 1)
	if e != nil {
		return nil, e
	}
	out := MarketAvgPrice{Amount: amount}
	if types.Symbol(q.Symbol).IsFunding() {
		out.RateAvg = avg
	} else {
		out.PriceAvg = avg
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

func itoaMarket(v int64) string { return strconv.FormatInt(v, 10) }
