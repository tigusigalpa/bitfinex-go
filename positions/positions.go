// Package positions implements the authenticated position and
// derivatives endpoints: active positions, history, snapshot,
// audit, claim/increase, funding type, collateral management
// and margin info.
//
// Write endpoints are never automatically retried.
package positions

import (
	"context"
	"net/url"
	"strings"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Position is one position positional row:
// [SYMBOL, STATUS, AMOUNT, BASE_PRICE, MARGIN_FUNDING,
//
//	MARGIN_FUNDING_TYPE, PL, PL_PERC, PRICE_LIQ, LEVERAGE, _, _,
//	POSITION_ID, MTS_CREATE, MTS_UPDATE, _, TYPE, _, COLLATERAL,
//	COLLATERAL_MIN, META].
type Position struct {
	Symbol            types.Symbol  `json:"symbol"`
	Status            string        `json:"status"`
	Amount            types.Decimal `json:"amount"`
	BasePrice         types.Decimal `json:"base_price"`
	MarginFunding     types.Decimal `json:"margin_funding"`
	MarginFundingType int64         `json:"margin_funding_type"` // 0 daily, 1 term
	PL                types.Decimal `json:"pl"`
	PLPerc            types.Decimal `json:"pl_perc"`
	PriceLiq          types.Decimal `json:"price_liq"`
	Leverage          types.Decimal `json:"leverage"`
	ID                int64         `json:"id"`
	MTSCreate         types.Millis  `json:"mts_create"`
	MTSUpdate         types.Millis  `json:"mts_update"`
	Type              int64         `json:"type"`
	Collateral        types.Decimal `json:"collateral"`
	CollateralMin     types.Decimal `json:"collateral_min"`
	Meta              any           `json:"meta,omitempty"`
}

// FromWire decodes a position positional array. Exported for the
// WebSocket account stream (ps/pn/pu/pc events).
func FromWire(a []any) (Position, error) {
	p := Position{}
	if err := wire.AssertMinLength(a, 10, "position"); err != nil {
		return p, err
	}
	var err error
	if sym, serr := wire.Str(a, 0); serr != nil {
		return p, serr
	} else {
		p.Symbol = types.Symbol(sym)
	}
	if p.Status, err = wire.Str(a, 1); err != nil {
		return p, err
	}
	if p.Amount, err = wire.DecOr(a, 2, types.Decimal{}); err != nil {
		return p, err
	}
	if p.BasePrice, err = wire.DecOr(a, 3, types.Decimal{}); err != nil {
		return p, err
	}
	if p.MarginFunding, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return p, err
	}
	p.MarginFundingType = wire.IntOr(a, 5, 0)
	if p.PL, err = wire.DecOr(a, 6, types.Decimal{}); err != nil {
		return p, err
	}
	if p.PLPerc, err = wire.DecOr(a, 7, types.Decimal{}); err != nil {
		return p, err
	}
	if p.PriceLiq, err = wire.DecOr(a, 8, types.Decimal{}); err != nil {
		return p, err
	}
	if p.Leverage, err = wire.DecOr(a, 9, types.Decimal{}); err != nil {
		return p, err
	}
	p.ID = wire.IntOr(a, 12, 0)
	if mts, _, merr := wire.Mts(a, 13); merr != nil {
		return p, merr
	} else {
		p.MTSCreate = mts
	}
	if mts, _, merr := wire.Mts(a, 14); merr != nil {
		return p, merr
	} else {
		p.MTSUpdate = mts
	}
	p.Type = wire.IntOr(a, 16, 0)
	if p.Collateral, err = wire.DecOr(a, 18, types.Decimal{}); err != nil {
		return p, err
	}
	if p.CollateralMin, err = wire.DecOr(a, 19, types.Decimal{}); err != nil {
		return p, err
	}
	if v := wire.At(a, 20); v != nil {
		p.Meta = v
	}
	return p, nil
}

// ClaimRequest is POST /auth/w/position/claim — claim a position
// using available balance.
type ClaimRequest struct {
	ID     int64          // position id — required
	Amount *types.Decimal // optional partial claim
}

