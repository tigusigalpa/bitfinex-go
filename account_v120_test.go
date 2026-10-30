package bitfinex_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	bitfinex "github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/account"
	"github.com/tigusigalpa/bitfinex-go/market"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/types"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountListCursors(t *testing.T) {
	for _, kind := range []string{"addresses", "invoices", "payments"} {
		for _, replay := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/replay=%v", kind, replay), func(t *testing.T) {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					raw, _ := io.ReadAll(r.Body)
					m := jsonTree(t, raw).(map[string]any)
					if kind == "addresses" {
						if m["page"] != json.Number(fmt.Sprint(calls)) || m["method"] != "bitcoin" || m["pageSize"] != json.Number("20") {
							t.Errorf("bad numbered request %s", raw)
						}
						rows := [][]string{}
						n := 20
						if calls > 1 && !replay {
							n = 1
						}
						for i := 0; i < n; i++ {
							id := i
							if calls > 1 && !replay {
								id = 20 + i
							}
							rows = append(rows, []string{fmt.Sprintf("address%d", id), "exchange"})
						}
						json.NewEncoder(w).Encode(rows)
					} else {
						want := fmt.Sprint((calls - 1) * 2)
						if m["query"].(map[string]any)["offset"] != json.Number(want) || m["action"] != "get"+map[string]string{"invoices": "Invoices", "payments": "Payments"}[kind]+"ByUser" {
							t.Errorf("bad offset request %s", raw)
						}
						rows := []map[string]any{}
						if calls == 1 || replay {
							for i := 0; i < 2; i++ {
								rows = append(rows, map[string]any{"payment_hash": fmt.Sprint(i), "created_at": 100, "amount": 1, "bfx_amount": 0.00000001, "state": "pending"})
							}
						}
						json.NewEncoder(w).Encode(map[string]any{kind: rows})
					}
				}))
				defer srv.Close()
				c, e := bitfinex.NewClient(bitfinex.WithPrivateRESTBase(srv.URL+"/v2"), bitfinex.WithCredentials("fixture-key", "fixture-secret"), bitfinex.WithRateLimiter(unlimited{}))
				if e != nil {
					t.Fatal(e)
				}
				var n int
				if kind == "addresses" {
					it, err := c.Account.DepositAddressesIterator("bitcoin", 20)
					if calls != 0 {
						t.Fatal("eager I/O")
					}
					n, e = consumeHistoryAddresses(it, err)
				} else {
					n, e = consumeHistory(c.Account.InvoicePaymentsIterator("get" + map[string]string{"invoices": "Invoices", "payments": "Payments"}[kind] + "ByUser"))
				}
				if replay {
					var pe *types.ProtocolError
					if !errors.As(e, &pe) {
						t.Fatal("replayed page did not fail", e)
					}
				} else {
					want := 2
					if kind == "addresses" {
						want = 21
					}
					if e != nil || n != want {
						t.Fatalf("rows %d error %v", n, e)
					}
				}
				if calls != 2 {
					t.Fatalf("calls=%d", calls)
				}
			})
		}
	}
}
func consumeHistoryAddresses(it *types.Iterator[account.DepositAddress], e error) (int, error) {
	if e != nil {
		return 0, e
	}
	n := 0
	for it.Next(context.Background()) {
		n++
		if n > 100 {
			return n, fmt.Errorf("unbounded addresses")
		}
	}
	return n, it.Err()
}

func TestTypedTokenAndRejection(t *testing.T) {
	for _, mode := range []string{"success", "malformed", "503"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				b, _ := io.ReadAll(r.Body)
				m := jsonTree(t, b).(map[string]any)
				if m["scope"] != "api" || m["writePermission"] != false || m["caps"].([]any)[0] != "o" {
					t.Error("wrong token request")
				}
				switch mode {
				case "503":
					w.WriteHeader(503)
					fmt.Fprint(w, `["error",10000,"fixture"]`)
				case "malformed":
					fmt.Fprint(w, `[null]`)
				default:
					fmt.Fprint(w, `["fixture-auth-token"]`)
				}
			}))
			defer srv.Close()
			c, e := bitfinex.NewClient(bitfinex.WithPrivateRESTBase(srv.URL+"/v2"), bitfinex.WithCredentials("fixture-key", "fixture-secret"), bitfinex.WithRateLimiter(unlimited{}), bitfinex.WithRetryPolicy(3, time.Millisecond, time.Millisecond, time.Second))
			if e != nil {
				t.Fatal(e)
			}
			r, e := c.Account.GenerateTokenTyped(context.Background(), account.TokenRequest{TTLSeconds: 60, Capabilities: []string{"o"}})
			if mode == "success" {
				if e != nil || r.Data.Value() != "fixture-auth-token" {
					t.Fatal(e)
				}
				if strings.Contains(fmt.Sprintf("%v %#v", r.Data, r.Data), "fixture-auth-token") {
					t.Fatal("token leaked")
				}
				if _, e := json.Marshal(r.Data); e == nil {
					t.Fatal("token serialized")
				}
			} else if e == nil {
				t.Fatal("expected token failure")
			}
			if calls != 1 {
				t.Fatalf("token retried %d", calls)
			}
		})
	}
}

