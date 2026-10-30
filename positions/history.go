package positions

import (
	"context"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"strconv"
)

func historyBody(q *types.HistoryQuery) map[string]any { m := q.Body(); delete(m, "sort"); return m }
func positionHistoryList(r *httpx.Result) (*types.Response[[]Position], error) {
	rows, e := wire.Rows(r.Data, "position history")
	if e != nil {
		return nil, e
	}
	out := make([]Position, 0, len(rows))
	for _, row := range rows {
		if e = wire.AssertMinLength(row, 14, "position history"); e != nil {
			return nil, e
		}
		for _, index := range []int{11, 12, 13} {
			if _, e = wire.MustInt(row, index, "position history identity/timestamp"); e != nil {
				return nil, e
			}
		}
		a := make([]any, 0, len(row)+1)
		a = append(a, row[:11]...)
		a = append(a, nil)
		a = append(a, row[11:]...)
		p, e := FromWire(a)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return types.NewResponse(out, httpx.Meta(r)), nil
}

// AuditRequest selects position IDs and a bounded history window.
type AuditRequest struct {
	IDs     []int64
	History *types.HistoryQuery
}

// AuditMany retrieves a documented audit ID array; ascending sort is unsupported.
func (s *Service) AuditMany(ctx context.Context, q AuditRequest) (*types.Response[[]Position], error) {
	if e := q.History.ValidateFor(500, false); e != nil {
		return nil, e
	}
	if len(q.IDs) == 0 {
		return nil, &types.ValidationError{Field: "id", Message: "position audit requires IDs"}
	}
	for _, id := range q.IDs {
		if id <= 0 {
			return nil, &types.ValidationError{Field: "id", Message: "positive position IDs required"}
		}
	}
	body := historyBody(q.History)
	body["id"] = append([]int64(nil), q.IDs...)
	r, e := s.t.PostAuth(ctx, "v2/auth/r/positions/audit", body, readBucket())
	if e != nil {
		return nil, e
	}
	return positionHistoryList(r)
}

// HistoryIterator traverses closed positions with overlap-safe observation identities.
func (s *Service) HistoryIterator(q *types.HistoryQuery) (*types.Iterator[Position], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[Position]{Maximum: 100, Fetch: s.History, Timestamp: func(v Position) types.Millis { return v.MTSUpdate }, Key: positionObservationKey})
}

// SnapshotIterator traverses historical position snapshots.
func (s *Service) SnapshotIterator(q *types.HistoryQuery) (*types.Iterator[Position], error) {
	return types.NewHistoryIterator(q, types.HistoryPagination[Position]{Maximum: 500, Fetch: s.Snapshot, Timestamp: func(v Position) types.Millis { return v.MTSUpdate }, Key: positionObservationKey})
}

// AuditIterator traverses a single position's observations without dropping updates.
func (s *Service) AuditIterator(id int64, q *types.HistoryQuery) (*types.Iterator[Position], error) {
	if id <= 0 {
		return nil, &types.ValidationError{Field: "id", Message: "positive position id required"}
	}
	return types.NewHistoryIterator(q, types.HistoryPagination[Position]{Maximum: 500, Fetch: func(ctx context.Context, q *types.HistoryQuery) (*types.Response[[]Position], error) {
		return s.Audit(ctx, id, q)
	}, Timestamp: func(v Position) types.Millis { return v.MTSUpdate }, Key: positionObservationKey})
}
func positionObservationKey(v Position) string {
	return strconv.FormatInt(v.ID, 10) + ":" + strconv.FormatInt(int64(v.MTSUpdate), 10)
}
