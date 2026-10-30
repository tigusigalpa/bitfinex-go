package market

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strings"
)

// ConfigKey identifies one documented public configuration; multi-requests retain order.
type ConfigKey string

const (
	ConfigExchangePairs      ConfigKey = "pub:list:pair:exchange"
	ConfigMarginPairs        ConfigKey = "pub:list:pair:margin"
	ConfigFuturesPairs       ConfigKey = "pub:list:pair:futures"
	ConfigSecuritiesPairs    ConfigKey = "pub:list:pair:securities"
	ConfigCurrencies         ConfigKey = "pub:list:currency"
	ConfigMarginCurrencies   ConfigKey = "pub:list:currency:margin"
	ConfigCompetitions       ConfigKey = "pub:list:competitions"
	ConfigCurrencySymbols    ConfigKey = "pub:map:currency:sym"
	ConfigCurrencyLabels     ConfigKey = "pub:map:currency:label"
	ConfigCurrencyUnits      ConfigKey = "pub:map:currency:unit"
	ConfigCurrencyUnderlying ConfigKey = "pub:map:currency:undl"
	ConfigCurrencyPools      ConfigKey = "pub:map:currency:pool"
	ConfigCurrencyExplorers  ConfigKey = "pub:map:currency:explorer"
	ConfigCurrencyFees       ConfigKey = "pub:map:currency:tx:fee"
	ConfigTransactionMethods ConfigKey = "pub:map:tx:method"
	ConfigPairInfo           ConfigKey = "pub:info:pair"
	ConfigFuturesPairInfo    ConfigKey = "pub:info:pair:futures"
	ConfigWalletStatus       ConfigKey = "pub:info:tx:status"
	ConfigDerivativeFees     ConfigKey = "pub:fees"
	ConfigMarginRisk         ConfigKey = "pub:spec:margin"
)

// CurrencyMapping maps a currency to its documented symbol/label/unit/underlying/pool.
type CurrencyMapping struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

// CurrencyExplorer contains documented explorer/address/transaction URL templates.
type CurrencyExplorer struct {
	Currency    string `json:"currency"`
	Explorer    string `json:"explorer"`
	Address     string `json:"address"`
	Transaction string `json:"transaction"`
}

// TransactionMethod maps a deposit/withdrawal method to its supported currencies.
type TransactionMethod struct {
	Method     string   `json:"method"`
	Currencies []string `json:"currencies"`
}

// CurrencyTransactionFees retains the documented exact fee schedule order.
type CurrencyTransactionFees struct {
	Currency string          `json:"currency"`
	Fees     []types.Decimal `json:"fees"`
}

// PairInfo contains minimum/maximum order size and optional margin coefficients.
type PairInfo struct {
	Pair          string        `json:"pair"`
	FirstTrade    *types.Millis `json:"first_trade,omitempty"`
	MinimumOrder  types.Decimal `json:"minimum_order"`
	MaximumOrder  types.Decimal `json:"maximum_order"`
	InitialMargin types.Decimal `json:"initial_margin"`
	MinimumMargin types.Decimal `json:"minimum_margin"`
}

// WalletStatus describes operational flags and deposit confirmation requirements.
type WalletStatus struct {
	Method               string `json:"method"`
	Deposits             bool   `json:"deposits"`
	Withdrawals          bool   `json:"withdrawals"`
	DepositPaymentID     bool   `json:"deposit_payment_id"`
	WithdrawalPaymentID  bool   `json:"withdrawal_payment_id"`
	DepositConfirmations *int64 `json:"deposit_confirmations,omitempty"`
}

// PerpetualFees contains the documented derivative funding correction coefficients.
type PerpetualFees struct {
	ClampMinimum   types.Decimal `json:"correction_clamp_min"`
	InsuranceClamp types.Decimal `json:"correction_clamp_insurance"`
	Slope          types.Decimal `json:"correction_slope"`
}

// DerivativeFees contains perpetual derivative fee configuration.
type DerivativeFees struct {
	Perpetual PerpetualFees `json:"deriv_perp"`
}

// MarginRisk contains named exact haircut/risk coefficients. Keys are server-defined.
type MarginRisk struct {
	Configuration map[string]types.Decimal `json:"conf"`
	Initial       map[string]types.Decimal `json:"initial"`
	Minimum       map[string]types.Decimal `json:"minimum"`
}

