package positions

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"net/url"
	"strings"
)

// CollateralLimits is the minimum/maximum assigned derivative collateral.
type CollateralLimits struct {
	Minimum types.Decimal `json:"minimum"`
	Maximum types.Decimal `json:"maximum"`
}

// TradableBalances describes adjusted and total buying power in both currencies.
type TradableBalances struct {
	Quote      types.Decimal `json:"quote"`
	QuoteTotal types.Decimal `json:"quote_total"`
	Base       types.Decimal `json:"base"`
	BaseTotal  types.Decimal `json:"base_total"`
}

// IncreasePreview describes the position, balance and funding impact of an increase.
type IncreasePreview struct {
	MaximumPosition         types.Decimal    `json:"maximum_position"`
	CurrentPosition         types.Decimal    `json:"current_position"`
	BaseCurrencyBalance     types.Decimal    `json:"base_currency_balance"`
	Tradable                TradableBalances `json:"tradable"`
	FundingAvailable        types.Decimal    `json:"funding_available"`
	FundingValue            types.Decimal    `json:"funding_value"`
	FundingRequired         types.Decimal    `json:"funding_required"`
	FundingValueCurrency    string           `json:"funding_value_currency"`
	FundingRequiredCurrency string           `json:"funding_required_currency"`
}

// SetDerivativeCollateralAmount sets an exact collateral AMOUNT, not a currency.
// It parses the documented [STATUS] result and never retries an ambiguous write.
func (s *Service) SetDerivativeCollateralAmount(ctx context.Context, symbol string, amount types.Decimal) (*types.Response[types.OperationStatus], error) {
	if err := validateDerivative(symbol); err != nil {
		return nil, err
	}
	if !amount.IsValid() || amount.Sign() < 0 {
		return nil, &types.ValidationError{Field: "collateral", Message: "collateral must be a non-negative decimal amount"}
	}
	r, e := s.t.PostAuth(ctx, "v2/auth/w/deriv/collateral/set", map[string]any{"symbol": symbol, "collateral": amount.CanonicalString()}, writeBucket())
	if e != nil {
		return nil, e
	}
	status, e := wire.Status(r.Data)
	if e != nil {
		return nil, e
	}
	return types.NewResponse(status, httpx.Meta(r)), nil
}

func validateDerivative(symbol string) error {
	sym, e := types.ParseSymbol(symbol)
	if e != nil {
		return e
	}
	if !sym.IsTrading() || !strings.Contains(symbol, "F0:") {
		return &types.ValidationError{Field: "symbol", Message: "derivative trading symbol required"}
	}
	return nil
}

// DerivativeCollateralLimitsTyped returns exact collateral bounds.
func (s *Service) DerivativeCollateralLimitsTyped(ctx context.Context, symbol string) (*types.Response[CollateralLimits], error) {
	if e := validateDerivative(symbol); e != nil {
		return nil, e
	}
	r, e := s.t.PostAuth(ctx, "v2/auth/calc/deriv/collateral/limits", map[string]any{"symbol": symbol}, readBucket())
	if e != nil {
		return nil, e
	}
	a, e := wire.AsArr(r.Data, "collateral limits")
	if e != nil {
		return nil, e
	}
	lo, e := wire.RequiredDecimal(a, 0)
	if e != nil {
		return nil, e
	}
	hi, e := wire.RequiredDecimal(a, 1)
	if e != nil {
		return nil, e
	}
	if lo.IsNegative() || lo.Cmp(hi) > 0 {
		return nil, &types.ProtocolError{Message: "invalid collateral bounds"}
	}
	return types.NewResponse(CollateralLimits{Minimum: lo, Maximum: hi}, httpx.Meta(r)), nil
}

// IncreaseInfoTyped returns the documented nested position/balance/funding model.
func (s *Service) IncreaseInfoTyped(ctx context.Context, symbol string, amount types.Decimal) (*types.Response[IncreasePreview], error) {
	if e := (IncreaseRequest{Symbol: symbol, Amount: amount}).Validate(); e != nil {
		return nil, e
	}
	r, e := s.t.PostAuth(ctx, "v2/auth/r/position/increase/info", map[string]any{"symbol": symbol, "amount": amount.CanonicalString()}, readBucket())
	if e != nil {
		return nil, e
	}
	a, e := wire.AsArr(r.Data, "position increase info")
	if e != nil {
		return nil, e
	}
	if e = wire.AssertMinLength(a, 6, "position increase info"); e != nil {
		return nil, e
	}
	get := func(v any, n int) ([]any, error) {
		a, e := wire.AsArr(v, "increase info component")
		if e != nil {
			return nil, e
		}
		return a, wire.AssertMinLength(a, n, "increase info component")
	}
	p, e := get(a[0], 2)
	if e != nil {
		return nil, e
	}
	b, e := get(a[1], 3)
	if e != nil {
		return nil, e
	}
	t, e := get(b[1], 4)
	if e != nil {
		return nil, e
	}
	f, e := get(a[4], 2)
	if e != nil {
		return nil, e
	}
	c, e := get(a[5], 2)
	if e != nil {
		return nil, e
	}
	var out IncreasePreview
	for _, x := range []struct {
		p *types.Decimal
		a []any
		i int
	}{{&out.MaximumPosition, p, 0}, {&out.CurrentPosition, p, 1}, {&out.BaseCurrencyBalance, b, 0}, {&out.FundingAvailable, b, 2}, {&out.Tradable.Quote, t, 0}, {&out.Tradable.QuoteTotal, t, 1}, {&out.Tradable.Base, t, 2}, {&out.Tradable.BaseTotal, t, 3}, {&out.FundingValue, f, 0}, {&out.FundingRequired, f, 1}} {
		*x.p, e = wire.RequiredDecimal(x.a, x.i)
		if e != nil {
			return nil, e
		}
	}
	out.FundingValueCurrency, e = wire.RequiredString(c, 0)
	if e != nil {
		return nil, e
	}
	out.FundingRequiredCurrency, e = wire.RequiredString(c, 1)
	if e != nil {
		return nil, e
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// MarginInfoTyped supports base, a trading symbol, and sym_all.
func (s *Service) MarginInfoTyped(ctx context.Context, key string) (*types.Response[[]types.MarginInfo], error) {
	if key == "" {
		key = "base"
	}
	if key != "base" && key != "sym_all" {
		if _, e := types.ParseSymbol(key); e != nil {
			return nil, e
		}
	}
	r, e := s.t.PostAuth(ctx, "v2/auth/r/info/margin/"+url.PathEscape(key), nil, readBucket())
	if e != nil {
		return nil, e
	}
	list, e := wire.AsArr(r.Data, "margin info")
	if e != nil {
		return nil, e
	}
	out := []types.MarginInfo{}
	if key != "sym_all" {
		v, e := wire.MarginInfo(list)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	} else {
		for _, row := range list {
			v, e := wire.MarginInfo(row)
			if e != nil {
				return nil, e
			}
			out = append(out, v)
		}
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}
