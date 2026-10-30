package account

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strings"
)

// Validate checks a deposit-address page before I/O.
func (r DepositAddressesRequest) Validate() error {
	if strings.TrimSpace(r.Method) == "" || r.Method != strings.TrimSpace(r.Method) {
		return &types.ValidationError{Field: "method", Message: "deposit method required"}
	}
	if r.Page < 0 || int64(r.Page) > 2147483647 || r.PageSize < 0 || (r.PageSize != 0 && (r.PageSize < 20 || r.PageSize > 100)) {
		return &types.ValidationError{Field: "page", Message: "page must be positive and pageSize must be 20..100"}
	}
	return nil
}

// DepositAddressesTyped retrieves a numbered page for an explicit deposit method.
func (s *Service) DepositAddressesTyped(ctx context.Context, r DepositAddressesRequest) (*types.Response[[]DepositAddress], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	if r.Page == 0 {
		r.Page = 1
	}
	if r.PageSize == 0 {
		r.PageSize = 20
	}
	return accountCall(s, ctx, "v2/auth/r/deposit/address/all", map[string]any{"method": r.Method, "page": r.Page, "pageSize": r.PageSize}, false, func(v any) ([]DepositAddress, error) {
		rows, e := wire.Rows(v, "deposit addresses")
		if e != nil {
			return nil, e
		}
		out := make([]DepositAddress, 0, len(rows))
		for _, r := range rows {
			if e = wire.AssertMinLength(r, 2, "deposit address"); e != nil {
				return nil, e
			}
			address, e := wire.RequiredString(r, 0)
			if e != nil {
				return nil, e
			}
			wallet, e := wire.RequiredString(r, 1)
			if e != nil {
				return nil, e
			}
			out = append(out, DepositAddress{Address: address, Wallet: wallet})
		}
		return out, nil
	})
}

// Validate enforces the documented list-vs-ID Lightning query modes.
func (q InvoiceQuery) Validate() error {
	if q.Offset < 0 || int64(q.Offset) > 2147483647 {
		return &types.ValidationError{Field: "offset", Message: "offset must not be negative"}
	}
	switch q.Action {
	case "getInvoicesByUser", "getPaymentsByUser":
		if q.PaymentHash != "" {
			return &types.ValidationError{Field: "txid", Message: "txid is only valid for an ID lookup"}
		}
	case "getInvoiceById", "getPaymentById":
		if q.PaymentHash == "" || q.Offset != 0 {
			return &types.ValidationError{Field: "txid", Message: "ID lookup requires payment hash and no offset"}
		}
	default:
		return &types.ValidationError{Field: "action", Message: "unknown invoice/payment query action"}
	}
	return nil
}

// InvoicePaymentsTyped retrieves the selected Lightning list or ID lookup.
func (s *Service) InvoicePaymentsTyped(ctx context.Context, q InvoiceQuery) (*types.Response[LightningResult], error) {
	if e := q.Validate(); e != nil {
		return nil, e
	}
	query := map[string]any{}
	if q.PaymentHash != "" {
		query["txid"] = q.PaymentHash
	} else {
		query["offset"] = q.Offset
	}
	return accountCall(s, ctx, "v2/auth/r/ext/invoice/payments", map[string]any{"action": q.Action, "query": query}, false, func(v any) (LightningResult, error) {
		var out LightningResult
		m, e := wire.Object(v)
		if e != nil {
			return out, e
		}
		decode := func(v any) (LightningRecord, error) {
			if _, e := wire.Object(v, "payment_hash", "created_at", "amount", "bfx_amount", "state"); e != nil {
				return LightningRecord{}, e
			}
			return wire.Into[LightningRecord](v)
		}
		key := ""
		if q.Action == "getInvoicesByUser" {
			key = "invoices"
		}
		if q.Action == "getPaymentsByUser" {
			key = "payments"
		}
		if key == "" {
			r, e := decode(v)
			if e != nil {
				return out, e
			}
			out.Record = &r
			return out, nil
		}
		list, e := wire.AsArr(m[key], "Lightning records")
		if e != nil {
			return out, e
		}
		rows := make([]LightningRecord, 0, len(list))
		for _, v := range list {
			x, e := decode(v)
			if e != nil {
				return out, e
			}
			rows = append(rows, x)
		}
		if key == "invoices" {
			out.Invoices = rows
		} else {
			out.Payments = rows
		}
		return out, nil
	})
}

// MovementInfoTyped returns the full deposit/withdrawal detail model.
func (s *Service) MovementInfoTyped(ctx context.Context, id int64) (*types.Response[MovementDetails], error) {
	if id <= 0 {
		return nil, &types.ValidationError{Field: "id", Message: "positive movement id required"}
	}
	return accountCall(s, ctx, "v2/auth/r/movements/info", map[string]any{"id": id}, false, func(v any) (MovementDetails, error) {
		var out MovementDetails
		a, e := wire.AsArr(v, "movement info")
		if e != nil {
			return out, e
		}
		out.Movement, e = movementFromWire(a)
		if e != nil {
			return out, e
		}
		for _, x := range []struct {
			p *string
			i int
		}{{&out.Remark, 4}, {&out.Memo, 17}, {&out.Note, 21}, {&out.ExternalID, 28}, {&out.ExternalStatus, 29}, {&out.ExternalDescription, 30}} {
			*x.p, e = wire.Str(a, x.i)
			if e != nil {
				return out, e
			}
		}
		out.BankFees, _, e = wire.Decimal(a, 24)
		if e != nil {
			return out, e
		}
		out.BankRouterID, _, e = wire.Int(a, 25)
		if e != nil {
			return out, e
		}
		out.BankAccount, e = wire.Into[types.JSONValue](wire.At(a, 31))
		return out, e
	})
}

