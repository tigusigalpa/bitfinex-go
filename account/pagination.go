package account

import (
	"context"
	"encoding/json"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strconv"
)

// LedgersIterator traverses history with documented limits and overlap-safe identities.
func (s *Service) LedgersIterator(currency string, q *types.HistoryQuery) (*types.Iterator[LedgerEntry], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[LedgerEntry]{Maximum: 2500, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]LedgerEntry], error) {
		return s.Ledgers(ctx, currency, q)
	}, Timestamp: func(v LedgerEntry) types.Millis { return v.MTS }, Key: func(v LedgerEntry) string { return strconv.FormatInt(v.ID, 10) }})
}

// LoginHistoryIterator traverses history with documented limits and overlap-safe identities.
func (s *Service) LoginHistoryIterator(q *types.HistoryQuery) (*types.Iterator[LoginEntry], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[LoginEntry]{Maximum: 250, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]LoginEntry], error) {
		return s.LoginHistory(ctx, q)
	}, Timestamp: func(v LoginEntry) types.Millis { return v.MTS }, Key: func(v LoginEntry) string { return strconv.FormatInt(v.ID, 10) }})
}

// AuditHistoryIterator traverses history with documented limits and overlap-safe identities.
func (s *Service) AuditHistoryIterator(q *types.HistoryQuery) (*types.Iterator[AuditEntry], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[AuditEntry]{Maximum: 500, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]AuditEntry], error) {
		return s.AuditHistoryTyped(ctx, q)
	}, Timestamp: func(v AuditEntry) types.Millis { return v.MTS }, Key: func(v AuditEntry) string { return auditObservationKey(v) }})
}

// MovementsIterator traverses history with documented limits and overlap-safe identities.
func (s *Service) MovementsIterator(currency string, q *types.HistoryQuery) (*types.Iterator[Movement], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[Movement]{Maximum: 1000, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Movement], error) {
		return s.Movements(ctx, currency, q)
	}, Timestamp: func(v Movement) types.Millis { return v.MTSUpdated }, Key: func(v Movement) string { return strconv.FormatInt(v.ID, 10) }})
}
func historyBody(q *types.HistoryQuery) map[string]any { b := q.Body(); delete(b, "sort"); return b }
func auditObservationKey(v AuditEntry) string          { b, _ := json.Marshal(v); return string(b) }
