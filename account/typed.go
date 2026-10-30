package account

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"regexp"
)

func accountCall[T any](s *Service, ctx context.Context, path string, body any, write bool, decode func(any) (T, error)) (*types.Response[T], error) {
	rc := readBucket()
	if write {
		rc = writeBucket()
	}
	r, e := s.t.PostAuth(ctx, path, body, rc)
	if e != nil {
		return nil, e
	}
	v, e := decode(r.Data)
	if e != nil {
		return nil, e
	}
	return types.NewResponse(v, httpx.Meta(r)), nil
}

// SummaryTyped returns exact account fees, volume, funding earnings and LEO data.
func (s *Service) SummaryTyped(ctx context.Context) (*types.Response[AccountSummary], error) {
	return accountCall(s, ctx, "v2/auth/r/summary", nil, false, decodeSummary)
}
func decodeSummary(v any) (AccountSummary, error) {
	var out AccountSummary
	a, e := wire.AsArr(v, "summary")
	if e != nil {
		return out, e
	}
	if e = wire.AssertMinLength(a, 10, "summary"); e != nil {
		return out, e
	}
	fees, e := wire.AsArr(a[4], "summary fees")
	if e != nil {
		return out, e
	}
	if e = wire.AssertMinLength(fees, 2, "summary fees"); e != nil {
		return out, e
	}
	for i, p := range []*FeeRates{&out.Maker, &out.Taker} {
		row, e := wire.AsArr(fees[i], "fee rates")
		if e != nil {
			return out, e
		}
		if e = wire.AssertMinLength(row, 6, "fee rates"); e != nil {
			return out, e
		}
		for j, d := range []*types.Decimal{&p.Crypto, &p.Stable, &p.Fiat} {
			*d, e = wire.RequiredDecimal(row, j)
			if e != nil {
				return out, e
			}
		}
		p.Derivatives, e = wire.RequiredDecimal(row, 5)
		if e != nil {
			return out, e
		}
	}
	t, e := wire.AsArr(a[5], "summary trading")
	if e != nil {
		return out, e
	}
	if e = wire.AssertMinLength(t, 3, "summary trading"); e != nil {
		return out, e
	}
	vol, e := wire.AsArr(t[0], "trading volumes")
	if e != nil {
		return out, e
	}
	for _, v := range vol {
		if _, e = wire.Object(v, "curr", "vol"); e != nil {
			return out, e
		}
		x, e := wire.Into[TradingVolume](v)
		if e != nil {
			return out, e
		}
		out.Volumes = append(out.Volumes, x)
	}
	out.TradingFees, e = wire.DecimalMap(t[1])
	if e != nil {
		return out, e
	}
	out.TradingFeesTotal, e = wire.RequiredDecimal(t, 2)
	if e != nil {
		return out, e
	}
	f, e := wire.AsArr(a[6], "funding earnings")
	if e != nil {
		return out, e
	}
	if e = wire.AssertMinLength(f, 3, "funding earnings"); e != nil {
		return out, e
	}
	out.FundingEarnings, e = wire.DecimalMap(f[1])
	if e != nil {
		return out, e
	}
	out.FundingEarningsTotal, e = wire.RequiredDecimal(f, 2)
	if e != nil {
		return out, e
	}
	leo, e := wire.Object(a[9], "leo_lev", "leo_amount_avg")
	if e != nil {
		return out, e
	}
	out.LEOLevel, e = wire.MustInt([]any{leo["leo_lev"]}, 0, "LEO level")
	if e != nil {
		return out, e
	}
	out.LEOAverage, e = wire.RequiredDecimal([]any{leo["leo_amount_avg"]}, 0)
	return out, e
}

