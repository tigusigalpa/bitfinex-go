package account

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// DepositAddressesIterator traverses numbered address pages for one method.
func (s *Service) DepositAddressesIterator(method string, pageSize int) (*types.Iterator[DepositAddress], error) {
	r := DepositAddressesRequest{Method: method, Page: 1, PageSize: pageSize}
	if e := r.Validate(); e != nil {
		return nil, e
	}
	if r.PageSize == 0 {
		r.PageSize = 20
	}
	seen := map[DepositAddress]struct{}{}
	finished := false
	return types.NewIterator(func(ctx context.Context) ([]DepositAddress, error) {
		if finished {
			return nil, nil
		}
		resp, e := s.DepositAddressesTyped(ctx, r)
		if e != nil {
			return nil, e
		}
		if len(resp.Data) > r.PageSize {
			return nil, &types.ProtocolError{Message: "deposit address page exceeds requested page size"}
		}
		finished = len(resp.Data) < r.PageSize
		out := make([]DepositAddress, 0, len(resp.Data))
		for _, v := range resp.Data {
			if _, dup := seen[v]; !dup {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
		if !finished && len(out) == 0 {
			return nil, &types.ProtocolError{Message: "deposit address pagination made no progress"}
		}
		if r.Page == 2147483647 && !finished {
			return nil, &types.ProtocolError{Message: "deposit address page cursor exhausted"}
		}
		r.Page++
		return out, nil
	}), nil
}

// InvoicePaymentsIterator traverses the documented offset-based invoice/payment lists.
// ID actions use InvoicePaymentsTyped directly. Replayed non-empty pages fail explicitly.
func (s *Service) InvoicePaymentsIterator(action string) (*types.Iterator[LightningRecord], error) {
	if action != "getInvoicesByUser" && action != "getPaymentsByUser" {
		return nil, &types.ValidationError{Field: "action", Message: "iterator requires an invoice/payment list action"}
	}
	q := InvoiceQuery{Action: action}
	seen := map[string]struct{}{}
	return types.NewIterator(func(ctx context.Context) ([]LightningRecord, error) {
		r, e := s.InvoicePaymentsTyped(ctx, q)
		if e != nil {
			return nil, e
		}
		rows := r.Data.Payments
		if action == "getInvoicesByUser" {
			rows = r.Data.Invoices
		}
		if len(rows) == 0 {
			return nil, nil
		}
		out := make([]LightningRecord, 0, len(rows))
		for _, v := range rows {
			if v.PaymentHash == "" {
				return nil, &types.ProtocolError{Message: "Lightning record has no payment hash"}
			}
			if _, dup := seen[v.PaymentHash]; !dup {
				seen[v.PaymentHash] = struct{}{}
				out = append(out, v)
			}
		}
		if len(out) == 0 {
			return nil, &types.ProtocolError{Message: "Lightning pagination made no progress"}
		}
		if int64(q.Offset)+int64(len(rows)) > 2147483647 {
			return nil, &types.ProtocolError{Message: "Lightning offset exhausted"}
		}
		q.Offset += len(rows)
		return out, nil
	}), nil
}
