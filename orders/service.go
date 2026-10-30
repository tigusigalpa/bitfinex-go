package orders

import (
	"context"
	"fmt"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Service exposes the order endpoints. All write methods are
// idempotent=false: they are never automatically retried.
type Service struct {
	t *httpx.Transport
}

// New binds a Service to a transport.
func New(t *httpx.Transport) *Service { return &Service{t: t} }

func readBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "orders-read", Idempotent: true}
}

func writeBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "orders-write", Idempotent: false}
}

// Active returns POST /auth/r/orders[/{symbol}] — open orders,
// optionally filtered by symbol.
func (s *Service) Active(ctx context.Context, symbol string) (*types.Response[[]Order], error) {
	path := "v2/auth/r/orders"
	if symbol != "" {
		sym, err := types.ParseSymbol(symbol)
		if err != nil {
			return nil, err
		}
		path += "/" + sym.String()
	}
	r, err := s.t.PostAuth(ctx, path, nil, readBucket())
	if err != nil {
		return nil, err
	}
	return s.orderList(r)
}

// Submit sends POST /auth/w/order/submit. NEVER retried: if the
// outcome is ambiguous (timeout, transport failure), reconcile
// via ClientOrderID against Active/History — do not resubmit
// blindly.
func (s *Service) Submit(ctx context.Context, req SubmitRequest) (*types.Response[WriteResult], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/order/submit", req.Body(), writeBucket())
	if err != nil {
		return nil, err
	}
	return s.writeResult(r)
}

// Update sends POST /auth/w/order/update. Never retried.
func (s *Service) Update(ctx context.Context, req UpdateRequest) (*types.Response[WriteResult], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/order/update", req.Body(), writeBucket())
	if err != nil {
		return nil, err
	}
	return s.writeResult(r)
}

// Cancel sends POST /auth/w/order/cancel. Never retried.
func (s *Service) Cancel(ctx context.Context, req CancelRequest) (*types.Response[WriteResult], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/order/cancel", req.Body(), writeBucket())
	if err != nil {
		return nil, err
	}
	return s.writeResult(r)
}

// CancelMultiRequest selects orders for POST
// /auth/w/order/cancel/multi. Set All to cancel everything, or
// pass ID/CID/GID sets (at least one selector required).
type CancelMultiRequest struct {
	All  bool
	IDs  []int64
	CIDs []ClientOrderID
	GIDs []int64
}

// Validate enforces at least one selector.
func (r CancelMultiRequest) Validate() error {
	selectors := 0
	if len(r.IDs) > 0 {
		selectors++
	}
	if len(r.CIDs) > 0 {
		selectors++
	}
	if len(r.GIDs) > 0 {
		selectors++
	}
	if r.All {
		selectors++
	}
	if selectors != 1 {
		return &types.ValidationError{Field: "selectors", Message: "choose exactly one of ids, cids, gids or all"}
	}
	for _, id := range append(append([]int64{}, r.IDs...), r.GIDs...) {
		if id <= 0 {
			return &types.ValidationError{Field: "id", Message: "ids must be positive"}
		}
	}
	for _, cid := range r.CIDs {
		if err := cid.Validate(); err != nil {
			return err
		}
	}
	if r.All {
		return nil
	}
	if len(r.IDs) > 0 || len(r.CIDs) > 0 || len(r.GIDs) > 0 {
		return nil
	}
	return &types.ValidationError{Field: "selectors", Message: "cancel multi needs ids, cids, gids, or all=true"}
}

// Body renders the request body.
func (r CancelMultiRequest) Body() map[string]any {
	body := map[string]any{}
	if r.All {
		body["all"] = 1
		return body
	}
	if len(r.IDs) > 0 {
		body["id"] = r.IDs
	}
	if len(r.CIDs) > 0 {
		cids := make([][]any, len(r.CIDs))
		for i, c := range r.CIDs {
			cids[i] = []any{c.ID, c.Date}
		}
		body["cid"] = cids
	}
	if len(r.GIDs) > 0 {
		body["gid"] = r.GIDs
	}
	return body
}

// CancelMulti sends POST /auth/w/order/cancel/multi. Never
// retried.
func (s *Service) CancelMulti(ctx context.Context, req CancelMultiRequest) (*types.Response[WriteResult], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/order/cancel/multi", req.Body(), writeBucket())
	if err != nil {
		return nil, err
	}
	return s.writeResult(r)
}

