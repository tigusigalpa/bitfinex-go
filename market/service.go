package market

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Service exposes the public REST v2 endpoints. Constructed via
// (*bitfinex.Client).Market — not meant to be built directly.
type Service struct {
	t *httpx.Transport
}

// New binds a Service to a transport.
func New(t *httpx.Transport) *Service { return &Service{t: t} }

func readBucket(name string) httpx.RequestContext {
	return httpx.RequestContext{Bucket: name, Idempotent: true}
}

func listData(r *httpx.Result, what string) ([]any, error) {
	return wire.AsArr(r.Data, what)
}

// PlatformStatus returns GET /platform/status — 1 = operative,
// 0 = maintenance.
func (s *Service) PlatformStatus(ctx context.Context) (*types.Response[int], error) {
	r, err := s.t.GetPublic(ctx, "platform/status", nil, readBucket("platform-status"))
	if err != nil {
		return nil, err
	}
	rows, err := listData(r, "platform status")
	if err != nil {
		return nil, err
	}
	status := 0
	if len(rows) > 0 {
		if n, ok := rows[0].(interface{ String() string }); ok {
			if n.String() == "1" {
				status = 1
			}
		}
	}
	return types.NewResponse(status, httpx.Meta(r)), nil
}

// Tickers returns GET /tickers for the given symbols, or all
// tickers when symbols is empty (server "ALL").
func (s *Service) Tickers(ctx context.Context, symbols ...string) (*types.Response[[]Ticker], error) {
	param := "ALL"
	if len(symbols) > 0 {
		parts := make([]string, len(symbols))
		for i, raw := range symbols {
			sym, err := types.ParseSymbol(raw)
			if err != nil {
				return nil, err
			}
			parts[i] = sym.String()
		}
		param = strings.Join(parts, ",")
	}
	r, err := s.t.GetPublic(ctx, "tickers", map[string]string{"symbols": param}, readBucket("tickers"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "tickers")
	if err != nil {
		return nil, err
	}
	out := make([]Ticker, 0, len(rows))
	for _, row := range rows {
		if err := wire.AssertMinLength(row, 1, "ticker row"); err != nil {
			return nil, err
		}
		symStr, err := wire.Str(row, 0)
		if err != nil {
			return nil, err
		}
		tk, err := tickerFromWire(row[1:], types.Symbol(symStr))
		if err != nil {
			return nil, err
		}
		out = append(out, tk)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// Ticker returns GET /ticker/{symbol}.
func (s *Service) Ticker(ctx context.Context, symbol string) (*types.Response[Ticker], error) {
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.GetPublic(ctx, "ticker/"+sym.String(), nil, readBucket("ticker"))
	if err != nil {
		return nil, err
	}
	row, err := listData(r, "ticker")
	if err != nil {
		return nil, err
	}
	tk, err := tickerFromWire(row, sym)
	if err != nil {
		return nil, err
	}
	return types.NewResponse(tk, httpx.Meta(r)), nil
}

// TickersHistory returns GET /tickers/hist for given symbols.
func (s *Service) TickersHistory(ctx context.Context, symbols []string, q *types.HistoryQuery) (*types.Response[[]Ticker], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if q != nil && q.Limit > 250 {
		return nil, &types.ValidationError{Field: "limit", Message: "ticker history limit must not exceed 250"}
	}
	if len(symbols) == 0 {
		return nil, &types.ValidationError{Field: "symbols", Message: "at least one symbol required"}
	}
	parts := make([]string, len(symbols))
	for i, raw := range symbols {
		sym, err := types.ParseSymbol(raw)
		if err != nil {
			return nil, err
		}
		if !sym.IsTrading() {
			return nil, &types.ValidationError{Field: "symbol", Message: "ticker history supports trading symbols only"}
		}
		parts[i] = sym.String()
	}
	params := map[string]string{"symbols": strings.Join(parts, ",")}
	for k, v := range q.Params() {
		params[k] = v
	}
	r, err := s.t.GetPublic(ctx, "tickers/hist", params, readBucket("tickers"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "tickers/hist")
	if err != nil {
		return nil, err
	}
	out := make([]Ticker, 0, len(rows))
	for _, row := range rows {
		symStr, err := wire.Str(row, 0)
		if err != nil {
			return nil, err
		}
		if err := wire.AssertMinLength(row, 13, "ticker history"); err != nil {
			return nil, err
		}
		bid, ok, err := wire.Decimal(row, 1)
		if err != nil || !ok {
			return nil, &types.ProtocolError{Message: "ticker history requires bid"}
		}
		ask, ok, err := wire.Decimal(row, 3)
		if err != nil || !ok {
			return nil, &types.ProtocolError{Message: "ticker history requires ask"}
		}
		mts, err := wire.MustInt(row, 12, "ticker history")
		if err != nil {
			return nil, err
		}
		tk := Ticker{Symbol: types.Symbol(symStr), Bid: bid, Ask: ask, MTS: types.Millis(mts)}
		out = append(out, tk)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// Trades returns GET /trades/{symbol}/hist — newest first unless
// sort=1.
func (s *Service) Trades(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]Trade], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.GetPublic(ctx, "trades/"+sym.String()+"/hist", q.Params(), readBucket("trades"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "trades")
	if err != nil {
		return nil, err
	}
	out := make([]Trade, 0, len(rows))
	for _, row := range rows {
		tr, err := tradeFromWire(row, sym)
		if err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

var bookPrecisionRe = regexp.MustCompile(`^(P[0-4]|R0)$`)

// Book returns GET /book/{symbol}/{precision} — P0–P4 aggregated
// or R0 raw. The result discriminates per level: aggregated
// trading → Entry, raw → Raw, funding → Funding.
func (s *Service) Book(ctx context.Context, symbol, precision string, q *types.HistoryQuery) (*types.Response[[]BookLevel], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	prec := strings.ToUpper(precision)
	if !bookPrecisionRe.MatchString(prec) {
		return nil, &types.ValidationError{Field: "precision", Message: "book precision must be P0–P4 or R0"}
	}
	params := map[string]string{}
	if q != nil && (q.Start != nil || q.End != nil || q.Sort != 0) {
		return nil, &types.ValidationError{Field: "book", Message: "book takes a length, not a history window"}
	}
	if q != nil && q.Limit != 0 {
		switch q.Limit {
		case 1, 25, 100, 250:
		default:
			return nil, &types.ValidationError{Field: "len", Message: "book length must be 1, 25, 100 or 250"}
		}
		params["len"] = fmt.Sprint(q.Limit)
	}
	r, err := s.t.GetPublic(ctx, "book/"+sym.String()+"/"+prec, params, readBucket("book"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "book")
	if err != nil {
		return nil, err
	}
	out := make([]BookLevel, 0, len(rows))
	for _, row := range rows {
		var lvl BookLevel
		switch {
		case prec == "R0" && sym.IsFunding():
			e, derr := RawFundingBookFromWire(row)
			if derr != nil {
				return nil, derr
			}
			lvl.RawFunding = &e
		case prec == "R0":
			e, derr := rawBookEntryFromWire(row)
			if derr != nil {
				return nil, derr
			}
			lvl.Raw = &e
		case sym.IsFunding():
			e, derr := fundingBookEntryFromWire(row)
			if derr != nil {
				return nil, derr
			}
			lvl.Funding = &e
		default:
			e, derr := bookEntryFromWire(row)
			if derr != nil {
				return nil, derr
			}
			lvl.Entry = &e
		}
		out = append(out, lvl)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// Stats returns GET /stats1/{key}:{size}:{symbol}:{side}/{section}
// e.g. key "pos.size", size "1m", symbol "tBTCUSD", side "long",
// section "last" or "hist".
func (s *Service) Stats(ctx context.Context, key, size, symbol, side, section string, q *types.HistoryQuery) (*types.Response[[]StatEntry], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("stats1/%s:%s:%s:%s/%s",
		escapeSeg(key), escapeSeg(size), sym.String(), escapeSeg(side), escapeSeg(section))
	r, err := s.t.GetPublic(ctx, path, q.Params(), readBucket("stats"))
	if err != nil {
		return nil, err
	}
	rows, err := listData(r, "stats")
	if err != nil {
		return nil, err
	}
	out := []StatEntry{}
	// "last" returns a single [MTS, VALUE]; "hist" a list.
	if len(rows) > 0 {
		if _, isArr := rows[0].([]any); !isArr {
			e, derr := statEntryFromWire(rows)
			if derr != nil {
				return nil, derr
			}
			return types.NewResponse([]StatEntry{e}, httpx.Meta(r)), nil
		}
	}
	for _, row := range rows {
		arr, ok := row.([]any)
		if !ok {
			return nil, &types.ProtocolError{Message: "expected positional row"}
		}
		e, derr := statEntryFromWire(arr)
		if derr != nil {
			return nil, derr
		}
		out = append(out, e)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

var candleRe = regexp.MustCompile(`^(trade|funding):[0-9]+[mhdDWM]+:`)

// Candles returns GET /candles/{candle}/{section}. The candle
// key looks like "trade:1m:tBTCUSD", "trade:1h:tBTCF0:USTF0" or
// "trade:1m:fUSD:p30". For section "last" the server returns a
// single candle row, decoded into a one-element slice.
func (s *Service) Candles(ctx context.Context, candle, section string, q *types.HistoryQuery) (*types.Response[[]Candle], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if !candleRe.MatchString(candle) {
		return nil, &types.ValidationError{Field: "candle", Message: `must look like "trade:1m:tBTCUSD"`}
	}
	if section == "" {
		section = "hist"
	}
	r, err := s.t.GetPublic(ctx, "candles/"+url.PathEscape(candle)+"/"+escapeSeg(section), q.Params(), readBucket("candles"))
	if err != nil {
		return nil, err
	}
	rows, err := listData(r, "candles")
	if err != nil {
		return nil, err
	}
	if section == "last" && len(rows) > 0 {
		if _, isArr := rows[0].([]any); !isArr {
			c, derr := candleFromWire(rows)
			if derr != nil {
				return nil, derr
			}
			return types.NewResponse([]Candle{c}, httpx.Meta(r)), nil
		}
	}
	out := make([]Candle, 0, len(rows))
	for _, row := range rows {
		arr, ok := row.([]any)
		if !ok {
			return nil, &types.ProtocolError{Message: "expected positional row"}
		}
		c, derr := candleFromWire(arr)
		if derr != nil {
			return nil, derr
		}
		out = append(out, c)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// Configs returns GET /conf/pub:{action}:{object}[:{detail}] —
// symbol maps, pair lists, currency labels. The response shape
// varies by action; returned as decoded wire data.
func (s *Service) Configs(ctx context.Context, action, object, detail string) (*types.Response[any], error) {
	path := "conf/pub:" + escapeSeg(action) + ":" + escapeSeg(object)
	if detail != "" {
		path += ":" + escapeSeg(detail)
	}
	r, err := s.t.GetPublic(ctx, path, nil, readBucket("conf"))
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// ListedPairs returns the trading pair list (conf pub:list:pair:exchange).
func (s *Service) ListedPairs(ctx context.Context) (*types.Response[any], error) {
	return s.Configs(ctx, "list", "pair", "exchange")
}

// ListedFundingCurrencies returns the funding currency list
// (conf pub:list:currency).
func (s *Service) ListedFundingCurrencies(ctx context.Context) (*types.Response[any], error) {
	return s.Configs(ctx, "list", "currency", "")
}

// DerivativesStatus returns GET /status/deriv for the given keys
// (e.g. "tBTCF0:USTF0"), or all when keys is empty.
func (s *Service) DerivativesStatus(ctx context.Context, keys ...string) (*types.Response[[]DerivativeStatus], error) {
	param := "ALL"
	if len(keys) > 0 {
		param = strings.Join(keys, ",")
	}
	r, err := s.t.GetPublic(ctx, "status/deriv", map[string]string{"keys": param}, readBucket("public"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "derivatives status")
	if err != nil {
		return nil, err
	}
	out := make([]DerivativeStatus, 0, len(rows))
	for _, row := range rows {
		d, derr := derivStatusFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, d)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// DerivativesStatusHistory returns GET /status/deriv/{key}/hist.
func (s *Service) DerivativesStatusHistory(ctx context.Context, key string, q *types.HistoryQuery) (*types.Response[[]DerivativeStatus], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.GetPublic(ctx, "status/deriv/"+escapeSeg(key)+"/hist", q.Params(), readBucket("public"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "derivatives status history")
	if err != nil {
		return nil, err
	}
	out := make([]DerivativeStatus, 0, len(rows))
	for _, row := range rows {
		d, derr := derivStatusFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, d)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// Liquidations returns GET /liquidations/hist.
func (s *Service) Liquidations(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Liquidation], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	r, err := s.t.GetPublic(ctx, "liquidations/hist", q.Params(), readBucket("public"))
	if err != nil {
		return nil, err
	}
	outer, err := listData(r, "liquidations")
	if err != nil {
		return nil, err
	}
	out := []Liquidation{}
	// payload wraps rows in an extra list level
	for _, o := range outer {
		inner, ok := o.([]any)
		if !ok {
			return nil, &types.ProtocolError{Message: "expected positional row"}
		}
		for _, row := range inner {
			arr, ok := row.([]any)
			if !ok {
				continue
			}
			l, derr := liquidationFromWire(arr)
			if derr != nil {
				return nil, derr
			}
			out = append(out, l)
		}
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// Rankings returns GET /rankings/{key}:{timeframe}:{symbol}/{section}
// — leaderboard rows. key e.g. "plu_diff", timeframe "1D"/"1W"/"1M",
// section "hist" or "last".
func (s *Service) Rankings(ctx context.Context, key, timeframe, symbol, section string, q *types.HistoryQuery) (*types.Response[[]RankingRow], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("rankings/%s:%s:%s/%s",
		escapeSeg(key), escapeSeg(timeframe), sym.String(), escapeSeg(section))
	r, err := s.t.GetPublic(ctx, path, q.Params(), readBucket("public"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "rankings")
	if err != nil {
		return nil, err
	}
	out := make([]RankingRow, 0, len(rows))
	for _, row := range rows {
		rr := RankingRow{}
		if mts, _, merr := wire.Mts(row, 0); merr != nil {
			return nil, merr
		} else {
			rr.MTS = mts
		}
		rr.Username = wire.StrOr(row, 2, "")
		if len(row) > 3 {
			rr.Extra = append([]any(nil), row[3:]...)
		}
		out = append(out, rr)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// FundingStats returns GET /funding/stats/{symbol}/hist.
func (s *Service) FundingStats(ctx context.Context, symbol string, q *types.HistoryQuery) (*types.Response[[]FundingStatsEntry], error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	sym, err := types.ParseSymbol(symbol)
	if err != nil {
		return nil, err
	}
	r, err := s.t.GetPublic(ctx, "funding/stats/"+sym.String()+"/hist", q.Params(), readBucket("public"))
	if err != nil {
		return nil, err
	}
	rows, err := wire.Rows(r.Data, "funding stats")
	if err != nil {
		return nil, err
	}
	out := make([]FundingStatsEntry, 0, len(rows))
	for _, row := range rows {
		e, derr := fundingStatsFromWire(row)
		if derr != nil {
			return nil, derr
		}
		out = append(out, e)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// VASPs returns GET /ext/vasps — the registry of virtual asset
// service providers (shape varies; returned as decoded wire data).
func (s *Service) VASPs(ctx context.Context) (*types.Response[any], error) {
	r, err := s.t.GetPublic(ctx, "ext/vasps", nil, readBucket("public"))
	if err != nil {
		return nil, err
	}
	return types.NewResponse(r.Data, httpx.Meta(r)), nil
}

// MarketAveragePrice POSTs /calc/trade/avg — the hypothetical
// average fill price for amount on the given symbols.
func (s *Service) MarketAveragePrice(ctx context.Context, symbols []string, amount types.Decimal) (*types.Response[MarketAvgPrice], error) {
	if !amount.IsValid() || amount.IsZero() {
		return nil, &types.ValidationError{Field: "amount", Message: "market average price amount must be non-zero"}
	}
	if len(symbols) == 0 {
		return nil, &types.ValidationError{Field: "symbols", Message: "at least one symbol required"}
	}
	parts := make([]string, len(symbols))
	for i, raw := range symbols {
		sym, err := types.ParseSymbol(raw)
		if err != nil {
			return nil, err
		}
		parts[i] = sym.String()
	}
	r, err := s.t.PostPublic(ctx, "calc/trade/avg", map[string]any{
		"symbol": strings.Join(parts, ","),
		"amount": amount.CanonicalString(),
	}, httpx.RequestContext{Bucket: "calc", Idempotent: true})
	if err != nil {
		return nil, err
	}
	row, err := listData(r, "calc/trade/avg")
	if err != nil {
		return nil, err
	}
	res := MarketAvgPrice{}
	if d, _, derr := wire.Decimal(row, 0); derr != nil {
		return nil, derr
	} else {
		res.PriceAvg = d
	}
	if d, _, derr := wire.Decimal(row, 1); derr != nil {
		return nil, derr
	} else {
		res.Amount = d
	}
	return types.NewResponse(res, httpx.Meta(r)), nil
}

// ForeignExchangeRate POSTs /calc/fx — indicative conversion
// rate ccy1→ccy2.
func (s *Service) ForeignExchangeRate(ctx context.Context, ccy1, ccy2 string) (*types.Response[types.Decimal], error) {
	if strings.TrimSpace(ccy1) == "" || strings.TrimSpace(ccy2) == "" {
		return nil, &types.ValidationError{Field: "ccy", Message: "foreign exchange rate requires two currency codes"}
	}
	r, err := s.t.PostPublic(ctx, "calc/fx", map[string]any{
		"ccy1": strings.ToUpper(strings.TrimSpace(ccy1)),
		"ccy2": strings.ToUpper(strings.TrimSpace(ccy2)),
	}, httpx.RequestContext{Bucket: "calc", Idempotent: true})
	if err != nil {
		return nil, err
	}
	row, err := listData(r, "calc/fx")
	if err != nil {
		return nil, err
	}
	d, ok, derr := wire.Decimal(row, 0)
	if derr != nil {
		return nil, derr
	}
	if !ok {
		return nil, &types.ProtocolError{Message: "calc/fx returned no rate"}
	}
	return types.NewResponse(d, httpx.Meta(r)), nil
}

func escapeSeg(s string) string { return url.PathEscape(s) }
