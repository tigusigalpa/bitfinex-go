package market

import (
	"context"
	"errors"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/retry"
	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
	"strings"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestBookLengthQuery(t *testing.T) {
	d := &testutil.FakeDoer{}
	s := New(httpx.New(httpx.Config{Doer: d, PublicBase: "https://example.invalid/v2", Retry: retry.NewPolicy(1, time.Millisecond, time.Millisecond, time.Second)}))
	_, _ = s.Book(context.Background(), "tBTCUSD", "P0", &types.HistoryQuery{Limit: 25})
	if len(d.Requests) != 1 || !strings.HasSuffix(d.Requests[0].URL, "?len=25") {
		t.Fatal("book length was not sent as len")
	}
}

func TestTickerHistoryUsesDocumentedIndexes(t *testing.T) {
	s := svc(t, `[["tBTCUSD",100,null,101,null,null,null,null,null,null,null,null,1700000000000,"future-field"]]`)
	r, err := s.TickersHistory(context.Background(), []string{"tBTCUSD"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Data) != 1 || r.Data[0].Bid.String() != "100" || r.Data[0].Ask.String() != "101" || r.Data[0].MTS != 1700000000000 || r.Data[0].LastPrice.IsValid() {
		t.Fatal("history parsed as current ticker")
	}
	for _, body := range []string{`[[]]`, `[["tBTCUSD"]]`, `[["tBTCUSD",true,null,101,null,null,null,null,null,null,null,null,1700000000000]]`} {
		_, err := svc(t, body).TickersHistory(context.Background(), []string{"tBTCUSD"}, nil)
		var pe *types.ProtocolError
		if !errors.As(err, &pe) {
			t.Fatalf("malformed history accepted: %v", err)
		}
	}
}

func TestRawFundingBookContract(t *testing.T) {
	r, err := svc(t, `[[42,30,0.0001,-100]]`).Book(context.Background(), "fUSD", "R0", &types.HistoryQuery{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	l := r.Data[0].RawFunding
	if l == nil || l.OfferID != 42 || l.Period != 30 || l.Rate.String() != "0.0001" || l.Amount.String() != "-100" || r.Data[0].Raw != nil {
		t.Fatal("raw funding decoded as trading book")
	}
}

func TestMalformedListsDoNotPanic(t *testing.T) {
	ctx := context.Background()
	for _, body := range []string{`[[]]`, `[null]`, `[false]`, `[[true]]`} {
		for _, f := range []func(*Service) error{
			func(s *Service) error { _, e := s.Tickers(ctx); return e },
			func(s *Service) error { _, e := s.Trades(ctx, "tBTCUSD", nil); return e },
			func(s *Service) error { _, e := s.Book(ctx, "tBTCUSD", "P0", nil); return e },
			func(s *Service) error { _, e := s.Candles(ctx, "trade:1m:tBTCUSD", "hist", nil); return e },
		} {
			if err := f(svc(t, body)); err == nil {
				t.Fatalf("malformed list accepted: %s", body)
			}
		}
	}
}