// TransferTyped returns the concrete transfer payload and server notification.
func (s *Service) TransferTyped(ctx context.Context, r TransferRequest) (*types.Response[types.Notification[TransferResult]], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	return accountCall(s, ctx, "v2/auth/w/transfer", r.body(), true, func(v any) (types.Notification[TransferResult], error) {
		return wire.Notification(v, func(v any) (TransferResult, error) {
			var out TransferResult
			a, e := wire.AsArr(v, "transfer")
			if e != nil {
				return out, e
			}
			if e = wire.AssertMinLength(a, 8, "transfer"); e != nil {
				return out, e
			}
			out.MTS, _, e = wire.Mts(a, 0)
			if e != nil {
				return out, e
			}
			for _, x := range []struct {
				p *string
				i int
			}{{&out.From, 1}, {&out.To, 2}, {&out.Currency, 4}, {&out.CurrencyTo, 5}} {
				*x.p, e = wire.Str(a, x.i)
				if e != nil {
					return out, e
				}
			}
			out.Amount, e = wire.RequiredDecimal(a, 7)
			return out, e
		})
	})
}

// DepositAddressTyped returns concrete method/currency/address/memo information.
func (s *Service) DepositAddressTyped(ctx context.Context, r DepositAddressRequest) (*types.Response[types.Notification[DepositAddressResult]], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	renew := 0
	if r.Renew {
		renew = 1
	}
	return accountCall(s, ctx, "v2/auth/w/deposit/address", map[string]any{"wallet": r.Wallet, "method": r.Method, "op_renew": renew}, true, func(v any) (types.Notification[DepositAddressResult], error) {
		return wire.Notification(v, func(v any) (DepositAddressResult, error) {
			var out DepositAddressResult
			a, e := wire.AsArr(v, "deposit address")
			if e != nil {
				return out, e
			}
			if e = wire.AssertMinLength(a, 6, "deposit address"); e != nil {
				return out, e
			}
			for _, x := range []struct {
				p *string
				i int
			}{{&out.Method, 1}, {&out.Currency, 2}, {&out.Address, 4}, {&out.PoolAddress, 5}} {
				*x.p, e = wire.Str(a, x.i)
				if e != nil {
					return out, e
				}
			}
			return out, nil
		})
	})
}

// InvoiceTyped parses the native five-field invoice array. Never retried.
func (s *Service) InvoiceTyped(ctx context.Context, r InvoiceRequest) (*types.Response[GeneratedInvoice], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	return accountCall(s, ctx, "v2/auth/w/deposit/invoice", map[string]any{"wallet": r.Wallet, "currency": r.Currency, "amount": r.Amount.CanonicalString()}, true, func(v any) (GeneratedInvoice, error) {
		var out GeneratedInvoice
		a, e := wire.AsArr(v, "invoice")
		if e != nil {
			return out, e
		}
		if e = wire.AssertMinLength(a, 5, "invoice"); e != nil {
			return out, e
		}
		out.Hash, e = wire.RequiredString(a, 0)
		if e != nil {
			return out, e
		}
		out.Invoice, e = wire.RequiredString(a, 1)
		if e != nil {
			return out, e
		}
		out.Amount, e = wire.RequiredDecimal(a, 4)
		return out, e
	})
}

// WithdrawTyped returns the native typed withdrawal acknowledgement. Never retried.
func (s *Service) WithdrawTyped(ctx context.Context, r WithdrawRequest) (*types.Response[types.Notification[WithdrawalResult]], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	body := map[string]any{"method": r.Method, "wallet": r.Wallet}
	if strings.EqualFold(r.Method, "lnx") {
		body["invoice"] = r.Invoice
	} else {
		body["address"] = r.Address
		body["amount"] = r.Amount.CanonicalString()
	}
	if r.PaymentID != "" {
		body["payment_id"] = r.PaymentID
	}
	return accountCall(s, ctx, "v2/auth/w/withdraw", body, true, func(v any) (types.Notification[WithdrawalResult], error) {
		return wire.Notification(v, func(v any) (WithdrawalResult, error) {
			var out WithdrawalResult
			a, e := wire.AsArr(v, "withdrawal")
			if e != nil {
				return out, e
			}
			if e = wire.AssertMinLength(a, 9, "withdrawal"); e != nil {
				return out, e
			}
			out.ID, _, e = wire.Int(a, 0)
			if e != nil {
				return out, e
			}
			for _, x := range []struct {
				p *string
				i int
			}{{&out.Method, 2}, {&out.PaymentID, 3}, {&out.Wallet, 4}} {
				*x.p, e = wire.Str(a, x.i)
				if e != nil {
					return out, e
				}
			}
			out.Amount, _, e = wire.Decimal(a, 5)
			if e != nil {
				return out, e
			}
			out.Fee, _, e = wire.Decimal(a, 8)
			return out, e
		})
	})
}
