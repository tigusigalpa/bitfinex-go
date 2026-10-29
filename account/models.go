// Package account implements the authenticated account
// endpoints: user info, summary, logins, permissions, token
// generation, audit trail, ledgers, order-availability calc,
// alerts, user settings, and the money-moving endpoints
// (transfer, deposit, invoice, withdrawal, movements).
//
// High-impact transfer/invoice/withdrawal calls require explicit confirmation
// (Confirm: true) so a high-impact write can never be fired
// accidentally. Writes are never automatically retried.
package account

import (
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// UserInfo is the /auth/r/info/user profile row.
type UserInfo struct {
	ID                int64        `json:"id"`
	Email             string       `json:"email"`
	Username          string       `json:"username"`
	MTSCreate         types.Millis `json:"mts_create"`
	Verified          int64        `json:"verified"`
	VerificationLevel int64        `json:"verification_level"`
	Timezone          string       `json:"timezone"`
	Locale            string       `json:"locale"`
	Company           string       `json:"company"`
}

func userInfoFromWire(a []any) (UserInfo, error) {
	u := UserInfo{}
	if err := wire.AssertMinLength(a, 6, "user info"); err != nil {
		return u, err
	}
	var err error
	u.ID = wire.IntOr(a, 0, 0)
	if u.Email, err = wire.Str(a, 1); err != nil {
		return u, err
	}
	if u.Username, err = wire.Str(a, 2); err != nil {
		return u, err
	}
	if mts, _, merr := wire.Mts(a, 3); merr != nil {
		return u, merr
	} else {
		u.MTSCreate = mts
	}
	u.Verified = wire.IntOr(a, 4, 0)
	u.VerificationLevel = wire.IntOr(a, 5, 0)
	if u.Timezone, err = wire.Str(a, 7); err != nil {
		return u, err
	}
	if u.Locale, err = wire.Str(a, 8); err != nil {
		return u, err
	}
	if u.Company, err = wire.Str(a, 9); err != nil {
		return u, err
	}
	return u, nil
}

// LedgerEntry is one /auth/r/ledgers row:
// [ID, CURRENCY, _, MTS, _, AMOUNT, BALANCE, _, DESCRIPTION].
// Documented retention: up to ~6 years.
type LedgerEntry struct {
	ID          int64         `json:"id"`
	Currency    string        `json:"currency"`
	MTS         types.Millis  `json:"mts"`
	Amount      types.Decimal `json:"amount"`
	Balance     types.Decimal `json:"balance"`
	Description string        `json:"description"`
}

func ledgerEntryFromWire(a []any) (LedgerEntry, error) {
	e := LedgerEntry{}
	if err := wire.AssertMinLength(a, 6, "ledger entry"); err != nil {
		return e, err
	}
	var err error
	e.ID = wire.IntOr(a, 0, 0)
	if e.Currency, err = wire.Str(a, 1); err != nil {
		return e, err
	}
	if mts, _, merr := wire.Mts(a, 3); merr != nil {
		return e, merr
	} else {
		e.MTS = mts
	}
	if e.Amount, err = wire.DecOr(a, 5, types.Decimal{}); err != nil {
		return e, err
	}
	if e.Balance, err = wire.DecOr(a, 6, types.Decimal{}); err != nil {
		return e, err
	}
	if e.Description, err = wire.Str(a, 8); err != nil {
		return e, err
	}
	return e, nil
}

// Alert is one price alert row: [KEY, TYPE, SYMBOL, PRICE, COUNT].
type Alert struct {
	Key    string        `json:"key"`
	Type   string        `json:"type"`
	Symbol string        `json:"symbol"`
	Price  types.Decimal `json:"price"`
	Count  int64         `json:"count"`
}

func alertFromWire(a []any) (Alert, error) {
	al := Alert{}
	if err := wire.AssertMinLength(a, 4, "alert"); err != nil {
		return al, err
	}
	var err error
	if al.Key, err = wire.Str(a, 0); err != nil {
		return al, err
	}
	if al.Type, err = wire.Str(a, 1); err != nil {
		return al, err
	}
	if al.Symbol, err = wire.Str(a, 2); err != nil {
		return al, err
	}
	if al.Price, err = wire.DecOr(a, 3, types.Decimal{}); err != nil {
		return al, err
	}
	al.Count = wire.IntOr(a, 4, 0)
	return al, nil
}

// Movement is one deposit/withdrawal history row:
// [ID, CURRENCY, CURRENCY_NAME, _, _, MTS_STARTED, MTS_UPDATED, _,
//
//	_, STATUS, _, _, AMOUNT, FEES, DESTINATION_ADDRESS, _,
//	TRANSACTION_ID].
type Movement struct {
	ID                 int64         `json:"id"`
	Currency           string        `json:"currency"`
	CurrencyName       string        `json:"currency_name"`
	MTSStarted         types.Millis  `json:"mts_started"`
	MTSUpdated         types.Millis  `json:"mts_updated"`
	Status             string        `json:"status"`
	Amount             types.Decimal `json:"amount"`
	Fees               types.Decimal `json:"fees"`
	DestinationAddress string        `json:"destination_address"`
	TransactionID      string        `json:"transaction_id"`
}

func movementFromWire(a []any) (Movement, error) {
	m := Movement{}
	if err := wire.AssertMinLength(a, 14, "movement"); err != nil {
		return m, err
	}
	var err error
	m.ID = wire.IntOr(a, 0, 0)
	if m.Currency, err = wire.Str(a, 1); err != nil {
		return m, err
	}
	if m.CurrencyName, err = wire.Str(a, 2); err != nil {
		return m, err
	}
	if mts, _, merr := wire.Mts(a, 5); merr != nil {
		return m, merr
	} else {
		m.MTSStarted = mts
	}
	if mts, _, merr := wire.Mts(a, 6); merr != nil {
		return m, merr
	} else {
		m.MTSUpdated = mts
	}
	if m.Status, err = wire.Str(a, 9); err != nil {
		return m, err
	}
	if m.Amount, err = wire.DecOr(a, 12, types.Decimal{}); err != nil {
		return m, err
	}
	if m.Fees, err = wire.DecOr(a, 13, types.Decimal{}); err != nil {
		return m, err
	}
	if m.DestinationAddress, err = wire.Str(a, 14); err != nil {
		return m, err
	}
	if m.TransactionID, err = wire.Str(a, 16); err != nil {
		return m, err
	}
	return m, nil
}

// LoginEntry is one /auth/r/logins/hist row. Only documented
// leading fields are typed; the tail is preserved.
type LoginEntry struct {
	ID    int64        `json:"id"`
	MTS   types.Millis `json:"mts"`
	IP    string       `json:"ip"`
	Extra []any        `json:"extra,omitempty"`
}

func loginEntryFromWire(a []any) (LoginEntry, error) {
	e := LoginEntry{}
	if err := wire.AssertMinLength(a, 1, "login entry"); err != nil {
		return e, err
	}
	e.ID = wire.IntOr(a, 0, 0)
	if mts, _, merr := wire.Mts(a, 1); merr != nil {
		return e, merr
	} else {
		e.MTS = mts
	}
	var err error
	if e.IP, err = wire.Str(a, 3); err != nil {
		return e, err
	}
	if len(a) > 4 {
		e.Extra = append([]any(nil), a[4:]...)
	}
	return e, nil
}