// Configuration is a typed union selected by Key; only the selected payload is populated.
type Configuration struct {
	Key            ConfigKey                 `json:"key"`
	Symbols        []string                  `json:"symbols,omitempty"`
	Competitions   []string                  `json:"competitions,omitempty"`
	Mappings       []CurrencyMapping         `json:"mappings,omitempty"`
	Explorers      []CurrencyExplorer        `json:"explorers,omitempty"`
	Methods        []TransactionMethod       `json:"methods,omitempty"`
	Fees           []CurrencyTransactionFees `json:"fees,omitempty"`
	Pairs          []PairInfo                `json:"pairs,omitempty"`
	Wallets        []WalletStatus            `json:"wallets,omitempty"`
	DerivativeFees *DerivativeFees           `json:"derivative_fees,omitempty"`
	MarginRisk     *MarginRisk               `json:"margin_risk,omitempty"`
}

func configKind(key ConfigKey) string {
	switch key {
	case ConfigExchangePairs, ConfigMarginPairs, ConfigFuturesPairs, ConfigSecuritiesPairs, ConfigCurrencies, ConfigMarginCurrencies:
		return "symbols"
	case ConfigCompetitions:
		return "competitions"
	case ConfigCurrencySymbols, ConfigCurrencyLabels, ConfigCurrencyUnits, ConfigCurrencyUnderlying, ConfigCurrencyPools:
		return "mapping"
	case ConfigCurrencyExplorers:
		return "explorers"
	case ConfigTransactionMethods:
		return "methods"
	case ConfigCurrencyFees:
		return "fees"
	case ConfigPairInfo, ConfigFuturesPairInfo:
		return "pairs"
	case ConfigWalletStatus:
		return "wallets"
	case ConfigDerivativeFees:
		return "derivative-fees"
	case ConfigMarginRisk:
		return "margin-risk"
	}
	return ""
}

