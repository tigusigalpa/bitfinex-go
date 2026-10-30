package funding

import (
	"context"
	"net/url"
	"strings"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Service exposes the margin-funding endpoints.
type Service struct {
	t *httpx.Transport
}

// New binds a Service to a transport.
func New(t *httpx.Transport) *Service { return &Service{t: t} }

func readBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "funding-read", Idempotent: true}
}

func writeBucket() httpx.RequestContext {
	return httpx.RequestContext{Bucket: "funding-write", Idempotent: false}
}

// Offers returns POST /auth/r/funding/offers[/{symbol}] — active
// funding offers, optionally filtered by symbol.
func (s *Service) Offers(ctx context.Context, symbol string) (*types.Response[[]FundingOffer], error) {
	path := "v2/auth/r/funding/offers"
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
	return decodeList(r, "funding offers", OfferFromWire)
}

// OfferRequest is POST /auth/w/funding/offer/submit.
type OfferRequest struct {
	Type   string        // TypeLend or TypeLoan — required
	Symbol string        // funding symbol, e.g. "fUSD" — required
	Amount types.Decimal // required non-zero
	Rate   types.Decimal // required, >= 0 ("0" = FRR)
	Period int           // days, within [2, 120]
	Flags  *int64
}

// Validate enforces the local contract before sending.
func (r OfferRequest) Validate() error {
	t := strings.ToUpper(strings.TrimSpace(r.Type))
	if t != TypeLimit && t != TypeFRRDeltaFix && t != TypeFRRDeltaVar {
		return &types.ValidationError{Field: "type", Message: "funding offer type must be LIMIT, FRRDELTAFIX or FRRDELTAVAR; direction is the amount sign"}
	}
	sym, err := types.ParseSymbol(r.Symbol)
	if err != nil {
		return err
	}
	if !sym.IsFunding() {
		return &types.ValidationError{Field: "symbol", Message: "funding offer requires a funding symbol (e.g. \"fUSD\")"}
	}
	if !r.Amount.IsValid() || r.Amount.IsZero() {
		return &types.ValidationError{Field: "amount", Message: "funding offer amount must be non-zero"}
	}
	if !r.Rate.IsValid() || (r.Rate.IsNegative() && t != TypeFRRDeltaFix) {
		return &types.ValidationError{Field: "rate", Message: "funding rate must be a non-negative decimal (use \"0\" for FRR)"}
	}
	if r.Period < 2 || r.Period > 120 {
		return &types.ValidationError{Field: "period", Message: "funding period must be within [2, 120] days"}
	}
	return nil
}

// Body renders the request body.
func (r OfferRequest) Body() map[string]any {
	body := map[string]any{
		"type":   strings.ToUpper(strings.TrimSpace(r.Type)),
		"symbol": r.Symbol,
		"amount": r.Amount.CanonicalString(),
		"rate":   r.Rate.CanonicalString(),
		"period": r.Period,
	}
	if r.Flags != nil {
		body["flags"] = *r.Flags
	}
	return body
}

// SubmitOffer sends POST /auth/w/funding/offer/submit.
// Never retried.
func (s *Service) SubmitOffer(ctx context.Context, req OfferRequest) (*types.Response[orders.Notification], error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/funding/offer/submit", req.Body(), writeBucket()))
}

// CancelOffer sends POST /auth/w/funding/offer/cancel for one
// offer id. Never retried.
func (s *Service) CancelOffer(ctx context.Context, offerID int64) (*types.Response[orders.Notification], error) {
	if offerID <= 0 {
		return nil, &types.ValidationError{Field: "id", Message: "offer id must be positive"}
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/funding/offer/cancel",
		map[string]any{"id": offerID}, writeBucket()))
}

// CancelAllOffers sends POST /auth/w/funding/offer/cancel/all
// for a funding symbol. Never retried.
func (s *Service) CancelAllOffers(ctx context.Context, symbol string) (*types.Response[orders.Notification], error) {
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	if !sym.IsFunding() {
		return nil, &types.ValidationError{Field: "symbol", Message: "cancel all offers requires a funding symbol"}
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/funding/offer/cancel/all",
		map[string]any{"currency": strings.TrimPrefix(sym.String(), "f")}, writeBucket()))
}

// Close sends POST /auth/w/funding/close — return funding early.
// Never retried.
func (s *Service) Close(ctx context.Context, id int64) (*types.Response[orders.Notification], error) {
	if id <= 0 {
		return nil, &types.ValidationError{Field: "id", Message: "funding id must be positive"}
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/funding/close",
		map[string]any{"id": id}, writeBucket()))
}

// AutoRenewRequest is POST /auth/w/funding/auto — enable or
// disable auto-renew with optional rate/period constraints.
type AutoRenewRequest struct {
	Status   bool           // required: enable (1) / disable (0)
	Currency string         // bare currency (e.g. "USD"), optional
	Amount   *types.Decimal // optional threshold
	Rate     *types.Decimal // optional rate
	Period   *int           // optional period days
}

