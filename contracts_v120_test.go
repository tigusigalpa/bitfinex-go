package bitfinex_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bitfinex "github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/account"
	"github.com/tigusigalpa/bitfinex-go/market"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func dataOnly[T any](r *types.Response[T], e error) (any, error) {
	if e != nil {
		return nil, e
	}
	if r.Meta.StatusCode != 200 || r.Meta.Attempts != 1 {
		return nil, fmt.Errorf("invalid response metadata: %+v", r.Meta)
	}
	return r.Data, nil
}
func jsonTree(t *testing.T, b []byte) any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.UseNumber()
	var v any
	if e := d.Decode(&v); e != nil {
		t.Fatal(e)
	}
	return v
}
func checkSubset(t *testing.T, got, want any) {
	t.Helper()
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("got %T, want object", got)
		}
		for k, v := range w {
			checkSubset(t, g[k], v)
		}
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i, v := range w {
			checkSubset(t, g[i], v)
		}
	default:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v (%T), want %v (%T)", got, got, want, want)
		}
	}
}

// Synthetic fixtures follow the official contracts audited in docs/review-v1.2.0.md.
// Each case verifies the complete outgoing body, native response and malformed
// response handling. Monetary writes also encounter an ambiguous HTTP 503 with
// retries enabled, proving that no automatic resubmission occurs.
func TestV120TypedContracts(t *testing.T) {
	ctx := context.Background()
	amount := types.MustDecimal("0.0100")
	price := types.MustDecimal("100.000")
	setting, _ := types.ParseJSONValue([]byte(`{"n":900719925474099312345}`))
	thalex := account.ThalexTransferRequest{Amount: amount, Currency: "BTC", TFA: account.TwoFactorToken{Method: "otp", Token: "123456"}, Confirm: true}
	thalexBody := `{"provider":"thalex","amount":"0.01","ccy":"BTC","tfaToken":{"method":"otp","token":"123456"}}`
	thalexResult := `{"type":"deposit","addressDest":"fixture","amount":0.01,"ccy":"BTC","createdAt":100,"updatedAt":101,"id":"fixture-id","status":"FUTURE_STATUS","fee":0.000000000000000001,"future":1}`
	movement := `[42,"USD","wire",null,"remark",100,101,null,null,"COMPLETED",null,null,1.234567890123456789,0.01,null,null,"dest","memo",null,null,"tx","note",null,null,0.2,7,null,null,"ext","ok","desc",{"iban":"fixture"},"tail"]`
	historyPosition := `["tBTCUSD","CLOSED",1,100,0,0,null,null,null,null,null,42,100,101,null,0,null,2,1,{"tag":"fixture"},"future"]`
	cases := []struct {
		name, method, path, body, query, fixture, bad, want string
		call                                                func(*bitfinex.Client) (any, error)
	}{
		{"collateral", "POST", "auth/w/deriv/collateral/set", `{"symbol":"tBTCF0:USTF0","collateral":"0.01"}`, "", `[1,"tail"]`, `[2]`, `{"success":true}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Positions.SetDerivativeCollateralAmount(ctx, "tBTCF0:USTF0", amount))
		}},
		{"collateral-adapter", "POST", "auth/w/deriv/collateral/set", `{"symbol":"tBTCF0:USTF0","collateral":"0.01"}`, "", `[0]`, `[]`, `{"type":"collateral-status-adapter","status":"ERROR","data":{"success":false}}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Positions.SetDerivativeCollateral(ctx, "tBTCF0:USTF0", "0.0100"))
		}},
		{"collateral-limits", "POST", "auth/calc/deriv/collateral/limits", `{"symbol":"tBTCF0:USTF0"}`, "", `[0.000000000000000001,100,"tail"]`, `[10,1]`, `{"minimum":"0.000000000000000001","maximum":"100"}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Positions.DerivativeCollateralLimitsTyped(ctx, "tBTCF0:USTF0"))
		}},
		{"increase-preview", "POST", "auth/r/position/increase/info", `{"symbol":"tBTCUSD","amount":"0.01"}`, "", `[[10,1],[2,[3,4,5,6],7],null,null,[8,9],["USD","BTC"],"tail"]`, `[[1]]`, `{"maximum_position":"10","tradable":{"base":"5","quote_total":"4"},"funding_required":"9","funding_required_currency":"BTC"}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Positions.IncreaseInfoTyped(ctx, "tBTCUSD", amount))
		}},
		{"margin-base", "POST", "auth/r/info/margin/base", `{}`, "", `["base",[1,null,3,4,5,99]]`, `["base",[1]]`, `[{"type":"base","base":{"pl":"1","balance":"3","required":"5"}}]`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Positions.MarginInfoTyped(ctx, "")) }},
		{"margin-symbols", "POST", "auth/r/info/margin/sym_all", `{}`, "", `[["sym","tBTCUSD",[1,2,3,4]]]`, `[["sym","tBTCUSD",[]]]`, `[{"type":"sym","symbol":"tBTCUSD","market":{"buy":"3","sell":"4"}}]`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Positions.MarginInfoTyped(ctx, "sym_all")) }},
		{"funding-info", "POST", "auth/r/info/funding/fUSD", `{}`, "", `["sym","fUSD",[0.001,0.002,2,3,99]]`, `["sym","fUSD",[]]`, `{"symbol":"fUSD","yield_loan":"0.001","duration_lend":"3"}`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Funding.InfoTyped(ctx, "fUSD")) }},
		{"summary", "POST", "auth/r/summary", `{}`, "", `[null,null,null,null,[[0.1,0.2,0.3,null,null,0.4],[0.5,0.6,0.7,null,null,0.8]],[[{"curr":"USD","vol":9007199254740993.123456789}],{"USD":0.01},0.01],[null,{"USD":0.02},0.02],null,null,{"leo_lev":2,"leo_amount_avg":100}]`, `[]`, `{"maker":{"derivatives":"0.4"},"volumes":[{"curr":"USD","vol":"9007199254740993.123456789"}],"leo_level":2,"funding_earnings":{"USD":"0.02"}}`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Account.SummaryTyped(ctx)) }},
		{"permissions", "POST", "auth/r/permissions", `{}`, "", `[["orders",1,0],["future-scope",0,1]]`, `[["orders",2,1]]`, `[{"scope":"orders","read":true,"write":false},{"scope":"future-scope","read":false,"write":true}]`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Account.PermissionsTyped(ctx)) }},
		{"audit", "POST", "auth/r/audit/hist", `{"limit":2}`, "", `[[100,null,"changed",null,null,"192.0.2.1",{"browser":"fixture"},"tail"]]`, `[[100]]`, `[{"mts":100,"log":"changed","ip":"192.0.2.1","user_agent":{"browser":"fixture"}}]`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.AuditHistoryTyped(ctx, &types.HistoryQuery{Limit: 2, Sort: types.SortDesc}))
		}},
		{"settings", "POST", "auth/r/settings", `{"keys":["api:fixture"]}`, "", `[["api:fixture",{"n":900719925474099312345}]]`, `[[1,2]]`, `[{"key":"api:fixture","value":{"n":900719925474099312345}}]`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.SettingsTyped(ctx, []string{"api:fixture"}))
		}},
		{"settings-set", "POST", "auth/w/settings/set", `{"settings":{"api:fixture":{"n":900719925474099312345}}}`, "", `[100,"settings-set",null,null,[1],0,"SUCCESS","ok"]`, `[100]`, `{"count":1,"status":"SUCCESS"}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.SettingsSetTyped(ctx, []account.Setting{{Key: "api:fixture", Value: setting}}))
		}},
		{"settings-delete", "POST", "auth/w/settings/del", `{"settings":["api:fixture"]}`, "", `[100,"settings-del",null,null,[1],0,"SUCCESS","ok"]`, `[100]`, `{"count":1,"status":"SUCCESS"}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.SettingsDeleteTyped(ctx, []string{"api:fixture"}))
		}},
		{"alert-set", "POST", "auth/w/alert/set", `{"type":"price","symbol":"tBTCUSD","price":"100"}`, "", `["key","price","tBTCUSD",100,2,"tail"]`, `[]`, `{"key":"key","price":"100","count":2}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.AlertSetTyped(ctx, account.AlertSetRequest{Type: "price", Symbol: "tBTCUSD", Price: price}))
		}},
		{"alert-delete", "POST", "auth/w/alert/price:tBTCUSD:100/del", `{}`, "", `[true]`, `[null]`, `{"success":true}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.AlertDeleteTyped(ctx, "tBTCUSD", price))
		}},
		{"deposit-list", "POST", "auth/r/deposit/address/all", `{"method":"bitcoin","page":1,"pageSize":20}`, "", `[["address","exchange","tail"]]`, `[["address"]]`, `[{"address":"address","wallet":"exchange"}]`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.DepositAddressesTyped(ctx, account.DepositAddressesRequest{Method: "bitcoin"}))
		}},
		{"lightning-list", "POST", "auth/r/ext/invoice/payments", `{"action":"getInvoicesByUser","query":{"offset":3}}`, "", `{"invoices":[{"payment_hash":"hash","created_at":100,"invoice_expiry":200,"amount":1000,"bfx_amount":0.00001,"state":"settled","future":1}]}`, `{"invoices":[{}]}`, `{"invoices":[{"payment_hash":"hash","amount":"1000","bfx_amount":"0.00001","invoice_expiry":200}]}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.InvoicePaymentsTyped(ctx, account.InvoiceQuery{Action: "getInvoicesByUser", Offset: 3}))
		}},
		{"lightning-id", "POST", "auth/r/ext/invoice/payments", `{"action":"getPaymentById","query":{"txid":"hash"}}`, "", `{"payment_hash":"hash","created_at":100,"amount":1000,"bfx_amount":0.00001,"state":"pending"}`, `{}`, `{"record":{"payment_hash":"hash","state":"pending"}}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.InvoicePaymentsTyped(ctx, account.InvoiceQuery{Action: "getPaymentById", PaymentHash: "hash"}))
		}},
		{"movement", "POST", "auth/r/movements/info", `{"id":42}`, "", movement, `[]`, `{"id":42,"amount":"1.234567890123456789","destination_address":"dest","transaction_id":"tx","memo":"memo","bank_router_id":7,"external_status":"ok","bank_account":{"iban":"fixture"}}`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Account.MovementInfoTyped(ctx, 42)) }},
		{"transfer", "POST", "auth/w/transfer", `{"from":"exchange","to":"margin","currency":"USD","currency_to":"USD","amount":"0.01"}`, "", `[100,"transfer",null,null,[99,"exchange","margin",null,"USD","USD",null,0.01],0,"SUCCESS","ok"]`, `[100,"transfer",null,null,[],0,"SUCCESS","ok"]`, `{"status":"SUCCESS","data":{"mts":99,"from":"exchange","amount":"0.01"}}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.TransferTyped(ctx, account.TransferRequest{From: "exchange", To: "margin", Currency: "USD", Amount: amount, Confirm: true}))
		}},
		{"deposit-create", "POST", "auth/w/deposit/address", `{"wallet":"exchange","method":"bitcoin","op_renew":1}`, "", `[100,"deposit",null,null,[null,"bitcoin","BTC",null,"address","memo"],0,"SUCCESS","ok"]`, `[100,"deposit",null,null,[],0,"SUCCESS","ok"]`, `{"data":{"address":"address","pool_address":"memo"}}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.DepositAddressTyped(ctx, account.DepositAddressRequest{Wallet: "exchange", Method: "bitcoin", Renew: true}))
		}},
		{"invoice-create", "POST", "auth/w/deposit/invoice", `{"wallet":"exchange","currency":"LNX","amount":"0.01"}`, "", `["hash","lninvoice",null,null,"0.01","tail"]`, `[]`, `{"hash":"hash","invoice":"lninvoice","amount":"0.01"}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.InvoiceTyped(ctx, account.InvoiceRequest{Wallet: "exchange", Currency: "LNX", Amount: amount, Confirm: true}))
		}},
		{"withdraw", "POST", "auth/w/withdraw", `{"wallet":"exchange","method":"bitcoin","address":"address","amount":"0.01"}`, "", `[100,"withdraw",null,null,[42,null,"bitcoin","memo","exchange",0.01,null,null,0.0001],0,"SUCCESS","ok"]`, `[100,"withdraw",null,null,[],0,"SUCCESS","ok"]`, `{"data":{"id":42,"method":"bitcoin","payment_id":"memo","amount":"0.01","fee":"0.0001"}}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Account.WithdrawTyped(ctx, account.WithdrawRequest{Wallet: "exchange", Method: "bitcoin", Address: "address", Amount: amount, Confirm: true}))
		}},
		{"thalex-deposit", "POST", "auth/w/ext/wallets/deposits/request", thalexBody, "", thalexResult, `{}`, `{"id":"fixture-id","amount":"0.01","fee":"0.000000000000000001","status":"FUTURE_STATUS"}`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Account.ThalexDeposit(ctx, thalex)) }},
		{"thalex-withdraw", "POST", "auth/w/ext/wallets/withdrawals/request", thalexBody, "", thalexResult, `{}`, `{"id":"fixture-id"}`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Account.ThalexWithdraw(ctx, thalex)) }},
		{"thalex-count", "POST", "auth/r/ext/wallets/transfers/free/count", `{"Provider":"thalex"}`, "", `{"deposits":{"available":2,"resetsAt":null},"withdrawals":{"available":1,"resetsAt":100}}`, `{"deposits":{}}`, `{"deposits":{"available":2,"resetsAt":null},"withdrawals":{"available":1,"resetsAt":100}}`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Account.ThalexFreeTransferCountTyped(ctx)) }},
		{"position-history", "POST", "auth/r/positions/hist", `{}`, "", "[" + historyPosition + "]", `[[1]]`, `[{"id":42,"mts_create":100,"mts_update":101,"collateral":"2","collateral_min":"1"}]`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Positions.History(ctx, nil)) }},
		{"position-snapshot", "POST", "auth/r/positions/snap", `{}`, "", "[" + historyPosition + "]", `[[1]]`, `[{"id":42,"mts_update":101}]`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Positions.Snapshot(ctx, nil)) }},
		{"position-audit", "POST", "auth/r/positions/audit", `{"id":[42,43]}`, "", "[" + historyPosition + "]", `[[1]]`, `[{"id":42,"mts_update":101}]`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Positions.AuditMany(ctx, positions.AuditRequest{IDs: []int64{42, 43}}))
		}},
		{"otc", "POST", "auth/r/orders/otc/tBTCUSD/hist", `{}`, "", `[[42,"tBTCUSD",100,101,null,1,"alice","bob",null,0.01,100,null,"EXECUTED","tail"]]`, `[[42]]`, `[{"id":42,"initiator":true,"initiator_nickname":"alice","amount":"0.01","price":"100"}]`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Orders.OTCHistoryTyped(ctx, "tBTCUSD", nil)) }},
		{"vasps", "GET", "ext/vasps", "", "", `[{"id":"did:fixture","name":"Fixture","future":1}]`, `[{}]`, `[{"id":"did:fixture","name":"Fixture"}]`, func(c *bitfinex.Client) (any, error) { return dataOnly(c.Market.VASPsTyped(ctx)) }},
		{"average-trade", "POST", "calc/trade/avg", `{}`, "amount=0.01&symbol=tBTCUSD", `[100.1234567890123456789,0.01,"tail"]`, `[]`, `{"price_avg":"100.1234567890123456789","amount":"0.01"}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Market.MarketAveragePriceTyped(ctx, market.AveragePriceRequest{Symbol: "tBTCUSD", Amount: amount}))
		}},
		{"average-funding", "POST", "calc/trade/avg", `{}`, "amount=0.01&period=2&rate_limit=100&symbol=fUSD", `[0.0001,0.01]`, `[]`, `{"rate_avg":"0.0001"}`, func(c *bitfinex.Client) (any, error) {
			return dataOnly(c.Market.MarketAveragePriceTyped(ctx, market.AveragePriceRequest{Symbol: "fUSD", Amount: amount, Period: 2, RateLimit: &price}))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"success", "malformed", "server"} {
				t.Run(mode, func(t *testing.T) {
					var calls atomic.Int32
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.Method != tc.method || r.URL.Path != "/v2/"+tc.path || r.URL.Query().Encode() != tc.query {
							t.Errorf("route: %s %s", r.Method, r.URL.String())
						}
						body, _ := io.ReadAll(r.Body)
						if tc.body != "" {
							if !reflect.DeepEqual(jsonTree(t, body), jsonTree(t, []byte(tc.body))) {
								t.Errorf("body: %s; want %s", body, tc.body)
							}
						}
						if strings.HasPrefix(tc.path, "auth/") && r.Header.Get("bfx-signature") == "" {
							t.Error("unsigned")
						}
						switch mode {
						case "server":
							w.WriteHeader(503)
							fmt.Fprint(w, `["error",10000,"synthetic"]`)
						case "malformed":
							fmt.Fprint(w, tc.bad)
						default:
							fmt.Fprint(w, tc.fixture)
						}
					}))
					defer srv.Close()
					attempts := 1
					if mode == "server" {
						attempts = 3
					}
					c, e := bitfinex.NewClient(bitfinex.WithPublicRESTBase(srv.URL+"/v2"), bitfinex.WithPrivateRESTBase(srv.URL+"/v2"), bitfinex.WithCredentials("fixture-key", "fixture-secret"), bitfinex.WithRateLimiter(unlimited{}), bitfinex.WithRetryPolicy(attempts, time.Millisecond, time.Millisecond, time.Second))
					if e != nil {
						t.Fatal(e)
					}
					got, e := tc.call(c)
					if mode == "success" {
						if e != nil {
							t.Fatal(e)
						}
						b, e := json.Marshal(got)
						if e != nil {
							t.Fatal(e)
						}
						checkSubset(t, jsonTree(t, b), jsonTree(t, []byte(tc.want)))
					} else if e == nil {
						t.Fatal("expected error")
					} else if mode == "malformed" {
						var pe *types.ProtocolError
						if !errors.As(e, &pe) {
							t.Fatalf("want protocol error: %T %v", e, e)
						}
					}
					if strings.Contains(tc.path, "/w/") && calls.Load() != 1 {
						t.Fatalf("write submitted %d times", calls.Load())
					}
				})
			}
		})
	}
}
