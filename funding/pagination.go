package funding

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strconv"
)

// OffersHistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) OffersHistoryIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[FundingOffer], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[FundingOffer]{Maximum: 500, SupportsSort: false, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]FundingOffer], error) {
		return s.OffersHistory(ctx, symbol, q)
	}, Timestamp: func(v FundingOffer) types.Millis { return v.MTSUpdate }, Key: func(v FundingOffer) string { return strconv.FormatInt(v.ID, 10) }})
}

// LoansHistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) LoansHistoryIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[FundingLoan], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[FundingLoan]{Maximum: 500, SupportsSort: false, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]FundingLoan], error) {
		return s.LoansHistory(ctx, symbol, q)
	}, Timestamp: func(v FundingLoan) types.Millis { return v.MTSUpdate }, Key: func(v FundingLoan) string { return strconv.FormatInt(v.ID, 10) }})
}

// CreditsHistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) CreditsHistoryIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[FundingCredit], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[FundingCredit]{Maximum: 500, SupportsSort: false, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]FundingCredit], error) {
		return s.CreditsHistory(ctx, symbol, q)
	}, Timestamp: func(v FundingCredit) types.Millis { return v.MTSUpdate }, Key: func(v FundingCredit) string { return strconv.FormatInt(v.ID, 10) }})
}

// TradesHistoryIterator lazily traverses the documented history with inclusive boundary de-duplication.
func (s *Service) TradesHistoryIterator(symbol string, q *types.HistoryQuery) (*types.Iterator[FundingTrade], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[FundingTrade]{Maximum: 10000, SupportsSort: true, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]FundingTrade], error) {
		return s.TradesHistory(ctx, symbol, q)
	}, Timestamp: func(v FundingTrade) types.Millis { return v.MTSCreate }, Key: func(v FundingTrade) string { return strconv.FormatInt(v.ID, 10) }})
}
func historyBody(q *types.HistoryQuery) map[string]any { b := q.Body(); delete(b, "sort"); return b }