// Body renders the request body.
func (r AutoRenewRequest) Body() map[string]any {
	status := 0
	if r.Status {
		status = 1
	}
	body := map[string]any{"status": status}
	if r.Currency != "" {
		body["currency"] = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(r.Currency), "f"))
	}
	if r.Amount != nil {
		body["amount"] = r.Amount.CanonicalString()
	}
	if r.Rate != nil {
		body["rate"] = r.Rate.CanonicalString()
	}
	if r.Period != nil {
		body["period"] = *r.Period
	}
	return body
}

// AutoRenew sends POST /auth/w/funding/auto. Never retried.
func (s *Service) AutoRenew(ctx context.Context, req AutoRenewRequest) (*types.Response[orders.Notification], error) {
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/funding/auto", req.Body(), writeBucket()))
}

// Keep sends POST /auth/w/funding/keep — keep funding taken from
// an offer of the given type ("loan" or "credit"). ids selects
// specific entries; empty means all. Never retried.
func (s *Service) Keep(ctx context.Context, fundingType string, ids []int64) (*types.Response[orders.Notification], error) {
	if fundingType != "loan" && fundingType != "credit" {
		return nil, &types.ValidationError{Field: "type", Message: `funding keep type must be "loan" or "credit"`}
	}
	body := map[string]any{"type": fundingType}
	if len(ids) > 0 {
		body["id"] = ids
	}
	return s.notification(s.t.PostAuth(ctx, "v2/auth/w/funding/keep", body, writeBucket()))
}

// OffersHistory returns POST /auth/r/funding/offers/{symbol}/hist.
func (s *Service) OffersHistory(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]FundingOffer], error) {
	if err := q.ValidateFor(500, false); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/funding/offers/"+sym.String()+"/hist", historyBody(q), readBucket())
	if err != nil {
		return nil, err
	}
	return decodeList(r, "funding offers history", OfferFromWire)
}

// Loans returns POST /auth/r/funding/loans/{symbol} — loans
// taken. Documented retention ~3 days.
func (s *Service) Loans(ctx context.Context, symbol string) (*types.Response[[]FundingLoan], error) {
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/funding/loans/"+sym.String(), nil, readBucket())
	if err != nil {
		return nil, err
	}
	return decodeList(r, "funding loans", LoanFromWire)
}

// LoansHistory returns POST /auth/r/funding/loans/{symbol}/hist.
// Documented retention ~3 days.
func (s *Service) LoansHistory(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]FundingLoan], error) {
	if err := q.ValidateFor(500, false); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/funding/loans/"+sym.String()+"/hist", historyBody(q), readBucket())
	if err != nil {
		return nil, err
	}
	return decodeList(r, "funding loans history", LoanFromWire)
}

// Credits returns POST /auth/r/funding/credits/{symbol} —
// credits provided. Documented retention ~3 days.
func (s *Service) Credits(ctx context.Context, symbol string) (*types.Response[[]FundingCredit], error) {
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/funding/credits/"+sym.String(), nil, readBucket())
	if err != nil {
		return nil, err
	}
	return decodeList(r, "funding credits", CreditFromWire)
}

// CreditsHistory returns POST /auth/r/funding/credits/{symbol}/hist.
// Documented retention ~3 days.
func (s *Service) CreditsHistory(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]FundingCredit], error) {
	if err := q.ValidateFor(500, false); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/funding/credits/"+sym.String()+"/hist", historyBody(q), readBucket())
	if err != nil {
		return nil, err
	}
	return decodeList(r, "funding credits history", CreditFromWire)
}

// TradesHistory returns POST /auth/r/funding/trades/{symbol}/hist.
func (s *Service) TradesHistory(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]FundingTrade], error) {
	if err := q.ValidateFor(10000, true); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/funding/trades/"+sym.String()+"/hist", q.Body(), readBucket())
	if err != nil {
		return nil, err
	}
	return decodeList(r, "funding trades", TradeFromWire)
}

// Info returns POST /auth/r/info/funding/{key} — e.g. key
// "sym_fUSD".
// Deprecated: use InfoTyped for the documented typed contract.
func (s *Service) Info(ctx context.Context, key string) (*types.Response[[]types.KeyValueInfo], error) {
	if strings.TrimSpace(key) == "" {
		return nil, &types.ValidationError{Field: "key", Message: "funding info key required (e.g. \"sym_fUSD\")"}
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/info/funding/"+url.PathEscape(key), nil, readBucket())
	if err != nil {
		return nil, err
	}
	data, err := wire.AsArr(r.Data, "funding info")
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

func decodeList[T any](r *httpx.Result, what string, dec func([]any) (T, error)) (*types.Response[[]T], error) {
	rows, err := wire.Rows(r.Data, what)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(rows))
	for _, row := range rows {
		v, derr := dec(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, v)
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
