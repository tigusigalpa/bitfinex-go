// Package wallets implements POST /auth/r/wallets — the account's
// wallet balances.
package wallets

import (
	"context"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Wallet is one account wallet row:
// [TYPE, CURRENCY, BALANCE, UNSETTLED_INTEREST, AVAILABLE_BALANCE,
//
//	LAST_CHANGE, LAST_CHANGE_METADATA].
type Wallet struct {
	Type              string        `json:"type"`
	Currency          string        `json:"currency"`
	Balance           types.Decimal `json:"balance"`
	UnsettledInterest types.Decimal `json:"unsettled_interest"`
	AvailableBalance  types.Decimal `json:"available_balance"`
	LastChange        string        `json:"last_change,omitempty"`
	// LastChangeMeta preserves the untyped nested metadata array.
	LastChangeMeta []any `json:"last_change_meta,omitempty"`
}

// FromWire decodes a wallet positional array. Exported so the
// WebSocket account stream ("ws"/"wu" events) can reuse it.
func FromWire(a []any) (Wallet, error) {
	w := Wallet{}
	if err := wire.AssertMinLength(a, 3, "wallet"); err != nil {
		return w, err
	}
	var err error
	if w.Type, err = wire.Str(a, 0); err != nil {
		return w, err
	}
	if w.Currency, err = wire.Str(a, 1); err != nil {
		return w, err
	}
	if w.Balance, err = wire.DecOr(a, 2, types.Decimal{}); err != nil {
		return w, err
	}
	if w.UnsettledInterest, err = wire.DecOr(a, 3, types.Decimal{}); err != nil {
		return w, err
	}
	if w.AvailableBalance, err = wire.DecOr(a, 4, types.Decimal{}); err != nil {
		return w, err
	}
	if w.LastChange, err = wire.Str(a, 5); err != nil {
		return w, err
	}
	if meta, ok, merr := wire.Nested(a, 6); merr != nil {
		return w, merr
	} else if ok {
		w.LastChangeMeta = meta
	}
	return w, nil
}

// Service exposes wallet endpoints.
type Service struct {
	t *httpx.Transport
}

// New binds a Service to a transport.
func New(t *httpx.Transport) *Service { return &Service{t: t} }

// All returns POST /auth/r/wallets — every wallet for the key's
// account.
func (s *Service) All(ctx context.Context) (*types.Response[[]Wallet], error) {
	r, err := s.t.PostAuth(ctx, "v2/auth/r/wallets", nil,
		httpx.RequestContext{Bucket: "wallets", Idempotent: true})
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "wallets")
	if err != nil {
		return nil, err
	}
	out := make([]Wallet, 0, len(rows))
	for _, row := range rows {
		w, derr := FromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, w)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}
