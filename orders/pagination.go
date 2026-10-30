package orders

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strconv"
)

// HistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) HistoryIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[Order], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[Order]{Maximum: 2500, SupportsSort: false, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Order], error) {
		return s.History(ctx, symbol, q)
	}, Timestamp: func(v Order) types.Millis { return v.MTSCreate }, Key: func(v Order) string { return strconv.FormatInt(v.ID, 10) }})
}

// TradesIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) TradesIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[TradeExecution], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[TradeExecution]{Maximum: 10000, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]TradeExecution], error) {
		return s.Trades(ctx, symbol, q)
	}, Timestamp: func(v TradeExecution) types.Millis { return v.MTSCreate }, Key: func(v TradeExecution) string { return strconv.FormatInt(v.ID, 10) }})
}

// OTCHistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) OTCHistoryIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[OTCOrder], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[OTCOrder]{Maximum: 500, SupportsSort: false, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]OTCOrder], error) {
		return s.OTCHistoryTyped(ctx, symbol, q)
	}, Timestamp: func(v OTCOrder) types.Millis { return v.MTSCreate }, Key: func(v OTCOrder) string { return strconv.FormatInt(v.ID, 10) }})
}
func historyBody(q *types.HistoryQuery) map[string]any { b := q.Body(); delete(b, "sort"); return b }
