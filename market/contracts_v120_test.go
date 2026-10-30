package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tigusigalpa/bitfinex-go/internal/ratelimit"
	"github.com/tigusigalpa/bitfinex-go/internal/retry"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestAllConfigurationVariants(t *testing.T) {
	cases := []struct {
		key     ConfigKey
		payload string
		check   func(Configuration) bool
	}{
		{ConfigExchangePairs, `["BTCUSD"]`, func(v Configuration) bool { return v.Symbols[0] == "BTCUSD" }},
		{ConfigMarginPairs, `["BTCUSD"]`, nil}, {ConfigFuturesPairs, `["BTCF0:USTF0"]`, nil}, {ConfigSecuritiesPairs, `["EXOUSD"]`, nil}, {ConfigCurrencies, `["BTC"]`, nil}, {ConfigMarginCurrencies, `["BTC"]`, nil},
		{ConfigCompetitions, `["fixture"]`, func(v Configuration) bool { return v.Competitions[0] == "fixture" }},
		{ConfigCurrencySymbols, `[["BTC","BTC"]]`, nil}, {ConfigCurrencyLabels, `[["BTC","Bitcoin"]]`, nil}, {ConfigCurrencyUnits, `[["BTC","BTC"]]`, nil}, {ConfigCurrencyUnderlying, `[["LNX","BTC"]]`, nil}, {ConfigCurrencyPools, `[["UST","ETH"]]`, nil},
		{ConfigCurrencyExplorers, `[["BTC",["https://example.invalid","address/{id}","tx/{id}"]]]`, func(v Configuration) bool { return v.Explorers[0].Transaction == "tx/{id}" }},
		{ConfigCurrencyFees, `[["BTC",[0.000000000000000001,0.2]]]`, func(v Configuration) bool { return v.Fees[0].Fees[0].String() == "0.000000000000000001" }},
		{ConfigTransactionMethods, `[["bitcoin",["BTC"]]]`, func(v Configuration) bool { return v.Methods[0].Currencies[0] == "BTC" }},
		{ConfigPairInfo, `[["BTCUSD",[100,null,null,0.001,1000,null,null,null,0.1,0.05]]]`, func(v Configuration) bool {
			return *v.Pairs[0].FirstTrade == 100 && v.Pairs[0].MinimumOrder.String() == "0.001" && v.Pairs[0].MinimumMargin.String() == "0.05"
		}},
		{ConfigFuturesPairInfo, `[["BTCF0:USTF0",[null,null,null,0.001,1000,null,null,null,null,null]]]`, func(v Configuration) bool { return v.Pairs[0].FirstTrade == nil && !v.Pairs[0].InitialMargin.IsValid() }},
		{ConfigWalletStatus, `[["bitcoin",1,0,null,null,null,null,0,1,null,null,3]]`, func(v Configuration) bool {
			return v.Wallets[0].Deposits && !v.Wallets[0].Withdrawals && v.Wallets[0].WithdrawalPaymentID && *v.Wallets[0].DepositConfirmations == 3
		}},
		{ConfigDerivativeFees, `{"deriv_perp":{"correction_clamp_min":0.000001,"correction_clamp_insurance":0.2,"correction_slope":0.3}}`, func(v Configuration) bool { return v.DerivativeFees.Perpetual.ClampMinimum.String() == "0.000001" }},
		{ConfigMarginRisk, `{"conf":{"haircut":0.01},"initial":{"BTC":0.2},"minimum":{"BTC":0.1}}`, func(v Configuration) bool { return v.MarginRisk.Initial["BTC"].String() == "0.2" }},
	}
	keys := make([]ConfigKey, len(cases))
	payloads := make([]string, len(cases))
	for i, tc := range cases {
		keys[i] = tc.key
		payloads[i] = tc.payload
		t.Run(string(tc.key), func(t *testing.T) {
			v, e := wire.Decode([]byte(tc.payload))
			if e != nil {
				t.Fatal(e)
			}
			out, e := decodeConfiguration(tc.key, v)
			if e != nil {
				t.Fatal(e)
			}
			if out.Key != tc.key || (tc.check != nil && !tc.check(out)) {
				t.Fatalf("bad configuration: %+v", out)
			}
			if _, e := decodeConfiguration(tc.key, true); e == nil {
				t.Fatal("accepted malformed configuration")
			}
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/conf/"+strings.Join(configKeyStrings(keys), ",") {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, "["+strings.Join(payloads, ",")+"]")
	}))
	defer srv.Close()
	s := New(httpx.New(httpx.Config{Doer: srv.Client(), PublicBase: srv.URL + "/v2", Limiter: ratelimit.New(nil, 0), Retry: retry.NewPolicy(1, 0, 0, 0)}))
	r, e := s.ConfigsTyped(context.Background(), keys...)
	if e != nil || len(r.Data) != len(cases) {
		t.Fatalf("batch: %v", e)
	}
	if _, e := s.ConfigsTyped(context.Background(), ConfigKey("pub:unknown")); e == nil {
		t.Fatal("unknown key")
	}
}
func configKeyStrings(keys []ConfigKey) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = string(k)
	}
	return out
}

