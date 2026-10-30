package funding

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// InfoTyped retrieves a funding currency's exact yield/duration calculations.
func (s *Service) InfoTyped(ctx context.Context, symbol string) (*types.Response[types.FundingInfo], error) {
	sym, e := types.ParseSymbol(symbol)
	if e != nil {
		return nil, e
	}
	if !sym.IsFunding() {
		return nil, &types.ValidationError{Field: "symbol", Message: "funding currency required"}
	}
	r, e := s.t.PostAuth(ctx, "v2/auth/r/info/funding/"+sym.String(), nil, readBucket())
	if e != nil {
		return nil, e
	}
	v, e := wire.FundingInfo(r.Data)
	if e != nil {
		return nil, e
	}
	return types.NewResponse(v, httpx.Meta(r)), nil
}