func TestV120ValidationBeforeIO(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("invalid request made I/O") }))
	defer srv.Close()
	c, e := bitfinex.NewClient(bitfinex.WithPrivateRESTBase(srv.URL+"/v2"), bitfinex.WithPublicRESTBase(srv.URL+"/v2"))
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	calls := []func() error{
		func() error { _, e := c.Positions.SetDerivativeCollateral(ctx, "tBTCF0:USTF0", "USD"); return e }, func() error {
			_, e := c.Positions.SetDerivativeCollateralAmount(ctx, "tBTCUSD", types.MustDecimal("1"))
			return e
		}, func() error {
			_, e := c.Positions.SetDerivativeCollateralAmount(ctx, "tBTCF0:USTF0", types.MustDecimal("-1"))
			return e
		},
		func() error { _, e := c.Positions.AuditMany(ctx, positions.AuditRequest{}); return e }, func() error { _, e := c.Account.DepositAddresses(ctx); return e }, func() error {
			_, e := c.Account.DepositAddressesTyped(ctx, account.DepositAddressesRequest{Method: "bitcoin", PageSize: 19})
			return e
		}, func() error { _, e := c.Account.DepositAddressesIterator("", 20); return e },
		func() error { _, e := c.Account.InvoicePayments(ctx, 10); return e }, func() error {
			_, e := c.Account.InvoicePaymentsTyped(ctx, account.InvoiceQuery{Action: "getInvoiceById"})
			return e
		}, func() error {
			_, e := c.Account.InvoicePaymentsTyped(ctx, account.InvoiceQuery{Action: "getInvoicesByUser", PaymentHash: "hash"})
			return e
		}, func() error { _, e := c.Account.InvoicePaymentsIterator("getInvoiceById"); return e },
		func() error { _, e := c.Account.SettingsTyped(ctx, []string{"invalid"}); return e }, func() error {
			_, e := c.Account.SettingsSetTyped(ctx, []account.Setting{{Key: "api:x"}, {Key: "api:x"}})
			return e
		}, func() error { _, e := c.Account.SettingsDeleteTyped(ctx, nil); return e },
		func() error {
			_, e := c.Account.GenerateTokenTyped(ctx, account.TokenRequest{TTLSeconds: 60, Capabilities: []string{"wd"}})
			return e
		}, func() error {
			_, e := c.Account.GenerateTokenTyped(ctx, account.TokenRequest{TTLSeconds: 60, Capabilities: []string{"invalid"}})
			return e
		},
		func() error {
			_, e := c.Account.ThalexDeposit(ctx, account.ThalexTransferRequest{Currency: "BTC", Amount: types.MustDecimal("1"), Confirm: true})
			return e
		}, func() error {
			_, e := c.Account.InvoiceTyped(ctx, account.InvoiceRequest{Currency: "LNX", Wallet: "exchange", Amount: types.MustDecimal("0.1"), Confirm: true})
			return e
		},
		func() error {
			_, e := c.Market.MarketAveragePriceTyped(ctx, market.AveragePriceRequest{Symbol: "fUSD", Amount: types.MustDecimal("1")})
			return e
		}, func() error { _, e := c.Market.ConfigsTyped(ctx, market.ConfigKey("invalid")); return e },
		func() error { _, e := c.Orders.HistoryIterator("", &types.HistoryQuery{Sort: types.SortAsc}); return e }, func() error { _, e := c.Funding.OffersHistory(ctx, "fUSD", &types.HistoryQuery{Limit: 501}); return e },
	}
	for i, call := range calls {
		var v *types.ValidationError
		if e := call(); !errors.As(e, &v) {
			t.Fatalf("case %d: want ValidationError, got %v", i, e)
		}
	}
	valid := account.TwoFactorToken{Method: "u2f", Response: &account.U2FAssertion{ID: "id", Response: account.U2FClientResponse{ClientDataJSON: "fixture", AttestationObject: "fixture", Signature: "fixture"}}}
	if e := valid.Validate(); e != nil {
		t.Fatal(e)
	}
	valid.Token = "123456"
	if valid.Validate() == nil {
		t.Fatal("mixed factors accepted")
	}
}
