package market

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strconv"
)

// TickersHistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) TickersHistoryIterator(symbols []string, q *types.HistoryQuery) (*types.Iterator[Ticker], error) {
	symbols = append([]string(nil), symbols...)
	return types.NewHistoryIterator(q, types.HistoryPagination[Ticker]{Maximum: 250, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Ticker], error) {
		return s.TickersHistory(ctx, symbols, q)
	}, Timestamp: func(v Ticker) types.Millis { return v.MTS }, Key: func(v Ticker) string { return v.Symbol.String() + ":" + strconv.FormatInt(int64(v.MTS), 10) }})
}

// TradesIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) TradesIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[Trade], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[Trade]{Maximum: 10000, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Trade], error) {
		return s.Trades(ctx, symbol, q)
	}, Timestamp: func(v Trade) types.Millis { return v.MTS }, Key: func(v Trade) string { return strconv.FormatInt(v.ID, 10) }})
}

// StatsIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) StatsIterator(key, size, symbol, side string, q *types.HistoryQuery) (*types.Iterator[StatEntry], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[StatEntry]{Maximum: 10000, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]StatEntry], error) {
		return s.Stats(ctx, key, size, symbol, side, "hist", q)
	}, Timestamp: func(v StatEntry) types.Millis { return v.MTS }, Key: func(v StatEntry) string { return strconv.FormatInt(int64(v.MTS), 10) }})
}

// CandlesIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) CandlesIterator(candle string, q *types.HistoryQuery) (*types.Iterator[Candle], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[Candle]{Maximum: 10000, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Candle], error) {
		return s.Candles(ctx, candle, "hist", q)
	}, Timestamp: func(v Candle) types.Millis { return v.MTS }, Key: func(v Candle) string { return strconv.FormatInt(int64(v.MTS), 10) }})
}

// DerivativesStatusHistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) DerivativesStatusHistoryIterator(key string, q *types.HistoryQuery) (*types.Iterator[DerivativeStatus], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[DerivativeStatus]{Maximum: 5000, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]DerivativeStatus], error) {
		return s.DerivativesStatusHistory(ctx, key, q)
	}, Timestamp: func(v DerivativeStatus) types.Millis { return v.MTS }, Key: func(v DerivativeStatus) string { return strconv.FormatInt(int64(v.MTS), 10) }})
}

// LiquidationsIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) LiquidationsIterator(q *types.HistoryQuery) (*types.Iterator[Liquidation], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[Liquidation]{Maximum: 500, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Liquidation], error) {
		return s.Liquidations(ctx, q)
	}, Timestamp: func(v Liquidation) types.Millis { return v.MTS }, Key: func(v Liquidation) string {
		return strconv.FormatInt(v.PositionID, 10) + ":" + v.Timestamp.String() + ":" + v.Amount.String() + ":" + v.PriceAcquired.String() + ":" + strconv.FormatInt(v.IsMatch, 10) + ":" + strconv.FormatInt(v.IsMarketSold, 10)
	}})
}

// RankingsIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) RankingsIterator(key, timeframe, symbol string, q *types.HistoryQuery) (*types.Iterator[RankingRow], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[RankingRow]{Maximum: 10000, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]RankingRow], error) {
		return s.Rankings(ctx, key, timeframe, symbol, "hist", q)
	}, Timestamp: func(v RankingRow) types.Millis { return v.MTS }, Key: func(v RankingRow) string { return v.Username + ":" + strconv.FormatInt(int64(v.MTS), 10) }})
}

// FundingStatsIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) FundingStatsIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[FundingStatsEntry], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[FundingStatsEntry]{Maximum: 250, SupportsSort: false, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]FundingStatsEntry], error) {
		return s.FundingStats(ctx, symbol, q)
	}, Timestamp: func(v FundingStatsEntry) types.Millis { return v.MTS }, Key: func(v FundingStatsEntry) string { return strconv.FormatInt(int64(v.MTS), 10) }})
}
func historyParams(q *types.HistoryQuery) map[string]string {
	p := q.Params()
	delete(p, "sort")
	return p
}