// ConfigsTyped supports every documented configuration variant, including ordered batches.
func (s *Service) ConfigsTyped(ctx context.Context, keys ...ConfigKey) (*types.Response[[]Configuration], error) {
	if len(keys) == 0 {
		return nil, &types.ValidationError{Field: "configs", Message: "at least one configuration key required"}
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		if configKind(k) == "" {
			return nil, &types.ValidationError{Field: "config", Message: "unsupported configuration key"}
		}
		parts[i] = string(k)
	}
	r, e := s.t.GetPublic(ctx, "conf/"+strings.Join(parts, ","), nil, readBucket("conf"))
	if e != nil {
		return nil, e
	}
	values, e := wire.AsArr(r.Data, "configurations")
	if e != nil {
		return nil, e
	}
	if len(values) != len(keys) {
		return nil, &types.ProtocolError{Message: "configuration batch result count mismatch"}
	}
	out := make([]Configuration, len(keys))
	for i, k := range keys {
		out[i], e = decodeConfiguration(k, values[i])
		if e != nil {
			return nil, e
		}
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}
func configStrings(v any) ([]string, error) {
	a, e := wire.AsArr(v, "configuration strings")
	if e != nil {
		return nil, e
	}
	out := make([]string, len(a))
	for i := range a {
		out[i], e = wire.Str(a, i)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func decodeConfiguration(key ConfigKey, v any) (Configuration, error) {
	out := Configuration{Key: key}
	kind := configKind(key)
	if kind == "symbols" || kind == "competitions" {
		a, e := configStrings(v)
		if kind == "symbols" {
			out.Symbols = a
		} else {
			out.Competitions = a
		}
		return out, e
	}
	if kind == "derivative-fees" {
		m, e := wire.Object(v, "deriv_perp")
		if e != nil {
			return out, e
		}
		if _, e = wire.Object(m["deriv_perp"], "correction_clamp_min", "correction_clamp_insurance", "correction_slope"); e != nil {
			return out, e
		}
		x, e := wire.Into[DerivativeFees](v)
		out.DerivativeFees = &x
		return out, e
	}
	if kind == "margin-risk" {
		m, e := wire.Object(v, "conf", "initial", "minimum")
		if e != nil {
			return out, e
		}
		x := &MarginRisk{}
		x.Configuration, e = wire.DecimalMap(m["conf"])
		if e != nil {
			return out, e
		}
		x.Initial, e = wire.DecimalMap(m["initial"])
		if e != nil {
			return out, e
		}
		x.Minimum, e = wire.DecimalMap(m["minimum"])
		out.MarginRisk = x
		return out, e
	}
	rows, e := wire.Rows(v, "configuration rows")
	if e != nil {
		return out, e
	}
	for _, r := range rows {
		if e = wire.AssertMinLength(r, 2, "configuration row"); e != nil {
			return out, e
		}
		name, e := wire.RequiredString(r, 0)
		if e != nil {
			return out, e
		}
		switch kind {
		case "mapping":
			value, e := wire.Str(r, 1)
			if e != nil {
				return out, e
			}
			out.Mappings = append(out.Mappings, CurrencyMapping{Currency: name, Value: value})
		case "explorers":
			a, e := wire.AsArr(r[1], "explorer")
			if e != nil {
				return out, e
			}
			if e = wire.AssertMinLength(a, 3, "explorer"); e != nil {
				return out, e
			}
			x := CurrencyExplorer{Currency: name}
			for i, p := range []*string{&x.Explorer, &x.Address, &x.Transaction} {
				*p, e = wire.Str(a, i)
				if e != nil {
					return out, e
				}
			}
			out.Explorers = append(out.Explorers, x)
		case "methods":
			currencies, e := configStrings(r[1])
			if e != nil {
				return out, e
			}
			out.Methods = append(out.Methods, TransactionMethod{Method: name, Currencies: currencies})
		case "fees":
			a, e := wire.AsArr(r[1], "currency transaction fees")
			if e != nil {
				return out, e
			}
			x := CurrencyTransactionFees{Currency: name, Fees: make([]types.Decimal, len(a))}
			for i := range a {
				x.Fees[i], e = wire.RequiredDecimal(a, i)
				if e != nil {
					return out, e
				}
			}
			out.Fees = append(out.Fees, x)
		case "pairs":
			a, e := wire.AsArr(r[1], "pair info")
			if e != nil {
				return out, e
			}
			if e = wire.AssertMinLength(a, 10, "pair info"); e != nil {
				return out, e
			}
			x := PairInfo{Pair: name}
			if mts, ok, e := wire.Mts(a, 0); e != nil {
				return out, e
			} else if ok {
				x.FirstTrade = &mts
			}
			for _, t := range []struct {
				p *types.Decimal
				i int
			}{{&x.MinimumOrder, 3}, {&x.MaximumOrder, 4}, {&x.InitialMargin, 8}, {&x.MinimumMargin, 9}} {
				*t.p, _, e = wire.Decimal(a, t.i)
				if e != nil {
					return out, e
				}
			}
			out.Pairs = append(out.Pairs, x)
		case "wallets":
			if e = wire.AssertMinLength(r, 9, "wallet status"); e != nil {
				return out, e
			}
			x := WalletStatus{Method: name}
			for _, t := range []struct {
				p *bool
				i int
			}{{&x.Deposits, 1}, {&x.Withdrawals, 2}, {&x.DepositPaymentID, 7}, {&x.WithdrawalPaymentID, 8}} {
				status, e := wire.Status([]any{r[t.i]})
				if e != nil {
					return out, e
				}
				*t.p = status.Success
			}
			if n, ok, e := wire.Int(r, 11); e != nil {
				return out, e
			} else if ok {
				x.DepositConfirmations = &n
			}
			out.Wallets = append(out.Wallets, x)
		}
	}
	return out, nil
}

// TradingPairs retrieves typed symbols for exchange/margin/futures/securities.
func (s *Service) TradingPairs(ctx context.Context, market string) (*types.Response[[]string], error) {
	r, e := s.ConfigsTyped(ctx, ConfigKey("pub:list:pair:"+market))
	if e != nil {
		return nil, e
	}
	return types.NewResponse(r.Data[0].Symbols, r.Meta), nil
}

// Currencies retrieves all platform currencies or margin-collateral currencies.
func (s *Service) Currencies(ctx context.Context, margin bool) (*types.Response[[]string], error) {
	key := ConfigCurrencies
	if margin {
		key = ConfigMarginCurrencies
	}
	r, e := s.ConfigsTyped(ctx, key)
	if e != nil {
		return nil, e
	}
	return types.NewResponse(r.Data[0].Symbols, r.Meta), nil
}