// PermissionsTyped returns typed per-scope read/write grants.
func (s *Service) PermissionsTyped(ctx context.Context) (*types.Response[[]Permission], error) {
	return accountCall(s, ctx, "v2/auth/r/permissions", nil, false, func(v any) ([]Permission, error) {
		rows, e := wire.Rows(v, "permissions")
		if e != nil {
			return nil, e
		}
		out := make([]Permission, 0, len(rows))
		for _, r := range rows {
			if e = wire.AssertMinLength(r, 3, "permission"); e != nil {
				return nil, e
			}
			scope, e := wire.RequiredString(r, 0)
			if e != nil {
				return nil, e
			}
			read, e := wire.Status([]any{r[1]})
			if e != nil {
				return nil, e
			}
			write, e := wire.Status([]any{r[2]})
			if e != nil {
				return nil, e
			}
			out = append(out, Permission{Scope: scope, Read: read.Success, Write: write.Success})
		}
		return out, nil
	})
}

// Validate checks token capabilities before a token can be minted.
func (r TokenRequest) Validate() error {
	if r.TTLSeconds < 1 || int64(r.TTLSeconds) > 2147483647 {
		return &types.ValidationError{Field: "ttl", Message: "positive int32 lifetime required"}
	}
	for _, c := range r.Capabilities {
		switch c {
		case "a", "o", "f", "s", "w", "bp":
		case "wd":
			if !r.WritePermission {
				return &types.ValidationError{Field: "caps", Message: "withdraw capability requires explicit write permission"}
			}
		default:
			return &types.ValidationError{Field: "caps", Message: "unknown token capability"}
		}
	}
	return nil
}

// GenerateTokenTyped mints a token, explicitly setting scope=api. Never retried.
func (s *Service) GenerateTokenTyped(ctx context.Context, r TokenRequest) (*types.Response[AuthToken], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	body := map[string]any{"scope": "api", "ttl": r.TTLSeconds, "writePermission": r.WritePermission}
	if len(r.Capabilities) > 0 {
		body["caps"] = r.Capabilities
	}
	if r.ClientIP != "" {
		body["_cust_ip"] = r.ClientIP
	}
	return accountCall(s, ctx, "v2/auth/w/token", body, true, func(v any) (AuthToken, error) {
		a, e := wire.AsArr(v, "token")
		if e != nil {
			return AuthToken{}, e
		}
		token, e := wire.RequiredString(a, 0)
		return AuthToken{value: token}, e
	})
}

// AuditHistoryTyped returns one documented account changelog page (maximum 500).
func (s *Service) AuditHistoryTyped(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]AuditEntry], error) {
	if e := q.ValidateFor(500, false); e != nil {
		return nil, e
	}
	body := q.Body()
	delete(body, "sort")
	return accountCall(s, ctx, "v2/auth/r/audit/hist", body, false, func(v any) ([]AuditEntry, error) {
		rows, e := wire.Rows(v, "audit")
		if e != nil {
			return nil, e
		}
		out := make([]AuditEntry, 0, len(rows))
		for _, r := range rows {
			if e = wire.AssertMinLength(r, 7, "audit entry"); e != nil {
				return nil, e
			}
			var x AuditEntry
			n, e := wire.MustInt(r, 0, "audit timestamp")
			if e != nil {
				return nil, e
			}
			x.MTS = types.Millis(n)
			x.Log, e = wire.RequiredString(r, 2)
			if e != nil {
				return nil, e
			}
			x.IP, e = wire.Str(r, 5)
			if e != nil {
				return nil, e
			}
			x.UserAgent, e = wire.Into[types.JSONValue](r[6])
			if e != nil {
				return nil, e
			}
			out = append(out, x)
		}
		return out, nil
	})
}

var settingKey = regexp.MustCompile(`^api:[A-Za-z0-9_-]*$`)

func validateSettingKeys(keys []string) error {
	if len(keys) == 0 {
		return &types.ValidationError{Field: "keys", Message: "at least one API setting key required"}
	}
	for _, k := range keys {
		if !settingKey.MatchString(k) {
			return &types.ValidationError{Field: "keys", Message: "keys must use the api: prefix and alphanumeric/underscore/hyphen suffix"}
		}
	}
	return nil
}