func TestDeferredMarketSchemas(t *testing.T) {
	s := `["tBTCF0:USTF0",100,null,101.1234567890123456789,102,null,103,null,200,0.1,0.2,null,0.3,null,null,104,null,null,105,null,null,null,0.4,0.5,"tail"]`
	raw, e := wire.Decode([]byte(s))
	if e != nil {
		t.Fatal(e)
	}
	a := raw.([]any)
	d, e := derivStatusFromWire(a)
	if e != nil || d.DerivPrice.String() != "101.1234567890123456789" || d.MarkPrice.String() != "104" || d.OpenInterest.String() != "105" || d.ClampMax.String() != "0.5" {
		t.Fatalf("derivative: %+v %v", d, e)
	}
	hist, e := DerivativeStatusHistoryFromWire(a[1:], "tBTCF0:USTF0")
	if e != nil || hist != d {
		t.Fatalf("history differs: %+v %v", hist, e)
	}
	lraw, _ := wire.Decode([]byte(`["pos",42,1574697680828.2002,null,"tBTCUSD",-0.01,100,null,1,0,null,99,"tail"]`))
	l, e := liquidationFromWire(lraw.([]any))
	if e != nil || l.MTS != 1574697680828 || l.Timestamp.String() != "1574697680828.2002" || l.PriceAcquired.String() != "99" {
		t.Fatalf("liquidation: %+v %v", l, e)
	}
	fraw, _ := wire.Decode([]byte(`[100,null,null,0.001,2,null,null,1000,900,null,null,12,"tail"]`))
	f, e := fundingStatsFromWire(fraw.([]any))
	if e != nil || f.FRR.String() != "0.001" || f.FundingProvided.String() != "1000" || f.FundingUsed.String() != "900" || f.FundingBelowThreshold.String() != "12" {
		t.Fatalf("funding: %+v %v", f, e)
	}
	rraw, _ := wire.Decode([]byte(`[100,null,"fixture",2,null,null,123.4567890123456789,null,null,"@fixture","tail"]`))
	r, e := rankingFromWire(rraw.([]any))
	if e != nil || r.Rank != 2 || r.Value.String() != "123.4567890123456789" || r.TwitterHandle != "@fixture" {
		t.Fatalf("ranking: %+v %v", r, e)
	}
	for _, decode := range []func([]any) error{func(a []any) error { _, e := derivStatusFromWire(a); return e }, func(a []any) error { _, e := liquidationFromWire(a); return e }, func(a []any) error { _, e := fundingStatsFromWire(a); return e }, func(a []any) error { _, e := rankingFromWire(a); return e }} {
		var p *types.ProtocolError
		if e := decode([]any{json.Number("1")}); !errors.As(e, &p) {
			t.Fatal(e)
		}
	}
}

func TestDeferredPublicRoutes(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{`[1,"tail"]`, `[0]`} {
		r, e := svc(t, raw).PlatformStatus(ctx)
		if e != nil || r.Data < 0 || r.Data > 1 {
			t.Fatal(r, e)
		}
	}
	for _, raw := range []string{`[]`, `[2]`, `[null]`} {
		if _, e := svc(t, raw).PlatformStatus(ctx); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	r, e := svc(t, `[1.1234567890123456789,"tail"]`).ForeignExchangeRate(ctx, "usd", "eur")
	if e != nil || r.Data.String() != "1.1234567890123456789" {
		t.Fatal(r, e)
	}
	if _, e := svc(t, `[]`).ForeignExchangeRate(ctx, "USD", "EUR"); e == nil {
		t.Fatal("missing FX rate")
	}
	stats, e := svc(t, `[100,1.1234567890123456789,"tail"]`).Stats(ctx, "pos.size", "1m", "tBTCUSD", "long", "last", nil)
	if e != nil || stats.Data[0].Value.String() != "1.1234567890123456789" {
		t.Fatal(stats, e)
	}
	if _, e := svc(t, `[[100]]`).Stats(ctx, "pos.size", "1m", "tBTCUSD", "long", "hist", nil); e == nil {
		t.Fatal("invalid stats")
	}
	current := `[["tBTCF0:USTF0",100,null,1,2,null,3,null,200,0.1,0.2,null,0.3,null,null,4,null,null,5,null,null,null,0.4,0.5]]`
	d, e := svc(t, current).DerivativesStatus(ctx, "tBTCF0:USTF0")
	if e != nil || d.Data[0].OpenInterest.String() != "5" {
		t.Fatal(d, e)
	}
	if _, e := svc(t, `[[1]]`).DerivativesStatus(ctx, "tBTCF0:USTF0"); e == nil {
		t.Fatal("invalid derivative")
	}
}