// Multi sends POST /auth/w/order/multi — combined
// on/ou/oc/oc_multi/ox_multi ops in one write. Each op is
// [verb, payload] where payload uses the same field names as the
// corresponding single-op request body. Never retried.
func (s *Service) Multi(ctx context.Context, ops []MultiOp) (*types.Response[WriteResult], error) {
	if len(ops) == 0 {
		return nil, &types.ValidationError{Field: "ops", Message: "multi requires at least one op"}
	}
	raw := make([]any, len(ops))
	for i, op := range ops {
		if err := op.Validate(); err != nil {
			return nil, err
		}
		raw[i] = []any{op.Op, op.Args}
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/order/multi", map[string]any{"ops": raw}, writeBucket())
	if err != nil {
		return nil, err
	}
	return s.writeResult(r)
}

// History returns POST /auth/r/orders[/hist|/{symbol}/hist] —
// closed orders, optionally filtered by symbol. Documented
// retention: ~2 weeks.
func (s *Service) History(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]Order], error) {
	path := "v2/auth/r/orders"
	if symbol != "" {
		sym, err := types.ParseSymbol(symbol)
		if err != nil {
			return nil, err
		}
		path += "/" + sym.String()
	}
	path += "/hist"
	r, err := s.t.PostAuth(ctx, path, historyBody(q), readBucket())
	if err != nil {
		return nil, err
	}
	return s.orderList(r)
}

// OrderTrades returns POST /auth/r/order/{symbol}:{id}/trades —
// executions of one order. Documented retention: ~10 days.
func (s *Service) OrderTrades(ctx context.Context, symbol string, orderID int64) (*types.Response[[]TradeExecution], error) {
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if orderID <= 0 {
		return nil, &types.ValidationError{Field: "order_id", Message: "order id must be positive"}
	}
	path := fmt.Sprintf("v2/auth/r/order/%s:%d/trades", sym.String(), orderID)
	r, err := s.t.PostAuth(ctx, path, nil, readBucket())
	if err != nil {
		return nil, err
	}
	return s.tradeList(r)
}

// Trades returns POST /auth/r/trades/hist[/{symbol}] — your
// executions, optionally filtered by symbol.
func (s *Service) Trades(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]TradeExecution], error) {
	path := "v2/auth/r/trades"
	if symbol != "" {
		sym, err := types.ParseSymbol(symbol)
		if err != nil {
			return nil, err
		}
		path += "/" + sym.String()
	}
	path += "/hist"
	r, err := s.t.PostAuth(ctx, path, q.Body(), readBucket())
	if err != nil {
		return nil, err
	}
	return s.tradeList(r)
}

// OTCHistory returns POST /auth/r/orders/otc/{symbol}/hist —
// OTC desk orders.
// Deprecated: use OTCHistoryTyped for the documented typed contract.
func (s *Service) OTCHistory(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]Order], error) {
	r, e := s.OTCHistoryTyped(ctx, symbol, q)
	if e != nil {
		return nil, e
	}
	out := make([]Order, 0, len(r.Data))
	for _, o := range r.Data {
		out = append(out, Order{ID: o.ID, Symbol: o.Symbol, MTSCreate: o.MTSCreate, MTSUpdate: o.MTSUpdate, Amount: o.Amount, AmountOrig: o.Amount, Price: o.Price, Status: o.Status, Type: "OTC"})
	}
	return types.NewResponse(out, r.Meta), nil
}

// ---- shared decoders ----

func (s *Service) orderList(r *httpx.Result) (*types.Response[[]Order], error) {
	rows, err := wire.Rows(r.Data, "orders")
	if err != nil {
		return nil, err
	}
	out := make([]Order, 0, len(rows))
	for _, row := range rows {
		o, derr := FromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, o)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

func (s *Service) tradeList(r *httpx.Result) (*types.Response[[]TradeExecution], error) {
	rows, err := wire.Rows(r.Data, "trade executions")
	if err != nil {
		return nil, err
	}
	out := make([]TradeExecution, 0, len(rows))
	for _, row := range rows {
		te, derr := TradeExecutionFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, te)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

func (s *Service) writeResult(r *httpx.Result) (*types.Response[WriteResult], error) {
	raw, err := wire.AsArr(r.Data, "order write result")
	if err != nil {
		return nil, err
	}
	res, err := writeResultFromNotification(raw)
	if err != nil {
		return nil, err
	}
	return types.NewResponse(res, httpx.Meta(r)), nil
}
