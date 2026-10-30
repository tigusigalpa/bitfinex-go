package bitfinex_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	bitfinex "github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/funding"
	"github.com/tigusigalpa/bitfinex-go/market"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestRequiredEntityIDs(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		index     int
		decode    func([]any) (int64, error)
	}{
		{"order", orderRow, 0, func(a []any) (int64, error) { v, e := orders.FromWire(a); return v.ID, e }},
		{"own-trade", `[42,"tBTCUSD",100,43,1,100,"LIMIT",100,1,0.01,"USD"]`, 0, func(a []any) (int64, error) { v, e := orders.TradeExecutionFromWire(a); return v.ID, e }},
		{"own-trade-order", `[42,"tBTCUSD",100,43,1,100,"LIMIT",100,1,0.01,"USD"]`, 3, func(a []any) (int64, error) { v, e := orders.TradeExecutionFromWire(a); return v.OrderID, e }},
		{"offer", offerRow, 0, func(a []any) (int64, error) { v, e := funding.OfferFromWire(a); return v.ID, e }},
		{"credit", creditRow, 0, func(a []any) (int64, error) { v, e := funding.CreditFromWire(a); return v.ID, e }},
		{"loan", creditRow, 0, func(a []any) (int64, error) { v, e := funding.LoanFromWire(a); return v.ID, e }},
		{"funding-trade", `[42,"fUSD",100,43,100,0.001,2,1]`, 0, func(a []any) (int64, error) { v, e := funding.TradeFromWire(a); return v.ID, e }},
		{"funding-trade-offer", `[42,"fUSD",100,43,100,0.001,2,1]`, 3, func(a []any) (int64, error) { v, e := funding.TradeFromWire(a); return v.OfferID, e }},
		{"public-trade", `[42,100,1,100]`, 0, func(a []any) (int64, error) { v, e := market.TradeFromWire(a, "tBTCUSD"); return v.ID, e }},
		{"public-funding-trade", `[42,100,1,0.001,2]`, 0, func(a []any) (int64, error) { v, e := market.TradeFromWire(a, "fUSD"); return v.ID, e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := jsonTree(t, []byte(tc.raw)).([]any)
			for _, bad := range []any{nil, "", "invalid-id", true, json.Number("1.5"), json.Number("9223372036854775808"), json.Number("0"), json.Number("-1")} {
				a[tc.index] = bad
				_, e := tc.decode(a)
				var pe *types.ProtocolError
				if !errors.As(e, &pe) {
					t.Fatalf("accepted invalid ID %v: %v", bad, e)
				}
			}
			for _, valid := range []any{json.Number("9007199254740993"), "9007199254740993"} {
				a[tc.index] = valid
				for len(a) < 40 {
					a = append(a, nil)
				}
				v, e := tc.decode(append(a, "unknown-tail"))
				if e != nil || v != 9007199254740993 {
					t.Fatalf("lost exact ID: %d %v", v, e)
				}
			}
		})
	}
}

func TestOptionalRelatedIDsRetainTypeErrors(t *testing.T) {
	for _, index := range []int{1, 2, 25} {
		a := jsonTree(t, []byte(orderRow)).([]any)
		a = append(a, nil)
		a[index] = "invalid-id"
		if _, e := orders.FromWire(a); e == nil {
			t.Fatalf("invalid optional ID at %d accepted", index)
		}
		a[index] = nil
		if _, e := orders.FromWire(a); e != nil {
			t.Fatal(e)
		}
	}
	// Legacy active-position rows have no ID; a present ID still needs a valid integer.
	a := jsonTree(t, []byte(`["tBTCUSD","ACTIVE",1,100,0,0,null,null,null,null]`)).([]any)
	if _, e := positions.FromWire(a); e != nil {
		t.Fatal(e)
	}
	a = append(a, nil, nil, "invalid-id")
	if _, e := positions.FromWire(a); e == nil {
		t.Fatal("malformed position ID accepted")
	}
}

func TestLiquidationNullableAcquiredPriceRESTAndIterator(t *testing.T) {
	// Shape captured from the public API: an initial liquidation has no acquired price.
	const row = `["pos",194840755,1793333832536,null,"tDOGEF0:USTF0",-1e-8,0.076197,null,0,1,null,null]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "[["+row+"]]") }))
	defer srv.Close()
	c, e := bitfinex.NewClient(bitfinex.WithPublicRESTBase(srv.URL+"/v2"), bitfinex.WithRateLimiter(unlimited{}))
	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Market.Liquidations(context.Background(), &types.HistoryQuery{Limit: 3})
	if e != nil || len(r.Data) != 1 || r.Data[0].PriceAcquired.IsValid() {
		t.Fatalf("null price rejected/converted: %+v %v", r, e)
	}
	if n, e := consumeHistory(c.Market.LiquidationsIterator(&types.HistoryQuery{Limit: 3})); e != nil || n != 1 {
		t.Fatalf("iterator: %d %v", n, e)
	}
	a := jsonTree(t, []byte(row)).([]any)
	a[11] = json.Number("0")
	v, e := market.LiquidationFromWire(append(a, "tail"))
	if e != nil || !v.PriceAcquired.IsValid() || v.PriceAcquired.String() != "0" {
		t.Fatalf("zero price confused with absence: %+v %v", v, e)
	}
	for _, bad := range []any{true, "bad", []any{}} {
		a[11] = bad
		if _, e := market.LiquidationFromWire(a); e == nil {
			t.Fatal("malformed acquired price accepted")
		}
	}
	a[11] = nil
	a[6] = nil
	if _, e := market.LiquidationFromWire(a); e == nil {
		t.Fatal("required base price accepted as null")
	}
}
