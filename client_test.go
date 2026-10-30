package bitfinex_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bitfinex "github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/account"
	"github.com/tigusigalpa/bitfinex-go/funding"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/types"
)

type unlimited struct{}

func (unlimited) Wait(context.Context, string) error { return nil }
func (unlimited) Penalize(string, float64)           {}

const orderRow = `[42,0,7,"tBTCUSD",1700000000000,1700000000001,0.1,0.1,"EXCHANGE LIMIT",null,null,null,0,"ACTIVE",null,null,100,100,0,0,null,null,null,0,0]`
const offerRow = `[42,"fUSD",1700000000000,1700000000001,100,100,"LIMIT",null,null,0,"ACTIVE",null,null,null,0.001,2,0,0,null,0]`
const creditRow = `[42,"fUSD",1,1700000000000,1700000000001,100,0,"ACTIVE",null,null,null,0.001,2,1700000000000,1700000000001,null,null,null,0,0.001]`
const positionRow = `["tBTCUSD","ACTIVE",0.1,100,0,0,1,1,90,2,null,null,42,1700000000000,1700000000001,null,0,null,1,1]`
const notice = `[1700000000000,"test-req",1,null,[],0,"SUCCESS","accepted"]`

// These are synthetic transport/decoder fixtures, not live exchange observations.
// Each case checks routing, signing, decoding, server-error propagation and that
// an ambiguous write response is sent exactly once.
func TestRESTServiceContracts(t *testing.T) {
	ctx := context.Background()
	d := types.MustDecimal("0.1000")
	price := types.MustDecimal("100.00")
	cases := []struct {
		name, path, fixture string
		call                func(*bitfinex.Client) (any, error)
	}{
		{"wallets", "auth/r/wallets", `[["exchange","USD",123.45,0,120]]`, func(c *bitfinex.Client) (any, error) { return c.Wallets.All(ctx) }},
		{"orders", "auth/r/orders/tBTCUSD", "[" + orderRow + "]", func(c *bitfinex.Client) (any, error) { return c.Orders.Active(ctx, "tBTCUSD") }},
		{"order-history", "auth/r/orders/hist", "[" + orderRow + "]", func(c *bitfinex.Client) (any, error) { return c.Orders.History(ctx, "", nil) }},
		{"order-trades", "auth/r/order/tBTCUSD:42/trades", `[[42,"tBTCUSD",1700000000000,42,0.1,100,"LIMIT",100,1,0.01,"USD"]]`, func(c *bitfinex.Client) (any, error) { return c.Orders.OrderTrades(ctx, "tBTCUSD", 42) }},
		{"own-trades", "auth/r/trades/tBTCUSD/hist", `[]`, func(c *bitfinex.Client) (any, error) { return c.Orders.Trades(ctx, "tBTCUSD", nil) }},
		{"otc", "auth/r/orders/otc/tBTCUSD/hist", `[]`, func(c *bitfinex.Client) (any, error) { return c.Orders.OTCHistory(ctx, "tBTCUSD", nil) }},
		{"submit", "auth/w/order/submit", `[1700000000000,"on-req",1,null,[` + orderRow + `],0,"SUCCESS","accepted"]`, func(c *bitfinex.Client) (any, error) {
			return c.Orders.Submit(ctx, orders.SubmitRequest{Type: orders.TypeExchangeLimit, Symbol: "tBTCUSD", Amount: d, Price: &price})
		}},
		{"update", "auth/w/order/update", notice, func(c *bitfinex.Client) (any, error) {
			return c.Orders.Update(ctx, orders.UpdateRequest{ID: 42, Price: &price})
		}},
		{"cancel", "auth/w/order/cancel", notice, func(c *bitfinex.Client) (any, error) { return c.Orders.Cancel(ctx, orders.ByID(42)) }},
		{"cancel-multi", "auth/w/order/cancel/multi", notice, func(c *bitfinex.Client) (any, error) {
			return c.Orders.CancelMulti(ctx, orders.CancelMultiRequest{IDs: []int64{42}})
		}},
		{"multi", "auth/w/order/multi", notice, func(c *bitfinex.Client) (any, error) {
			return c.Orders.Multi(ctx, []orders.MultiOp{{Op: "oc", Args: map[string]any{"id": 42}}})
		}},
		{"positions", "auth/r/positions", "[" + positionRow + "]", func(c *bitfinex.Client) (any, error) { return c.Positions.Active(ctx) }},
		{"position-history", "auth/r/positions/hist", `[]`, func(c *bitfinex.Client) (any, error) { return c.Positions.History(ctx, nil) }},
		{"position-snapshot", "auth/r/positions/snap", `[]`, func(c *bitfinex.Client) (any, error) { return c.Positions.Snapshot(ctx, nil) }},
		{"position-audit", "auth/r/positions/audit", `[]`, func(c *bitfinex.Client) (any, error) { return c.Positions.Audit(ctx, 42, nil) }},
		{"claim", "auth/w/position/claim", notice, func(c *bitfinex.Client) (any, error) { return c.Positions.Claim(ctx, positions.ClaimRequest{ID: 42}) }},
		{"increase", "auth/w/position/increase", notice, func(c *bitfinex.Client) (any, error) {
			return c.Positions.Increase(ctx, positions.IncreaseRequest{Symbol: "tBTCUSD", Amount: d})
		}},
		{"increase-info", "auth/r/position/increase/info", `[]`, func(c *bitfinex.Client) (any, error) { return c.Positions.IncreaseInfo(ctx, "tBTCUSD", d) }},
		{"funding-type", "auth/w/position/update/funding/type", notice, func(c *bitfinex.Client) (any, error) { return c.Positions.UpdateFundingType(ctx, "tBTCUSD", 1) }},
		{"collateral-limits", "auth/calc/deriv/collateral/limits", `[]`, func(c *bitfinex.Client) (any, error) {
			return c.Positions.DerivativeCollateralLimits(ctx, "tBTCF0:USTF0")
		}},
		{"margin-info", "auth/r/info/margin/base", `["base",[100,50,40]]`, func(c *bitfinex.Client) (any, error) { return c.Positions.MarginInfo(ctx, "") }},
		{"offers", "auth/r/funding/offers/fUSD", "[" + offerRow + "]", func(c *bitfinex.Client) (any, error) { return c.Funding.Offers(ctx, "fUSD") }},
		{"offer-submit", "auth/w/funding/offer/submit", notice, func(c *bitfinex.Client) (any, error) {
			return c.Funding.SubmitOffer(ctx, funding.OfferRequest{Type: funding.TypeLimit, Symbol: "fUSD", Amount: d, Rate: d, Period: 2})
		}},
		{"offer-cancel", "auth/w/funding/offer/cancel", notice, func(c *bitfinex.Client) (any, error) { return c.Funding.CancelOffer(ctx, 42) }},
		{"offer-cancel-all", "auth/w/funding/offer/cancel/all", notice, func(c *bitfinex.Client) (any, error) { return c.Funding.CancelAllOffers(ctx, "fUSD") }},
		{"funding-close", "auth/w/funding/close", notice, func(c *bitfinex.Client) (any, error) { return c.Funding.Close(ctx, 42) }},
		{"auto-renew", "auth/w/funding/auto", notice, func(c *bitfinex.Client) (any, error) {
			return c.Funding.AutoRenew(ctx, funding.AutoRenewRequest{Currency: "USD", Amount: &d, Rate: &d, Period: ptr(2)})
		}},
		{"keep", "auth/w/funding/keep", notice, func(c *bitfinex.Client) (any, error) { return c.Funding.Keep(ctx, "credit", []int64{42}) }},
		{"offer-history", "auth/r/funding/offers/fUSD/hist", "[" + offerRow + "]", func(c *bitfinex.Client) (any, error) { return c.Funding.OffersHistory(ctx, "fUSD", nil) }},
		{"loans", "auth/r/funding/loans/fUSD", "[" + creditRow + "]", func(c *bitfinex.Client) (any, error) { return c.Funding.Loans(ctx, "fUSD") }},
		{"loan-history", "auth/r/funding/loans/fUSD/hist", `[]`, func(c *bitfinex.Client) (any, error) { return c.Funding.LoansHistory(ctx, "fUSD", nil) }},
		{"credits", "auth/r/funding/credits/fUSD", "[" + creditRow + "]", func(c *bitfinex.Client) (any, error) { return c.Funding.Credits(ctx, "fUSD") }},
		{"credit-history", "auth/r/funding/credits/fUSD/hist", `[]`, func(c *bitfinex.Client) (any, error) { return c.Funding.CreditsHistory(ctx, "fUSD", nil) }},
		{"funding-trades", "auth/r/funding/trades/fUSD/hist", `[[42,"fUSD",1700000000000,42,100,0.001,2,1]]`, func(c *bitfinex.Client) (any, error) { return c.Funding.TradesHistory(ctx, "fUSD", nil) }},
		{"funding-info", "auth/r/info/funding/sym_fUSD", `["sym",["fUSD",1,2]]`, func(c *bitfinex.Client) (any, error) { return c.Funding.Info(ctx, "sym_fUSD") }},
		{"user", "auth/r/info/user", `[42,"fixture@example.invalid","fixture",1700000000000,1,1,null,"UTC","en",""]`, func(c *bitfinex.Client) (any, error) { return c.Account.UserInfo(ctx) }},
		{"summary", "auth/r/summary", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.Summary(ctx) }},
		{"logins", "auth/r/logins/hist", `[[42,null,1700000000000,null,"192.0.2.1",null,null,"{}"]]`, func(c *bitfinex.Client) (any, error) { return c.Account.LoginHistory(ctx, nil) }},
		{"permissions", "auth/r/permissions", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.Permissions(ctx) }},
		{"token", "auth/w/token", `[]`, func(c *bitfinex.Client) (any, error) {
			return c.Account.GenerateToken(ctx, 60, []string{"orders"}, false)
		}},
		{"audit", "auth/r/audit/hist", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.AuditHistory(ctx, nil) }},
		{"ledgers", "auth/r/ledgers/USD/hist", `[[42,"USD",null,1700000000000,null,1.23,123.45,null,"fixture"]]`, func(c *bitfinex.Client) (any, error) { return c.Account.Ledgers(ctx, "USD", nil) }},
		{"available", "auth/calc/order/avail", `[123.45]`, func(c *bitfinex.Client) (any, error) {
			return c.Account.AvailableBalanceForOrder(ctx, "tBTCUSD", "LIMIT", d, nil, nil)
		}},
		{"alerts", "auth/r/alerts", `[["fixture","price","tBTCUSD",100,1]]`, func(c *bitfinex.Client) (any, error) { return c.Account.Alerts(ctx) }},
		{"alert-set", "auth/w/alert/set", `[]`, func(c *bitfinex.Client) (any, error) {
			return c.Account.AlertSet(ctx, account.AlertSetRequest{Type: "price", Symbol: "tBTCUSD", Price: price})
		}},
		{"alert-delete", "auth/w/alert/price:tBTCUSD:100/del", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.AlertDelete(ctx, "tBTCUSD", price) }},
		{"settings", "auth/r/settings", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.Settings(ctx, []string{"fixture"}) }},
		{"settings-set", "auth/w/settings/set", `[]`, func(c *bitfinex.Client) (any, error) {
			return c.Account.SettingsSet(ctx, map[string]any{"fixture": "value"})
		}},
		{"settings-delete", "auth/w/settings/del", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.SettingsDelete(ctx, []string{"fixture"}) }},
		{"transfer", "auth/w/transfer", notice, func(c *bitfinex.Client) (any, error) {
			return c.Account.Transfer(ctx, account.TransferRequest{From: "exchange", To: "margin", Currency: "USD", Amount: d, Confirm: true})
		}},
		{"deposit-address", "auth/w/deposit/address", notice, func(c *bitfinex.Client) (any, error) {
			return c.Account.DepositAddress(ctx, account.DepositAddressRequest{Method: "bitcoin", Wallet: "exchange"})
		}},
		{"deposit-addresses", "auth/r/deposit/address/all", `[]`, func(c *bitfinex.Client) (any, error) {
			return c.Account.DepositAddressesTyped(ctx, account.DepositAddressesRequest{Method: "bitcoin"})
		}},
		{"invoice", "auth/w/deposit/invoice", `["hash","lninvoice",null,null,"0.01"]`, func(c *bitfinex.Client) (any, error) {
			return c.Account.Invoice(ctx, account.InvoiceRequest{Currency: "LNX", Wallet: "exchange", Amount: types.MustDecimal("0.01"), Confirm: true})
		}},
		{"invoice-payments", "auth/r/ext/invoice/payments", `{"payments":[]}`, func(c *bitfinex.Client) (any, error) {
			return c.Account.InvoicePaymentsTyped(ctx, account.InvoiceQuery{Action: "getPaymentsByUser"})
		}},
		{"withdraw", "auth/w/withdraw", notice, func(c *bitfinex.Client) (any, error) {
			return c.Account.Withdraw(ctx, account.WithdrawRequest{Method: "bitcoin", Wallet: "exchange", Address: "fixture-destination", Amount: d, Confirm: true})
		}},
		{"movements", "auth/r/movements/USD/hist", `[[42,"USD","US Dollar",null,null,1700000000000,1700000000001,null,null,"COMPLETED",null,null,1.23,0.01,null,null,"fixture",null,null,null,"fixture-tx"]]`, func(c *bitfinex.Client) (any, error) { return c.Account.Movements(ctx, "USD", nil) }},
		{"movement-info", "auth/r/movements/info", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.MovementInfo(ctx, 42) }},
		{"thalex-count", "auth/r/ext/wallets/transfers/free/count", `[]`, func(c *bitfinex.Client) (any, error) { return c.Account.ThalexFreeTransferCount(ctx) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, status := range []int{200, 503} {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != "POST" || r.URL.Path != "/v2/"+tc.path {
						t.Errorf("wrong route %s %s", r.Method, r.URL.Path)
					}
					if r.Header.Get("bfx-signature") == "" {
						t.Error("missing signature")
					}
					body, _ := io.ReadAll(r.Body)
					if !json.Valid(body) {
						t.Error("invalid request JSON")
					}
					w.WriteHeader(status)
					if status == 200 {
						_, _ = io.WriteString(w, tc.fixture)
					} else {
						_, _ = io.WriteString(w, `["error",10020,"rejected"]`)
					}
				}))
				attempts := 1
				if strings.HasPrefix(tc.path, "auth/w/") {
					attempts = 3
				}
				c, err := bitfinex.NewClient(bitfinex.WithPrivateRESTBase(srv.URL+"/v2"), bitfinex.WithCredentials("fixture-key", "fixture-secret"), bitfinex.WithRateLimiter(unlimited{}), bitfinex.WithRetryPolicy(attempts, time.Millisecond, time.Millisecond, time.Second))
				if err != nil {
					t.Fatal(err)
				}
				result, err := tc.call(c)
				srv.Close()
				if status == 200 && (err != nil || result == nil) {
					t.Fatalf("fixture rejected: %v", err)
				}
				if status != 200 {
					var api *types.APIError
					if !errors.As(err, &api) || api.StatusCode != status {
						t.Fatalf("lost server error: %v", err)
					}
				}
				if calls != 1 {
					t.Fatalf("ambiguous operation sent %d times", calls)
				}
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestSignedRedirectIsBlocked(t *testing.T) {
	var received atomic.Int32
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer dest.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusTemporaryRedirect)
	}))
	defer src.Close()
	c, err := bitfinex.NewClient(bitfinex.WithHTTPClient(src.Client()), bitfinex.WithPrivateRESTBase(src.URL+"/v2"), bitfinex.WithCredentials("fixture-key", "fixture-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Wallets.All(context.Background()); err == nil || received.Load() != 0 {
		t.Fatal("signed redirect was followed")
	}
}

func TestClientOptionsFailClosed(t *testing.T) {
	for _, o := range []bitfinex.Option{bitfinex.WithPublicRESTBase("https://name:password@example.invalid"), bitfinex.WithPrivateRESTBase("https://example.invalid?token=private"), bitfinex.WithPublicWSURL("https://example.invalid"), bitfinex.WithHTTPClient(nil), bitfinex.WithUserAgent(""), bitfinex.WithTimeout(0)} {
		if _, err := bitfinex.NewClient(o); err == nil {
			t.Fatal("invalid option accepted")
		} else if strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "token=private") {
			t.Fatal("URL credentials exposed")
		}
	}
	c, _ := bitfinex.NewClient()
	if _, err := c.Wallets.All(context.Background()); !errors.Is(err, types.ErrCredentialsMissing) {
		t.Fatal("public-only private request did not fail locally")
	}
	ws := c.NewWebSocket()
	if ws == nil {
		t.Fatal("no websocket client")
	}
	_ = ws.Close()
}
