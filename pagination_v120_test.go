package bitfinex_test

import (
	"context"
	"encoding/json"
	"fmt"
	bitfinex "github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/types"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func consumeHistory[T any](it *types.Iterator[T], e error) (int, error) {
	if e != nil {
		return 0, e
	}
	defer it.Close()
	n := 0
	for it.Next(context.Background()) {
		n++
		if n > 10 {
			return n, fmt.Errorf("unbounded iterator")
		}
	}
	return n, it.Err()
}
func TestHistoryEndpointIterators(t *testing.T) {
	cases := []struct {
		name, path, row string
		ts, id          int
		sort, wrap      bool
		call            func(*bitfinex.Client, *types.HistoryQuery) (int, error)
	}{
		{"tickers", "tickers/hist", "[\"tBTCUSD\",100,null,101,null,null,null,null,null,null,null,null,100]", 12, -1, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.TickersHistoryIterator([]string{"tBTCUSD"}, q))
		}},
		{"trades", "trades/tBTCUSD/hist", "[1,100,1,100]", 1, 0, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.TradesIterator("tBTCUSD", q))
		}},
		{"stats", "stats1/pos.size:1m:tBTCUSD:long/hist", "[100,1]", 0, -1, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.StatsIterator("pos.size", "1m", "tBTCUSD", "long", q))
		}},
		{"candles", "candles/trade:1m:tBTCUSD/hist", "[100,1,1,1,1,1]", 0, -1, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.CandlesIterator("trade:1m:tBTCUSD", q))
		}},
		{"derivatives", "status/deriv/tBTCF0:USTF0/hist", "[100,null,101,102,null,103,null,200,0.1,0.2,null,0.3,null,null,104,null,null,105,null,null,null,0.4,0.5]", 0, -1, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.DerivativesStatusHistoryIterator("tBTCF0:USTF0", q))
		}},
		{"liquidations", "liquidations/hist", "[\"pos\",1,100,null,\"tBTCUSD\",1,100,null,1,0,null,99]", 2, 1, true, true, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.LiquidationsIterator(q))
		}},
		{"rankings", "rankings/plu_diff:1D:tBTCUSD/hist", "[100,null,\"fixture\",1,null,null,100,null,null,\"\"]", 0, -1, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.RankingsIterator("plu_diff", "1D", "tBTCUSD", q))
		}},
		{"funding-stats", "funding/stats/fUSD/hist", "[100,null,null,0.001,2,null,null,1000,900,null,null,12]", 0, -1, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Market.FundingStatsIterator("fUSD", q))
		}},
		{"orders", "auth/r/orders/hist", "[1,0,7,\"tBTCUSD\",100,101,1,1,\"EXCHANGE LIMIT\",null,null,null,0,\"ACTIVE\",null,null,100,100,0,0,null,null,null,0,0]", 4, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Orders.HistoryIterator("", q))
		}},
		{"own-trades", "auth/r/trades/tBTCUSD/hist", "[1,\"tBTCUSD\",100,42,1,100,\"LIMIT\",100,1,0.01,\"USD\"]", 2, 0, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Orders.TradesIterator("tBTCUSD", q))
		}},
		{"otc", "auth/r/orders/otc/tBTCUSD/hist", "[1,\"tBTCUSD\",100,101,null,1,\"alice\",\"bob\",null,1,100,null,\"EXECUTED\"]", 2, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Orders.OTCHistoryIterator("tBTCUSD", q))
		}},
		{"positions", "auth/r/positions/hist", "[\"tBTCUSD\",\"CLOSED\",1,100,0,0,null,null,null,null,null,1,99,100,null,0,null,1,1,{}]", 13, -1, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Positions.HistoryIterator(q))
		}},
		{"snapshots", "auth/r/positions/snap", "[\"tBTCUSD\",\"CLOSED\",1,100,0,0,null,null,null,null,null,1,99,100,null,0,null,1,1,{}]", 13, -1, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Positions.SnapshotIterator(q))
		}},
		{"audit", "auth/r/positions/audit", "[\"tBTCUSD\",\"CLOSED\",1,100,0,0,null,null,null,null,null,1,99,100,null,0,null,1,1,{}]", 13, -1, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Positions.AuditIterator(1, q))
		}},
		{"funding-offers", "auth/r/funding/offers/fUSD/hist", "[1,\"fUSD\",99,100,100,100,\"LIMIT\",null,null,0,\"EXECUTED\",null,null,null,0.001,2,0,0,null,0]", 3, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Funding.OffersHistoryIterator("fUSD", q))
		}},
		{"funding-loans", "auth/r/funding/loans/fUSD/hist", "[1,\"fUSD\",1,99,100,100,0,\"CLOSED\",null,null,null,0.001,2,99,100,null,null,null,0,0.001]", 4, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Funding.LoansHistoryIterator("fUSD", q))
		}},
		{"funding-credits", "auth/r/funding/credits/fUSD/hist", "[1,\"fUSD\",1,99,100,100,0,\"CLOSED\",null,null,null,0.001,2,99,100,null,null,null,0,0.001]", 4, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Funding.CreditsHistoryIterator("fUSD", q))
		}},
		{"funding-trades", "auth/r/funding/trades/fUSD/hist", "[1,\"fUSD\",100,42,100,0.001,2,1]", 2, 0, true, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Funding.TradesHistoryIterator("fUSD", q))
		}},
		{"ledgers", "auth/r/ledgers/USD/hist", "[1,\"USD\",null,100,null,1,100,null,\"fixture\"]", 3, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Account.LedgersIterator("USD", q))
		}},
		{"logins", "auth/r/logins/hist", "[1,null,100,null,\"192.0.2.1\",null,null,\"{}\"]", 2, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Account.LoginHistoryIterator(q))
		}},
		{"account-audit", "auth/r/audit/hist", "[100,null,\"changed\",null,null,\"192.0.2.1\",\"fixture\"]", 0, -1, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Account.AuditHistoryIterator(q))
		}},
		{"movements", "auth/r/movements/USD/hist", "[1,\"USD\",\"wire\",null,null,99,100,null,null,\"COMPLETED\",null,null,1,0.01,null,null,\"dest\",null,null,null,\"tx\"]", 6, 0, false, false, func(c *bitfinex.Client, q *types.HistoryQuery) (int, error) {
			return consumeHistory(c.Account.MovementsIterator("USD", q))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v2/"+tc.path {
					t.Errorf("path %s", r.URL.Path)
				}
				params := map[string]string{}
				for k, v := range r.URL.Query() {
					params[k] = v[0]
				}
				if strings.HasPrefix(tc.path, "auth/") {
					b, _ := io.ReadAll(r.Body)
					m := jsonTree(t, b).(map[string]any)
					for k, v := range m {
						if n, ok := v.(json.Number); ok {
							params[k] = n.String()
						}
					}
					if tc.name == "audit" {
						a := m["id"].([]any)
						if len(a) != 1 || a[0] != json.Number("1") {
							t.Error("audit id must be array")
						}
					}
				}
				if tc.sort && params["sort"] != "-1" {
					t.Error("missing descending sort")
				}
				if !tc.sort && params["sort"] != "" {
					t.Error("unsupported sort sent")
				}
				end, _ := strconv.Atoi(params["end"])
				limit, _ := strconv.Atoi(params["limit"])
				if params["start"] != "0" {
					t.Error("lost start")
				}
				if calls == 1 && end != 200 {
					t.Error("lost initial end")
				}
				if calls > 1 && end != 100 && end != 99 {
					t.Errorf("wrong cursor %d", end)
				}
				if limit != 1 && limit != 2 {
					t.Error("unexpected window growth")
				}
				a := jsonTree(t, []byte(tc.row)).([]any)
				b := append([]any(nil), a...)
				b[tc.ts] = json.Number("99")
				if tc.id >= 0 {
					b[tc.id] = json.Number("2")
				}
				rows := []any{}
				if end >= 100 {
					rows = append(rows, a)
				}
				if len(rows) < limit && end >= 99 {
					rows = append(rows, b)
				}
				var out any = rows
				if tc.wrap {
					out = []any{rows}
				}
				json.NewEncoder(w).Encode(out)
			}))
			defer srv.Close()
			c, e := bitfinex.NewClient(bitfinex.WithPublicRESTBase(srv.URL+"/v2"), bitfinex.WithPrivateRESTBase(srv.URL+"/v2"), bitfinex.WithCredentials("fixture-key", "fixture-secret"), bitfinex.WithRateLimiter(unlimited{}), bitfinex.WithRetryPolicy(1, time.Millisecond, time.Millisecond, time.Second))
			if e != nil {
				t.Fatal(e)
			}
			start, end := types.Millis(0), types.Millis(200)
			n, e := tc.call(c, &types.HistoryQuery{Start: &start, End: &end, Limit: 1})
			if e != nil || n != 2 || calls != 5 {
				t.Fatalf("rows=%d calls=%d error=%v", n, calls, e)
			}
		})
	}
}
