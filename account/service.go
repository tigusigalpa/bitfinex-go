package account

import (
	"context"
	"net/url"
	"strings"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Service exposes the account endpoints.
type Service struct {
	t *httpx.Transport
}

// New binds a Service to a transport.
func New(t *httpx.Transport) *Service { return &Service{t: t} }

func readBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "account-read", Idempotent: true}
}

func writeBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "account-write", Idempotent: false}
}

// UserInfo returns POST /auth/r/info/user.
func (s *Service) UserInfo(ctx context.Context) (*types.Response[UserInfo], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/info/user", nil, readBucket())
	if err != nil {
		return nil, err
	}
	raw, err := wire.AsArr(r.Data, "user info")
	if err != nil {
		return nil, err
	}
	u, err := userInfoFromWire(raw)
	if err != nil {
		return nil, err
	}
	return types.NewResponse(u, httpx.Meta(r)), nil
}

// Summary returns POST /auth/r/summary — 30-day fee/LEO/
// derivatives summary. The shape is semi-documented; rows are
// preserved untyped.
func (s *Service) Summary(ctx context.Context) (*types.Response[any], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/summary", nil, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// LoginHistory returns POST /auth/r/logins/hist.
func (s *Service) LoginHistory(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]LoginEntry], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/logins/hist", q.Body(), readBucket())
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "login history")
	if err != nil {
		return nil, err
	}
	out := make([]LoginEntry, 0, len(rows))
	for _, row := range rows {
		e, derr := loginEntryFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, e)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// Permissions returns POST /auth/r/permissions — the key's
