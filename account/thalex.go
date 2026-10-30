package account

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"regexp"
)

// U2FClientResponse uses the Thalex endpoint's documented assertion field names.
type U2FClientResponse struct {
	ClientDataJSON    string  `json:"clientDataJSON"`
	AttestationObject string  `json:"attestationObject"`
	Signature         string  `json:"signature"`
	UserHandle        *string `json:"userHandle,omitempty"`
}

// U2FAssertion is the base64url WebAuthn response passed to the exchange.
type U2FAssertion struct {
	ID       string            `json:"id,omitempty"`
	RawID    string            `json:"rawId,omitempty"`
	Response U2FClientResponse `json:"response"`
}

// TwoFactorToken selects exactly one documented OTP/U2F authentication mode.
type TwoFactorToken struct {
	Method   string        `json:"method"`
	Token    string        `json:"token,omitempty"`
	Response *U2FAssertion `json:"response,omitempty"`
}

// String redacts second-factor credentials.
func (t TwoFactorToken) String() string { return "TwoFactorToken{<redacted>}" }

// GoString redacts debug output.
func (t TwoFactorToken) GoString() string { return t.String() }

var otpRe = regexp.MustCompile(`^[0-9]{6}$`)

// Validate enforces mutually exclusive factor modes without exposing their data.
func (t TwoFactorToken) Validate() error {
	valid := false
	switch t.Method {
	case "otp":
		valid = otpRe.MatchString(t.Token) && t.Response == nil
	case "u2f":
		if t.Response != nil {
			r := t.Response
			valid = t.Token == "" && (r.ID != "" || r.RawID != "") && r.Response.ClientDataJSON != "" && r.Response.AttestationObject != "" && r.Response.Signature != ""
		}
	}
	if !valid {
		return &types.ValidationError{Field: "tfaToken", Message: "one complete OTP or U2F factor is required"}
	}
	return nil
}

// ThalexTransferRequest is a high-impact transfer between Bitfinex and Thalex.
type ThalexTransferRequest struct {
	Amount   types.Decimal
	Currency string
	TFA      TwoFactorToken
	Confirm  bool
}

// String redacts transfer and second-factor details.
func (r ThalexTransferRequest) String() string { return "ThalexTransferRequest{<redacted>}" }

// GoString redacts debug output.
func (r ThalexTransferRequest) GoString() string { return r.String() }

// Validate requires explicit confirmation, supported currency and a factor.
func (r ThalexTransferRequest) Validate() error {
	if !r.Confirm {
		return &types.ValidationError{Field: "confirm", Message: "Thalex transfer requires Confirm: true"}
	}
	if !r.Amount.IsValid() || r.Amount.Sign() <= 0 {
		return &types.ValidationError{Field: "amount", Message: "positive transfer amount required"}
	}
	switch r.Currency {
	case "USE", "ETH", "BTC":
	default:
		return &types.ValidationError{Field: "ccy", Message: "Thalex currency must be USE, ETH or BTC"}
	}
	return r.TFA.Validate()
}

// ThalexTransfer is the typed transfer object; unknown future statuses/types remain strings.
type ThalexTransfer struct {
	Type        string        `json:"type"`
	Destination string        `json:"addressDest"`
	Amount      types.Decimal `json:"amount"`
	Currency    string        `json:"ccy"`
	CreatedAt   types.Millis  `json:"createdAt"`
	UpdatedAt   types.Millis  `json:"updatedAt"`
	ID          string        `json:"id"`
	Status      string        `json:"status"`
	Fee         types.Decimal `json:"fee"`
}

func (s *Service) thalexTransfer(ctx context.Context, path string, r ThalexTransferRequest) (*types.Response[ThalexTransfer], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	return accountCall(s, ctx, path, map[string]any{"provider": "thalex", "amount": r.Amount.CanonicalString(), "ccy": r.Currency, "tfaToken": r.TFA}, true, func(v any) (ThalexTransfer, error) {
		if _, e := wire.Object(v, "type", "addressDest", "amount", "ccy", "createdAt", "updatedAt", "id", "status", "fee"); e != nil {
			return ThalexTransfer{}, e
		}
		out, e := wire.Into[ThalexTransfer](v)
		if e != nil {
			return out, e
		}
		if out.ID == "" || !out.Amount.IsValid() || !out.Fee.IsValid() {
			return out, &types.ProtocolError{Message: "invalid Thalex transfer response"}
		}
		return out, nil
	})
}

// ThalexDeposit requests a deposit to Thalex. Never automatically retried.
func (s *Service) ThalexDeposit(ctx context.Context, r ThalexTransferRequest) (*types.Response[ThalexTransfer], error) {
	return s.thalexTransfer(ctx, "v2/auth/w/ext/wallets/deposits/request", r)
}

// ThalexWithdraw requests a withdrawal from Thalex. Never automatically retried.
func (s *Service) ThalexWithdraw(ctx context.Context, r ThalexTransferRequest) (*types.Response[ThalexTransfer], error) {
	return s.thalexTransfer(ctx, "v2/auth/w/ext/wallets/withdrawals/request", r)
}

// ThalexFreeTransferCountTyped retrieves the documented provider quota object.
func (s *Service) ThalexFreeTransferCountTyped(ctx context.Context) (*types.Response[FreeTransferCount], error) {
	return accountCall(s, ctx, "v2/auth/r/ext/wallets/transfers/free/count", map[string]any{"Provider": "thalex"}, false, func(v any) (FreeTransferCount, error) {
		m, e := wire.Object(v, "deposits", "withdrawals")
		if e != nil {
			return FreeTransferCount{}, e
		}
		for _, k := range []string{"deposits", "withdrawals"} {
			if _, e = wire.Object(m[k], "available"); e != nil {
				return FreeTransferCount{}, e
			}
		}
		out, e := wire.Into[FreeTransferCount](v)
		if e == nil && (out.Deposits.Available < 0 || out.Withdrawals.Available < 0) {
			e = &types.ProtocolError{Message: "negative free transfer allowance"}
		}
		return out, e
	})
}
