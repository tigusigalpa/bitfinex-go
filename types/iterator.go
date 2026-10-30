package types

import (
	"context"
	"fmt"
	"time"
)

// PaginationError means the server's timestamp-only cursor cannot prove that a
// full millisecond window was exhausted. No timestamp is skipped. Narrow the
// query or use another server-side filter; previously yielded data remains valid.
type PaginationError struct {
	Timestamp Millis
	Limit     int
}

func (e *PaginationError) Error() string {
	return fmt.Sprintf("bitfinex: history window at %d is saturated at limit %d", e.Timestamp, e.Limit)
}

// Iterator is a lazy, context-aware, single-consumer stream. Constructing it does
// no I/O. Next fetches only when the current page has been drained. Always check
// Err after Next returns false. An iterator must not be used concurrently.
type Iterator[T any] struct {
	fetch func(context.Context) ([]T, error)
	page  []T
	index int
	value T
	err   error
	done  bool
}

// NewIterator builds an iterator. A nil/empty page ends the stream.
func NewIterator[T any](fetch func(context.Context) ([]T, error)) *Iterator[T] {
	i := &Iterator[T]{fetch: fetch}
	if fetch == nil {
		i.err = &ValidationError{Field: "iterator", Message: "nil fetch function"}
		i.done = true
	}
	return i
}

// Next advances to one value, respecting cancellation even for buffered values.
func (i *Iterator[T]) Next(ctx context.Context) bool {
	if i.done {
		return false
	}
	if i.fetch == nil {
		i.err = &ValidationError{Field: "iterator", Message: "uninitialized iterator"}
		i.Close()
		return false
	}
	if err := ctx.Err(); err != nil {
		i.err = err
		i.Close()
		return false
	}
	if i.index >= len(i.page) {
		page, err := i.fetch(ctx)
		if err != nil {
			i.err = err
			i.Close()
			return false
		}
		if len(page) == 0 {
			i.Close()
			return false
		}
		i.page = page
		i.index = 0
	}
	i.value = i.page[i.index]
	i.index++
	return true
}

// Value returns the item from the most recent successful Next call.
func (i *Iterator[T]) Value() T { return i.value }

// Err returns a terminal fetch/protocol/cancellation error, or nil at EOF.
func (i *Iterator[T]) Err() error { return i.err }

// Close stops iteration and releases the current page. It does no I/O.
func (i *Iterator[T]) Close() { i.done = true; i.page = nil; i.fetch = nil }

// HistoryPagination describes the documented cursor and limit for one endpoint.
// Key identifies an observation (an ID, or ID+timestamp for repeated snapshots).
type HistoryPagination[T any] struct {
	Maximum      int
	SupportsSort bool
	Fetch        func(context.Context, *HistoryQuery) (*Response[[]T], error)
	Timestamp    func(T) Millis
	Key          func(T) string
}

// ValidateFor checks the endpoint-specific maximum and whether sort is accepted.
func (q *HistoryQuery) ValidateFor(maximum int, supportsSort bool) error {
	if err := q.Validate(); err != nil {
		return err
	}
	if q != nil && ((maximum > 0 && q.Limit > maximum) || (!supportsSort && q.Sort == SortAsc)) {
		return &ValidationError{Field: "history", Message: "unsupported limit or sort for this endpoint"}
	}
	return nil
}

// NewHistoryIterator pages inclusively, de-duplicates boundary observations, and
// grows full timestamp windows up to the endpoint maximum. It fails explicitly
// rather than skipping records when a millisecond remains saturated. End is
// frozen at construction when omitted, so ascending streams remain finite.
func NewHistoryIterator[T any](query *HistoryQuery, cfg HistoryPagination[T]) (*Iterator[T], error) {
	if cfg.Maximum < 1 || cfg.Fetch == nil || cfg.Timestamp == nil || cfg.Key == nil {
		return nil, &ValidationError{Field: "pagination", Message: "incomplete endpoint pagination configuration"}
	}
	if err := query.ValidateFor(cfg.Maximum, cfg.SupportsSort); err != nil {
		return nil, err
	}
	q := HistoryQuery{}
	if query != nil {
		q = *query
	}
	if q.Start != nil {
		v := *q.Start
		q.Start = &v
	}
	if q.End != nil {
		v := *q.End
		q.End = &v
	} else {
		v := Millis(time.Now().UnixMilli())
		q.End = &v
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	direction := q.Sort
	if direction == 0 {
		direction = SortDesc
	}
	if cfg.SupportsSort {
		q.Sort = direction
	} else {
		q.Sort = 0
	}
	if q.Limit == 0 {
		q.Limit = 100
		if q.Limit > cfg.Maximum {
			q.Limit = cfg.Maximum
		}
	}
	baseLimit := q.Limit
	seen := map[string]struct{}{}
	var boundary Millis
	haveBoundary, finished := false, false
	var terminal error
	return NewIterator(func(ctx context.Context) ([]T, error) {
		for !finished {
			if terminal != nil {
				return nil, terminal
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			request := q
			if q.Start != nil {
				v := *q.Start
				request.Start = &v
			}
			if q.End != nil {
				v := *q.End
				request.End = &v
			}
			resp, err := cfg.Fetch(ctx, &request)
			if err != nil {
				return nil, err
			}
			if resp == nil || len(resp.Data) > q.Limit {
				return nil, &ProtocolError{Message: "invalid history page size"}
			}
			rows := resp.Data
			if len(rows) == 0 {
				finished = true
				return nil, nil
			}
			out := make([]T, 0, len(rows))
			pageKeys := map[string]struct{}{}
			var previous Millis
			for n, row := range rows {
				ts, key := cfg.Timestamp(row), cfg.Key(row)
				if key == "" || ts < 0 || (q.Start != nil && ts < *q.Start) || (q.End != nil && ts > *q.End) {
					return nil, &ProtocolError{Message: "history row outside requested window or missing identity"}
				}
				if n > 0 && ((direction == SortAsc && ts < previous) || (direction == SortDesc && ts > previous)) {
					return nil, &ProtocolError{Message: "history page is not ordered by its documented timestamp"}
				}
				previous = ts
				if _, dup := pageKeys[key]; dup {
					continue
				}
				pageKeys[key] = struct{}{}
				if haveBoundary && ts == boundary {
					if _, dup := seen[key]; dup {
						continue
					}
				}
				out = append(out, row)
			}
			last := cfg.Timestamp(rows[len(rows)-1])
			if len(rows) < q.Limit {
				finished = true
				return out, nil
			}
			stalled := haveBoundary && last == boundary
			if !haveBoundary || last != boundary {
				seen = map[string]struct{}{}
			}
			for _, row := range rows {
				if cfg.Timestamp(row) == last {
					seen[cfg.Key(row)] = struct{}{}
				}
			}
			boundary = last
			haveBoundary = true
			if direction == SortAsc {
				v := last
				q.Start = &v
			} else {
				v := last
				q.End = &v
			}
			if stalled {
				if q.Limit == cfg.Maximum {
					if len(out) > 0 {
						terminal = &PaginationError{Timestamp: last, Limit: q.Limit}
						return out, nil
					}
					return nil, &PaginationError{Timestamp: last, Limit: q.Limit}
				}
				if q.Limit > cfg.Maximum/2 {
					q.Limit = cfg.Maximum
				} else {
					q.Limit *= 2
				}
			} else {
				q.Limit = baseLimit
			}
			if len(out) > 0 {
				return out, nil
			}
		}
		return nil, nil
	}), nil
}