// permission matrix (shape varies; preserved untyped).
func (s *Service) Permissions(ctx context.Context) (*types.Response[any], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/permissions", nil, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// GenerateToken sends POST /auth/w/token — mints a short-lived
// auth token for this API key (e.g. for untrusted contexts).
// caps e.g. ["orders","funding"]. Write: never retried.
func (s *Service) GenerateToken(ctx context.Context, ttlSeconds int, caps []string, writePermission bool) (*types.Response[any], error) {
	if ttlSeconds <= 0 {
		return nil, &types.ValidationError{Field: "ttl", Message: "token ttl must be positive seconds"}
	}
	body := map[string]any{"ttl": ttlSeconds}
	if len(caps) > 0 {
		body["caps"] = caps
	}
	if writePermission {
		body["writePermission"] = true
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/token", body, writeBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// AuditHistory returns POST /auth/r/audit/hist — account change
// audit trail (shape varies; preserved untyped).
func (s *Service) AuditHistory(ctx context.Context, q *types.HistoryQuery) (*types.Response[any], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/audit/hist", q.Body(), readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// Ledgers returns POST /auth/r/ledgers[/{currency}]/hist.
// Documented retention: up to ~6 years.
func (s *Service) Ledgers(ctx context.Context, currency string, q *types.HistoryQuery) (*types.Response[[]LedgerEntry], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	path := "v2/auth/r/ledgers"
	if currency != "" {
		path += "/" + url.PathEscape(strings.ToUpper(strings.TrimSpace(currency)))
	}
	path += "/hist"
	r, err := s.t.PostAuth(ctx, path, q.Body(), readBucket())
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "ledgers")
	if err != nil {
		return nil, err
	}
	out := make([]LedgerEntry, 0, len(rows))
	for _, row := range rows {
		e, derr := ledgerEntryFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, e)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// AvailableBalanceForOrder POSTs /auth/calc/order/avail — the
// balance available for an order/offer with the given
// parameters. Calc endpoint: idempotent.
func (s *Service) AvailableBalanceForOrder(ctx context.Context, symbol, orderType string, amount types.Decimal, rate *types.Decimal, lev *int64) (*types.Response[types.Decimal], error) {
	if _, err := types.ParseSymbol(symbol); err != nil {
		return nil, err
	}
	if strings.TrimSpace(orderType) == "" {
		return nil, &types.ValidationError{Field: "type", Message: "order type required"}
	}
	if !amount.IsValid() {
		return nil, &types.ValidationError{Field: "amount", Message: "amount must be a valid decimal"}
	}
	body := map[string]any{
		"symbol": symbol,
		"type":   orderType,
		"amount": amount.CanonicalString(),
	}
	if rate != nil {
		body["rate"] = rate.CanonicalString()
	}
	if lev != nil {
		body["lev"] = *lev
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/calc/order/avail", body, readBucket())
	if err != nil {
		return nil, err
	}
	raw, err := wire.AsArr(r.Data, "order avail")
	if err != nil {
		return nil, err
	}
	d, _, derr := wire.Decimal(raw, 0)
	if derr != nil {
		return nil, derr
	}
	return types.NewResponse(d, httpx.Meta(r)), nil
}

// Alerts returns POST /auth/r/alerts — active price alerts.
func (s *Service) Alerts(ctx context.Context) (*types.Response[[]Alert], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/alerts", nil, readBucket())
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "alerts")
	if err != nil {
		return nil, err
	}
	out := make([]Alert, 0, len(rows))
	for _, row := range rows {
		a, derr := alertFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, a)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// AlertSetRequest is POST /auth/w/alert/set.
type AlertSetRequest struct {
	Type   string        // e.g. "price" — required
	Symbol string        // trading symbol — required
	Price  types.Decimal // required
}

// Validate enforces the local contract.
func (r AlertSetRequest) Validate() error {
	if strings.TrimSpace(r.Type) == "" {
		return &types.ValidationError{Field: "type", Message: "alert type required"}
	}
	if _, err := types.ParseSymbol(r.Symbol); err != nil {
		return err
	}
	if !r.Price.IsValid() {
		return &types.ValidationError{Field: "price", Message: "alert price must be a valid decimal"}
	}
	return nil
}

// AlertSet sends POST /auth/w/alert/set. Never retried.
func (s *Service) AlertSet(ctx context.Context, req AlertSetRequest) (*types.Response[any], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/alert/set", map[string]any{
		"type":   req.Type,
		"symbol": req.Symbol,
		"price":  req.Price.CanonicalString(),
	}, writeBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// AlertDelete sends POST /auth/w/alert/price:{symbol}:{price}/del.
// Never retried.
func (s *Service) AlertDelete(ctx context.Context, symbol string, price types.Decimal) (*types.Response[any], error) {
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if !price.IsValid() {
		return nil, &types.ValidationError{Field: "price", Message: "alert price must be a valid decimal"}
	}
	path := "v2/auth/w/alert/price:" + sym.String() + ":" + price.CanonicalString() + "/del"
	r, err := s.t.PostAuth(ctx, path, nil, writeBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// Settings returns POST /auth/r/settings — the user settings
// map, or only the given keys.
func (s *Service) Settings(ctx context.Context, keys []string) (*types.Response[any], error) {
	body := map[string]any{}
	if len(keys) > 0 {
		body["settings"] = keys
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/settings", body, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// SettingsSet sends POST /auth/w/settings/set. Never retried.
func (s *Service) SettingsSet(ctx context.Context, settings map[string]any) (*types.Response[any], error) {
	if len(settings) == 0 {
		return nil, &types.ValidationError{Field: "settings", Message: "settings map must not be empty"}
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/settings/set",
		map[string]any{"settings": settings}, writeBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// SettingsDelete sends POST /auth/w/settings/del for the given
// keys. Never retried.
func (s *Service) SettingsDelete(ctx context.Context, keys []string) (*types.Response[any], error) {
	if len(keys) == 0 {
		return nil, &types.ValidationError{Field: "settings", Message: "settings keys must not be empty"}
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/w/settings/del",
		map[string]any{"settings": keys}, writeBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

func decodeNotification(r *httpx.Result) (*orders.Notification, error) {
	raw, err := wire.AsArr(r.Data, "notification")
	if err != nil {
		return nil, err
	}
	n, err := orders.NotificationFromWire(raw)
	if err != nil {
		return nil, err
	}
	return &n, nil
}
