package market

import (
	"context"
	"testing"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/ratelimit"
	"github.com/tigusigalpa/bitfinex-go/internal/retry"
	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func svc(t *testing.T, bodies ...string) *Service {
	t.Helper()
	doer := &testutil.FakeDoer{}
	for _, b := range bodies {
		doer.Responses = append(doer.Responses, testutil.JSONResponse(200, b, nil))
	}
	tr := httpx.New(httpx.Config{
		Doer:       doer,
		PublicBase: "https://pub.example/v2",
		Limiter:    ratelimit.New(map[string]float64{"*": 60000}, 0),
		Retry:      retry.NewPolicy(1, 0, 0, 0),
	})
	return New(tr)
}

func TestTickerTrading(t *testing.T) {
	s := svc(t, `[65000,1.5,65010,2.0,100,0.0015,65005,2500.5,66000,64000]`)
	r, err := s.Ticker(context.Background(), "tBTCUSD")
	if err != nil {
		t.Fatal(err)
	}
	tk := r.Data
	if tk.LastPrice.String() != "65005" || tk.Bid.String() != "65000" || tk.Volume.String() != "2500.5" {
		t.Fatalf("bad ticker: %+v", tk)
	}
	if r.Meta.StatusCode != 200 {
		t.Fatal("bad meta")
	}
}

func TestTickerFunding(t *testing.T) {
	s := svc(t, `[0.0004,0.00035,2,30000,0.00045,3,20000,0.00001,0.02,0.0004,50000,0.0005,0.0003,null,null,15000,1700000000000]`)
	r, err := s.Ticker(context.Background(), "fUSD")
	if err != nil {
		t.Fatal(err)
	}
	tk := r.Data
	if tk.FRR.String() != "0.0004" || tk.BidPeriod != 2 || tk.FRRAmountAvailable.String() != "15000" {
		t.Fatalf("bad funding ticker: %+v", tk)
	}
}

func TestTickersList(t *testing.T) {
	s := svc(t, `[["tBTCUSD",65000,1.5,65010,2.0,100,0.0015,65005,2500.5,66000,64000],["tETHUSD",3000,1,3010,2,10,0.003,3005,500,3100,2900]]`)
	r, err := s.Tickers(context.Background(), "tBTCUSD", "tETHUSD")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Data) != 2 || r.Data[1].Symbol != "tETHUSD" {
		t.Fatalf("bad list: %+v", r.Data)
	}
}

func TestBookValidationAndDecode(t *testing.T) {
	s := svc(t, `[[65000,3,1.5],[65100,2,-0.5]]`)
	r, err := s.Book(context.Background(), "tBTCUSD", "P0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Data) != 2 || r.Data[0].Entry == nil || r.Data[0].Entry.Count != 3 {
		t.Fatalf("bad book: %+v", r.Data)
	}
	if r.Data[1].Entry.Amount.String() != "-0.5" {
		t.Fatal("ask side wrong")
	}
	if _, err := s.Book(context.Background(), "tBTCUSD", "P9", nil); err == nil {
		t.Fatal("invalid precision must fail")
	}
}

func TestTradesDecode(t *testing.T) {
	s := svc(t, `[[452815892,1700000000000,-0.01,65000],[452815891,1699999999000,0.02,64990]]`)
	r, err := s.Trades(context.Background(), "tBTCUSD", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Data) != 2 || r.Data[0].Price.String() != "65000" || r.Data[0].Amount.String() != "-0.01" {
		t.Fatalf("bad trades: %+v", r.Data)
	}
}

func TestBadSymbolRejected(t *testing.T) {
	s := svc(t)
	for _, sym := range []string{"", "BTCUSD", "xBTCUSD", "tbtc usd"} {
		if _, err := s.Ticker(context.Background(), sym); err == nil {
			t.Errorf("symbol %q must fail", sym)
		}
	}
	var ve *types.ValidationError
	_ = ve
}

func TestCandlesLast(t *testing.T) {
	s := svc(t, `[1700000000000,64000,65000,65500,63500,12.5]`)
	r, err := s.Candles(context.Background(), "trade:1m:tBTCUSD", "last", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Data) != 1 || r.Data[0].Close.String() != "65000" {
		t.Fatalf("bad candle: %+v", r.Data)
	}
}
