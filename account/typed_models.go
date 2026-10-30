package account

import "github.com/tigusigalpa/bitfinex-go/types"

// BrowserInfo is the documented structured login user-agent metadata.
type BrowserInfo struct {
	OS       string `json:"os"`
	Raw      string `json:"raw"`
	Browser  string `json:"browser"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Mobile   bool   `json:"is_mobile"`
}

// LoginMetadata is the ASN/geolocation/browser object in the login history string.
type LoginMetadata struct {
	ASN       string      `json:"asn"`
	Geo       string      `json:"geo"`
	UserAgent BrowserInfo `json:"user_agent"`
}

// FeeRates describes spot and derivative maker/taker fee coefficients.
type FeeRates struct {
	Crypto      types.Decimal `json:"crypto"`
	Stable      types.Decimal `json:"stable"`
	Fiat        types.Decimal `json:"fiat"`
	Derivatives types.Decimal `json:"derivatives"`
}

// TradingVolume is a currency's 30-day trading volume and optional total breakdown.
type TradingVolume struct {
	Currency string        `json:"curr"`
	Volume   types.Decimal `json:"vol"`
	Safe     types.Decimal `json:"vol_safe"`
	Maker    types.Decimal `json:"vol_maker"`
	BFX      types.Decimal `json:"vol_BFX"`
	BFXSafe  types.Decimal `json:"vol_BFX_safe"`
	BFXMaker types.Decimal `json:"vol_BFX_maker"`
}

// AccountSummary is the documented fee/volume/earnings/LEO summary.
type AccountSummary struct {
	Maker                FeeRates                 `json:"maker"`
	Taker                FeeRates                 `json:"taker"`
	Volumes              []TradingVolume          `json:"volumes"`
	TradingFees          map[string]types.Decimal `json:"trading_fees"`
	TradingFeesTotal     types.Decimal            `json:"trading_fees_total"`
	FundingEarnings      map[string]types.Decimal `json:"funding_earnings"`
	FundingEarningsTotal types.Decimal            `json:"funding_earnings_total"`
	LEOLevel             int64                    `json:"leo_level"`
	LEOAverage           types.Decimal            `json:"leo_average"`
}

// Permission is one API key scope; unknown future scope names are retained.
type Permission struct {
	Scope string `json:"scope"`
	Read  bool   `json:"read"`
	Write bool   `json:"write"`
}

// AuthToken protects generated token material from formatting/serialization.
type AuthToken struct{ value string }

// Value returns the token for explicit application use. Never log it.
func (t AuthToken) Value() string { return t.value }

// String redacts authentication material.
func (t AuthToken) String() string { return "AuthToken{<redacted>}" }

// GoString redacts debug output.
func (t AuthToken) GoString() string { return t.String() }

// MarshalJSON refuses accidental token serialization.
func (t AuthToken) MarshalJSON() ([]byte, error) {
	return nil, &types.ValidationError{Field: "token", Message: "authentication tokens are not serializable"}
}

// TokenRequest creates an API token with explicit capability codes.
type TokenRequest struct {
	TTLSeconds      int
	Capabilities    []string
	WritePermission bool
	ClientIP        string
}

// AuditEntry is a typed account changelog entry. Browser info may be an object or string.
type AuditEntry struct {
	MTS       types.Millis    `json:"mts"`
	Log       string          `json:"log"`
	IP        string          `json:"ip"`
	UserAgent types.JSONValue `json:"user_agent"`
}

// Setting is an API-prefixed key and its self-defined exact JSON value.
type Setting struct {
	Key   string          `json:"key"`
	Value types.JSONValue `json:"value"`
}

// SettingsResult is the server acknowledgement with the changed setting count.
type SettingsResult struct {
	MTS    types.Millis `json:"mts"`
	Type   string       `json:"type"`
	Count  int64        `json:"count"`
	Status string       `json:"status"`
}

// Success reports whether the settings change was acknowledged.
func (s SettingsResult) Success() bool { return s.Status == "SUCCESS" }

// DepositAddress is one address or payment memo and its receiving wallet.
type DepositAddress struct {
	Address string `json:"address"`
	Wallet  string `json:"wallet"`
}

// DepositAddressesRequest selects a method and numbered page (20..100 entries).
type DepositAddressesRequest struct {
	Method   string
	Page     int
	PageSize int
}

// InvoiceQuery selects one of the documented Lightning invoice/payment actions.
type InvoiceQuery struct {
	Action      string
	Offset      int
	PaymentHash string
}

// LightningRecord is one Lightning payment/invoice. Amount is in satoshis, BFXAmount in BTC.
type LightningRecord struct {
	PaymentHash        string        `json:"payment_hash"`
	CreatedAt          types.Millis  `json:"created_at"`
	Expiry             *types.Millis `json:"invoice_expiry"`
	Invoice            string        `json:"invoice"`
	RecipientPublicKey string        `json:"recipient_pub_key"`
	Memo               string        `json:"memo"`
	Amount             types.Decimal `json:"amount"`
	BFXAmount          types.Decimal `json:"bfx_amount"`
	State              string        `json:"state"`
}

// LightningResult holds the selected list or a single record.
type LightningResult struct {
	Invoices []LightningRecord `json:"invoices,omitempty"`
	Payments []LightningRecord `json:"payments,omitempty"`
	Record   *LightningRecord  `json:"record,omitempty"`
}

// MovementDetails adds the documented wire/provider information to a movement.
type MovementDetails struct {
	Movement
	Remark              string          `json:"remark"`
	Memo                string          `json:"memo"`
	Note                string          `json:"note"`
	BankFees            types.Decimal   `json:"bank_fees"`
	BankRouterID        int64           `json:"bank_router_id"`
	ExternalID          string          `json:"external_id"`
	ExternalStatus      string          `json:"external_status"`
	ExternalDescription string          `json:"external_description"`
	BankAccount         types.JSONValue `json:"bank_account"`
}

// FreeTransferAllowance is a Thalex free-transfer quota and optional reset time.
type FreeTransferAllowance struct {
	Available int64         `json:"available"`
	ResetsAt  *types.Millis `json:"resetsAt"`
}

// FreeTransferCount contains independent deposit and withdrawal quotas.
type FreeTransferCount struct {
	Deposits    FreeTransferAllowance `json:"deposits"`
	Withdrawals FreeTransferAllowance `json:"withdrawals"`
}

// TransferResult is the typed payload of an internal wallet transfer notification.
type TransferResult struct {
	MTS        types.Millis  `json:"mts"`
	From       string        `json:"from"`
	To         string        `json:"to"`
	Currency   string        `json:"currency"`
	CurrencyTo string        `json:"currency_to"`
	Amount     types.Decimal `json:"amount"`
}

// DepositAddressResult is the typed deposit-address notification payload.
type DepositAddressResult struct {
	Method      string `json:"method"`
	Currency    string `json:"currency"`
	Address     string `json:"address"`
	PoolAddress string `json:"pool_address"`
}

// GeneratedInvoice is the native invoice result (not a notification envelope).
type GeneratedInvoice struct {
	Hash    string        `json:"hash"`
	Invoice string        `json:"invoice"`
	Amount  types.Decimal `json:"amount"`
}

// WithdrawalResult is the typed withdrawal notification payload.
type WithdrawalResult struct {
	ID        int64         `json:"id"`
	Method    string        `json:"method"`
	PaymentID string        `json:"payment_id"`
	Wallet    string        `json:"wallet"`
	Amount    types.Decimal `json:"amount"`
	Fee       types.Decimal `json:"fee"`
}