// Validate enforces the local contract.
func (r ClaimRequest) Validate() error {
	if r.ID <= 0 {
		return &types.ValidationError{Field: "id", Message: "position id must be positive"}
	}
	return nil
}

// Body renders the request body.
func (r ClaimRequest) Body() map[string]any {
	body := map[string]any{"id": r.ID}
	if r.Amount != nil {
		body["amount"] = r.Amount.CanonicalString()
	}
	return body
}

// IncreaseRequest is POST /auth/w/position/increase.
type IncreaseRequest struct {
	Symbol string        // trading symbol — required
	Amount types.Decimal // required non-zero
}

// Validate enforces the local contract.
func (r IncreaseRequest) Validate() error {
	if _, err := types.ParseSymbol(r.Symbol); err != nil {
		return err
	}
	if !r.Amount.IsValid() || r.Amount.IsZero() {
		return &types.ValidationError{Field: "amount", Message: "increase amount must be non-zero"}
	}
	return nil
}

// Body renders the request body.
func (r IncreaseRequest) Body() map[string]any {
	return map[string]any{"symbol": r.Symbol, "amount": r.Amount.CanonicalString()}
}

// Service exposes position endpoints.
type Service struct {
	t *httpx.Transport
}

// New binds a Service to a transport.
func New(t *httpx.Transport) *Service { return &Service{t: t} }

func readBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "positions", Idempotent: true}
}

func writeBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "positions", Idempotent: false}
}

// Active returns POST /auth/r/positions.
func (s *Service) Active(ctx context.Context) (*types.Response[[]Position], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/positions", nil, readBucket())
	if err != nil {
		return nil, err
	}
	return s.positionList(r)
}

// History returns POST /auth/r/positions/hist.
func (s *Service) History(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Position], error) {
	if err := q.ValidateFor(100, false); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/positions/hist", historyBody(q), readBucket())
	if err != nil {
		return nil, err
	}
	return positionHistoryList(r)
}

// Snapshot returns POST /auth/r/positions/snap.
func (s *Service) Snapshot(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Position], error) {
	if err := q.ValidateFor(500, false); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/positions/snap", historyBody(q), readBucket())
	if err != nil {
		return nil, err
	}
	return positionHistoryList(r)
}

// Audit returns POST /auth/r/positions/audit for one position id.
func (s *Service) Audit(ctx context.Context, positionID int64, q *types.HistoryQuery) (*types.Response[[]Position], error) {
	if err := q.ValidateFor(500, false); err != nil {
		return nil, err
	}
	if positionID <= 0 {
		return nil, &types.ValidationError{Field: "id", Message: "position id must be positive"}
	}
	body := historyBody(q)
	body["id"] = []int64{positionID}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/positions/audit", body, readBucket())
	if err != nil {
		return nil, err
	}
	return positionHistoryList(r)
}

// Claim sends POST /auth/w/position/claim. Never retried.
func (s *Service) Claim(ctx context.Context, req ClaimRequest) (*types.Response[orders.Notification], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/position/claim", req.Body(), writeBucket()))
}

// Increase sends POST /auth/w/position/increase. Never retried.
func (s *Service) Increase(ctx context.Context, req IncreaseRequest) (*types.Response[orders.Notification], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/position/increase", req.Body(), writeBucket()))
}

