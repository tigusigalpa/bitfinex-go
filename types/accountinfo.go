package types

// BalanceInfo is the authenticated bu stream's total and net assets.
type BalanceInfo struct {
	Assets    Decimal `json:"assets"`
	NetAssets Decimal `json:"net_assets"`
}

// MarginBase is a base margin calculation; absent Decimal values mean pending calc.
type MarginBase struct {
	PL       Decimal `json:"pl"`
	Swaps    Decimal `json:"swaps"`
	Balance  Decimal `json:"balance"`
	Net      Decimal `json:"net"`
	Required Decimal `json:"required"`
}

// MarginSymbol is a per-market margin calculation.
type MarginSymbol struct {
	TradableBalance Decimal `json:"tradable_balance"`
	GrossBalance    Decimal `json:"gross_balance"`
	Buy             Decimal `json:"buy"`
	Sell            Decimal `json:"sell"`
}

// MarginInfo is a typed base/symbol union; exactly one calculation is populated.
type MarginInfo struct {
	Type   string        `json:"type"`
	Symbol Symbol        `json:"symbol,omitempty"`
	Base   *MarginBase   `json:"base,omitempty"`
	Market *MarginSymbol `json:"market,omitempty"`
}

// FundingInfo is the yield/duration calculation for a funding currency.
type FundingInfo struct {
	Type         string  `json:"type"`
	Symbol       Symbol  `json:"symbol"`
	YieldLoan    Decimal `json:"yield_loan"`
	YieldLend    Decimal `json:"yield_lend"`
	DurationLoan Decimal `json:"duration_loan"`
	DurationLend Decimal `json:"duration_lend"`
}

// Notification is the typed v2 write envelope. Check Success as well as the Go error.
type Notification[T any] struct {
	MTS       Millis `json:"mts"`
	Type      string `json:"type"`
	MessageID int64  `json:"message_id"`
	Data      T      `json:"data"`
	Code      int64  `json:"code"`
	Status    string `json:"status"`
	Text      string `json:"text"`
}

// Success reports the server's SUCCESS acknowledgement.
func (n Notification[T]) Success() bool { return n.Status == "SUCCESS" }

// OperationStatus represents endpoints returning a boolean/0-or-1 status array.
type OperationStatus struct {
	Success bool `json:"success"`
}