// SettingsTyped reads self-defined settings with exact, immutable JSON values.
func (s *Service) SettingsTyped(ctx context.Context, keys []string) (*types.Response[[]Setting], error) {
	if e := validateSettingKeys(keys); e != nil {
		return nil, e
	}
	return accountCall(s, ctx, "v2/auth/r/settings", map[string]any{"keys": keys}, false, func(v any) ([]Setting, error) {
		rows, e := wire.Rows(v, "settings")
		if e != nil {
			return nil, e
		}
		out := make([]Setting, 0, len(rows))
		for _, r := range rows {
			if e = wire.AssertMinLength(r, 2, "setting"); e != nil {
				return nil, e
			}
			k, e := wire.RequiredString(r, 0)
			if e != nil {
				return nil, e
			}
			val, e := wire.Into[types.JSONValue](r[1])
			if e != nil {
				return nil, e
			}
			out = append(out, Setting{Key: k, Value: val})
		}
		return out, nil
	})
}
func decodeSettingsResult(v any) (SettingsResult, error) {
	n, e := wire.Notification(v, func(v any) (int64, error) {
		a, e := wire.AsArr(v, "settings count")
		if e != nil {
			return 0, e
		}
		return wire.MustInt(a, 0, "settings count")
	})
	if e != nil {
		return SettingsResult{}, e
	}
	return SettingsResult{MTS: n.MTS, Type: n.Type, Count: n.Data, Status: n.Status}, nil
}

// SettingsSetTyped writes typed settings. Never automatically retried.
func (s *Service) SettingsSetTyped(ctx context.Context, settings []Setting) (*types.Response[SettingsResult], error) {
	keys := make([]string, len(settings))
	values := map[string]types.JSONValue{}
	for i, s := range settings {
		keys[i] = s.Key
		if _, dup := values[s.Key]; dup {
			return nil, &types.ValidationError{Field: "settings", Message: "duplicate setting key"}
		}
		values[s.Key] = s.Value
	}
	if e := validateSettingKeys(keys); e != nil {
		return nil, e
	}
	return accountCall(s, ctx, "v2/auth/w/settings/set", map[string]any{"settings": values}, true, decodeSettingsResult)
}

// SettingsDeleteTyped deletes the selected keys and returns the acknowledgement.
func (s *Service) SettingsDeleteTyped(ctx context.Context, keys []string) (*types.Response[SettingsResult], error) {
	if e := validateSettingKeys(keys); e != nil {
		return nil, e
	}
	return accountCall(s, ctx, "v2/auth/w/settings/del", map[string]any{"settings": keys}, true, decodeSettingsResult)
}

// AlertSetTyped returns the created price alert, rather than raw positional data.
func (s *Service) AlertSetTyped(ctx context.Context, r AlertSetRequest) (*types.Response[Alert], error) {
	if e := r.Validate(); e != nil {
		return nil, e
	}
	if r.Type != "price" || r.Price.Sign() <= 0 {
		return nil, &types.ValidationError{Field: "alert", Message: "positive price alert required"}
	}
	return accountCall(s, ctx, "v2/auth/w/alert/set", map[string]any{"type": r.Type, "symbol": r.Symbol, "price": r.Price.CanonicalString()}, true, func(v any) (Alert, error) {
		a, e := wire.AsArr(v, "alert")
		if e != nil {
			return Alert{}, e
		}
		return alertFromWire(a)
	})
}

// AlertDeleteTyped parses the native boolean status. Never retried.
func (s *Service) AlertDeleteTyped(ctx context.Context, symbol string, price types.Decimal) (*types.Response[types.OperationStatus], error) {
	sym, e := types.ParseSymbol(symbol)
	if e != nil {
		return nil, e
	}
	if !price.IsValid() || price.Sign() <= 0 {
		return nil, &types.ValidationError{Field: "price", Message: "positive alert price required"}
	}
	return accountCall(s, ctx, "v2/auth/w/alert/price:"+sym.String()+":"+price.CanonicalString()+"/del", nil, true, wire.Status)
}