// IncreaseInfo POSTs /auth/r/position/increase/info — a
// funding/impact preview for a hypothetical increase. Idempotent.
// Deprecated: use IncreaseInfoTyped for the documented typed contract.
func (s *Service) IncreaseInfo(ctx context.Context, symbol string, amount types.Decimal) (*types.Response[any], error) {
	if _, err := types.ParseSymbol(symbol); err != nil {
		return nil, err
	}
	if !amount.IsValid() || amount.IsZero() {
		return nil, &types.ValidationError{Field: "amount", Message: "increase info amount must be non-zero"}
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/position/increase/info", map[string]any{
		"symbol": symbol, "amount": amount.CanonicalString(),
	}, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// UpdateFundingType sends POST /auth/w/position/update/funding/type
// — fundingType: 0 = daily, 1 = term. Never retried.
func (s *Service) UpdateFundingType(ctx context.Context, symbol string, fundingType int) (*types.Response[orders.Notification], error) {
	if _, err := types.ParseSymbol(symbol); err != nil {
		return nil, err
	}
	if fundingType != 0 && fundingType != 1 {
		return nil, &types.ValidationError{Field: "type", Message: "funding type must be 0 (daily) or 1 (term)"}
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/position/update/funding/type",
		map[string]any{"symbol": symbol, "type": fundingType}, writeBucket()))
}

// SetDerivativeCollateral accepts a decimal amount string and adapts the status
// array to the legacy Notification result. Type is "collateral-status-adapter";
// this is an SDK compatibility envelope, not a server notification.
// Deprecated: use SetDerivativeCollateralAmount for the native typed status.
func (s *Service) SetDerivativeCollateral(ctx context.Context, symbol, collateralAmount string) (*types.Response[orders.Notification], error) {
	amount, err := types.NewDecimal(collateralAmount)
	if err != nil {
		return nil, &types.ValidationError{Field: "collateral", Message: "expected decimal collateral amount, not a currency"}
	}
	r, err := s.SetDerivativeCollateralAmount(ctx, symbol, amount)
	if err != nil {
		return nil, err
	}
	status := "ERROR"
	if r.Data.Success {
		status = "SUCCESS"
	}
	return types.NewResponse(orders.Notification{Type: "collateral-status-adapter", Status: status, Data: r.Data}, r.Meta), nil
}

// DerivativeCollateralLimits POSTs /auth/calc/deriv/collateral/limits
// — allowed collateral range. Idempotent calc endpoint.
// Deprecated: use DerivativeCollateralLimitsTyped for the documented typed contract.
func (s *Service) DerivativeCollateralLimits(ctx context.Context, symbol string) (*types.Response[any], error) {
	if _, err := types.ParseSymbol(symbol); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/calc/deriv/collateral/limits",
		map[string]any{"symbol": symbol}, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// MarginInfo POSTs /auth/r/info/margin/{key} — key "base", "all"
// or a symbol key like "sym_tBTCUSD". Returns one info envelope
// or a list of them depending on the key.
// Deprecated: use MarginInfoTyped for the documented typed contract.
func (s *Service) MarginInfo(ctx context.Context, key string) (*types.Response[[]types.KeyValueInfo], error) {
	if strings.TrimSpace(key) == "" {
		key = "base"
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/info/margin/"+url.PathEscape(key), nil, readBucket())
	if err != nil {
		return nil, err
	}
	data, err := wire.AsArr(r.Data, "margin info")
	if err != nil {
		return nil, err
	}
	out := []types.KeyValueInfo{}
	if len(data) > 0 {
		if _, nested := data[0].([]any); nested {
			for _, row := range data {
				if arr, ok := row.([]any); ok {
					out = append(out, types.KeyValueInfoFromSlice(arr))
				}
			}
		} else {
			out = append(out, types.KeyValueInfoFromSlice(data))
		}
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// ---- shared decoders ----

func (s *Service) positionList(r *httpx.Result) (*types.Response[[]Position], error) {
	rows, err := wire.Rows(r.Data, "positions")
	if err != nil {
		return nil, err
	}
	out := make([]Position, 0, len(rows))
	for _, row := range rows {
		p, derr := FromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, p)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

func (s *Service) notification(r *httpx.Result, err error) (*types.Response[orders.Notification], error) {
	if err != nil {
		return nil, err
	}
	raw, derr := wire.AsArr(r.Data, "notification")
	if derr != nil {
		return nil, derr
	}
	n, derr := orders.NotificationFromWire(raw)
	if derr != nil {
		return nil, derr
	}
	return types.NewResponse(n, httpx.Meta(r)), nil
}
