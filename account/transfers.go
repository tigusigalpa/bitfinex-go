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

// TransferRequest is POST /auth/w/transfer — an internal
// transfer between wallets. Confirm must be true.
type TransferRequest struct {
	From       string        // source wallet type, e.g. "exchange"
	To         string        // destination wallet type, e.g. "margin"
	Currency   string        // source currency, e.g. "USD"
	CurrencyTo string        // destination currency (defaults to Currency)
	Amount     types.Decimal // required, > 0
	Confirm    bool          // MUST be true — explicit opt-in
}

// Validate enforces the local contract, including the explicit
// confirmation flag required for high-impact writes.
func (r TransferRequest) Validate() error {
	if !validWallet(r.From) || !validWallet(r.To) {
		return &types.ValidationError{Field: "wallet", Message: "unsupported wallet type"}
	}
	if !r.Confirm {
		return &types.ValidationError{Field: "confirm", Message: "TransferRequest requires Confirm: true — internal transfers are high-impact writes and must be explicit"}
	}
	if strings.TrimSpace(r.From) == "" || strings.TrimSpace(r.To) == "" {
		return &types.ValidationError{Field: "wallet", Message: "from and to wallets are required"}
	}
	if strings.TrimSpace(r.Currency) == "" {
		return &types.ValidationError{Field: "currency", Message: "currency required"}
	}
	if !r.Amount.IsValid() || r.Amount.Sign() <= 0 {
		return &types.ValidationError{Field: "amount", Message: "transfer amount must be positive"}
	}
	return nil
}

func (r TransferRequest) body() map[string]any {
	to := r.CurrencyTo
	if to == "" {
		to = r.Currency
	}
	return map[string]any{
		"from":        r.From,
		"to":          r.To,
		"currency":    r.Currency,
		"currency_to": to,
		"amount":      r.Amount.CanonicalString(),
	}
}

// Transfer sends POST /auth/w/transfer. Never retried.
func (s *Service) Transfer(ctx context.Context, r TransferRequest) (*types.Response[orders.Notification], error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return s.notificationCall(ctx, "v2/auth/w/transfer", r.body(), "transfer")
}

// DepositAddressRequest is POST /auth/w/deposit/address —
// generates (or renews) a deposit address. Method is the
// transport method name (e.g. "bitcoin", "tetheruse"), wallet the
// destination wallet type.
type DepositAddressRequest struct {
	Method string // required
	Wallet string // required
	Renew  bool   // op_renew=1 forces a fresh address
}

// Validate enforces the local contract.
func (r DepositAddressRequest) Validate() error {
	if strings.TrimSpace(r.Method) == "" {
		return &types.ValidationError{Field: "method", Message: "deposit method required (e.g. \"bitcoin\")"}
	}
	if strings.TrimSpace(r.Wallet) == "" {
		return &types.ValidationError{Field: "wallet", Message: "destination wallet required (e.g. \"exchange\")"}
	}
	return nil
}

// DepositAddress sends POST /auth/w/deposit/address.
// Never retried.
func (s *Service) DepositAddress(ctx context.Context, r DepositAddressRequest) (*types.Response[orders.Notification], error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	renew := 0
	if r.Renew {
		renew = 1
	}
	return s.notificationCall(ctx, "v2/auth/w/deposit/address", map[string]any{
		"method":   r.Method,
		"wallet":   r.Wallet,
		"op_renew": renew,
	}, "transfer")
}

// DepositAddresses returns POST /auth/r/deposit/address/all —
// last used addresses per method (shape varies; untyped).
func (s *Service) DepositAddresses(ctx context.Context) (*types.Response[any], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/deposit/address/all", nil, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// InvoiceRequest is POST /auth/w/deposit/invoice — generates a
// Lightning (LNX) deposit invoice. Confirm must be true.
type InvoiceRequest struct {
	Currency string        // e.g. "LNX" — required
	Wallet   string        // destination wallet — required
	Amount   types.Decimal // required, > 0
	Confirm  bool          // MUST be true
}

// Validate enforces the local contract.
func (r InvoiceRequest) Validate() error {
	if !r.Confirm {
		return &types.ValidationError{Field: "confirm", Message: "InvoiceRequest requires Confirm: true — invoice generation is a write"}
	}
	if strings.TrimSpace(r.Currency) == "" || strings.TrimSpace(r.Wallet) == "" {
		return &types.ValidationError{Field: "currency", Message: "currency and wallet required"}
	}
	if !r.Amount.IsValid() || r.Amount.Sign() <= 0 {
		return &types.ValidationError{Field: "amount", Message: "invoice amount must be positive"}
	}
	return nil
}

// Invoice sends POST /auth/w/deposit/invoice. Never retried.
func (s *Service) Invoice(ctx context.Context, r InvoiceRequest) (*types.Response[orders.Notification], error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return s.notificationCall(ctx, "v2/auth/w/deposit/invoice", map[string]any{
		"currency": r.Currency,
		"wallet":   r.Wallet,
		"amount":   r.Amount.CanonicalString(),
	}, "transfer")
}

// InvoicePayments returns POST /auth/r/ext/invoice/payments —
// completed LNX invoice payments (shape varies; untyped).
func (s *Service) InvoicePayments(ctx context.Context, limit int) (*types.Response[any], error) {
	body := map[string]any{}
	if limit > 0 {
		body["limit"] = limit
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/ext/invoice/payments", body, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// WithdrawRequest is POST /auth/w/withdraw — the highest-impact
// write in the SDK. Confirm must be true AND a non-empty
// destination address is required.
type WithdrawRequest struct {
	Method    string        // withdrawal method (e.g. "bitcoin") — required
	Wallet    string        // source wallet — required
	Address   string        // destination address — required
	Amount    types.Decimal // required, > 0
	Invoice   string        // optional LNX invoice instead of address
	PaymentID string        // optional payment id / memo / tag
	Confirm   bool          // MUST be true
}

// Validate enforces the local contract.
func (r WithdrawRequest) Validate() error {
	if !validWallet(r.Wallet) {
		return &types.ValidationError{Field: "wallet", Message: "unsupported wallet type"}
	}
	if !r.Confirm {
		return &types.ValidationError{Field: "confirm", Message: "WithdrawRequest requires Confirm: true — withdrawals are irreversible and must be explicit"}
	}
	if strings.TrimSpace(r.Method) == "" {
		return &types.ValidationError{Field: "method", Message: "withdrawal method required"}
	}
	if strings.TrimSpace(r.Wallet) == "" {
		return &types.ValidationError{Field: "wallet", Message: "source wallet required"}
	}
	if strings.TrimSpace(r.Address) == "" && strings.TrimSpace(r.Invoice) == "" {
		return &types.ValidationError{Field: "address", Message: "a destination address or invoice is required"}
	}
	if strings.EqualFold(r.Method, "lnx") {
		if r.Invoice == "" || r.Address != "" {
			return &types.ValidationError{Field: "invoice", Message: "lnx requires invoice and no address"}
		}
		return nil
	}
	if r.Invoice != "" {
		return &types.ValidationError{Field: "invoice", Message: "invoice is only supported for lnx withdrawals"}
	}
	if !r.Amount.IsValid() || r.Amount.Sign() <= 0 {
		return &types.ValidationError{Field: "amount", Message: "withdrawal amount must be positive"}
	}
	return nil
}

// Withdraw sends POST /auth/w/withdraw. Never retried — if the
// outcome is ambiguous, reconcile via Movements before any
// resubmission.
func (s *Service) Withdraw(ctx context.Context, r WithdrawRequest) (*types.Response[orders.Notification], error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	body := map[string]any{
		"method":  r.Method,
		"wallet":  r.Wallet,
		"address": r.Address,
		"amount":  r.Amount.CanonicalString(),
	}
	if strings.EqualFold(r.Method, "lnx") {
		delete(body, "address")
		delete(body, "amount")
	}
	if r.Invoice != "" {
		body["invoice"] = r.Invoice
	}
	if r.PaymentID != "" {
		body["payment_id"] = r.PaymentID
	}
	return s.notificationCall(ctx, "v2/auth/w/withdraw", body, "withdraw")
}

func validWallet(s string) bool {
	switch s {
	case "exchange", "margin", "funding", "trading", "deposit":
		return true
	}
	return false
}

// Movements returns POST /auth/r/movements[/{currency}]/hist —
// deposits + withdrawals, optionally filtered by currency.
func (s *Service) Movements(ctx context.Context, currency string, q *types.HistoryQuery) (*types.Response[[]Movement], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	path := "v2/auth/r/movements"
	if currency != "" {
		path += "/" + url.PathEscape(strings.ToUpper(strings.TrimSpace(currency)))
	}
	path += "/hist"
	r, err := s.t.PostAuth(ctx, path, q.Body(), readBucket())
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "movements")
	if err != nil {
		return nil, err
	}
	out := make([]Movement, 0, len(rows))
	for _, row := range rows {
		m, derr := movementFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, m)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// MovementInfo returns POST /auth/r/movements/info — status
// detail for one movement id (shape varies; untyped).
func (s *Service) MovementInfo(ctx context.Context, id int64) (*types.Response[any], error) {
	if id <= 0 {
		return nil, &types.ValidationError{Field: "id", Message: "movement id must be positive"}
	}
	r, err := s.t.PostAuth(ctx, "v2/auth/r/movements/info", map[string]any{"id": id}, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// ThalexFreeTransferCount returns
// POST /auth/r/ext/wallets/transfers/free/count — the remaining
// free Thalex transfer count.
func (s *Service) ThalexFreeTransferCount(ctx context.Context) (*types.Response[any], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/ext/wallets/transfers/free/count", nil, readBucket())
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

func (s *Service) notificationCall(ctx context.Context, apiPath string, body any, bucket string) (*types.Response[orders.Notification], error) {
	r, err := s.t.PostAuth(ctx, apiPath, body, httpx.RequestContext{Bucket: bucket, Idempotent: false})
	if err != nil {
		return nil, err
	}
	n, err := decodeNotification(r)
	if err != nil {
		return nil, err
	}
	return types.NewResponse(*n, httpx.Meta(r)), nil
}
