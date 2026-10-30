package types

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

type historyItem struct {
	ID int
	TS Millis
}

func TestHistoryIteratorBoundaries(t *testing.T) {
	for _, sort := range []int{SortDesc, SortAsc} {
		t.Run(strconv.Itoa(sort), func(t *testing.T) {
			rows := []historyItem{{1, 10}, {2, 10}, {3, 10}, {4, 9}, {5, 8}, {6, 8}, {7, 7}}
			if sort == SortAsc {
				for l, r := 0, len(rows)-1; l < r; l, r = l+1, r-1 {
					rows[l], rows[r] = rows[r], rows[l]
				}
			}
			start, end := Millis(7), Millis(10)
			q := &HistoryQuery{Start: &start, End: &end, Limit: 2, Sort: sort}
			calls := 0
			grew := false
			it, err := NewHistoryIterator(q, HistoryPagination[historyItem]{Maximum: 5, SupportsSort: true, Timestamp: func(v historyItem) Millis { return v.TS }, Key: func(v historyItem) string { return strconv.Itoa(v.ID) }, Fetch: func(ctx context.Context, q *HistoryQuery) (*Response[[]historyItem], error) {
				calls++
				if q.Limit > 2 {
					grew = true
				}
				var page []historyItem
				for _, v := range rows {
					if v.TS >= *q.Start && v.TS <= *q.End {
						page = append(page, v)
						if len(page) == q.Limit {
							break
						}
					}
				}
				return NewResponse(page, ResponseMeta{}), nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatal("constructor made I/O")
			}
			start = 0
			end = 0
			q.Limit = 5
			var got []historyItem
			for it.Next(context.Background()) {
				got = append(got, it.Value())
				if len(got) > 20 {
					t.Fatal("unbounded iteration")
				}
			}
			if it.Err() != nil {
				t.Fatal(it.Err())
			}
			if !reflect.DeepEqual(got, rows) || !grew {
				t.Fatalf("got %v, grew=%v; want %v", got, grew, rows)
			}
		})
	}
}

func TestHistoryIteratorSaturationAndInvalidPages(t *testing.T) {
	cases := []struct {
		name                string
		pages               [][]historyItem
		protocol, saturated bool
	}{
		{"saturated", [][]historyItem{{{1, 5}, {2, 5}}, {{1, 5}, {3, 5}}}, false, true},
		{"outside", [][]historyItem{{{1, 11}}}, true, false},
		{"unordered", [][]historyItem{{{1, 4}, {2, 5}}}, true, false},
		{"oversized", [][]historyItem{{{1, 5}, {2, 4}, {3, 3}}}, true, false},
		{"short dedup", [][]historyItem{{{1, 5}, {1, 5}}, {{1, 5}}}, false, false},
		{"empty", [][]historyItem{nil}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			end := Millis(10)
			n := 0
			it, e := NewHistoryIterator(&HistoryQuery{End: &end, Limit: 2}, HistoryPagination[historyItem]{Maximum: 2, Timestamp: func(v historyItem) Millis { return v.TS }, Key: func(v historyItem) string { return strconv.Itoa(v.ID) }, Fetch: func(context.Context, *HistoryQuery) (*Response[[]historyItem], error) {
				if n >= len(tc.pages) {
					t.Fatal("unexpected extra fetch")
				}
				p := tc.pages[n]
				n++
				return NewResponse(p, ResponseMeta{}), nil
			}})
			if e != nil {
				t.Fatal(e)
			}
			var ids []int
			for it.Next(context.Background()) {
				ids = append(ids, it.Value().ID)
			}
			var pe *ProtocolError
			var se *PaginationError
			if errors.As(it.Err(), &pe) != tc.protocol || errors.As(it.Err(), &se) != tc.saturated {
				t.Fatalf("error %v", it.Err())
			}
			if tc.saturated && !reflect.DeepEqual(ids, []int{1, 2, 3}) {
				t.Fatalf("lost valid rows: %v", ids)
			}
		})
	}
}

func TestIteratorLifecycle(t *testing.T) {
	ctx := context.Background()
	calls := 0
	sentinel := errors.New("fetch failure")
	it := NewIterator(func(context.Context) ([]int, error) {
		calls++
		if calls == 1 {
			return []int{1, 2}, nil
		}
		return nil, sentinel
	})
	if !it.Next(ctx) || it.Value() != 1 {
		t.Fatal("first")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if it.Next(cancelled) || !errors.Is(it.Err(), context.Canceled) || calls != 1 {
		t.Fatal("buffer ignored cancellation")
	}
	it = NewIterator(func(context.Context) ([]int, error) { return nil, sentinel })
	if it.Next(ctx) || !errors.Is(it.Err(), sentinel) {
		t.Fatal(it.Err())
	}
	it = NewIterator(func(context.Context) ([]int, error) { t.Fatal("closed fetched"); return nil, nil })
	it.Close()
	if it.Next(ctx) || it.Err() != nil {
		t.Fatal("closed")
	}
	var zero Iterator[int]
	if zero.Next(ctx) || zero.Err() == nil {
		t.Fatal("zero iterator")
	}
	if NewIterator[int](nil).Err() == nil {
		t.Fatal("nil fetch")
	}
	cfg := HistoryPagination[int]{Maximum: 10, Timestamp: func(int) Millis { return 0 }, Key: strconv.Itoa, Fetch: func(context.Context, *HistoryQuery) (*Response[[]int], error) { return nil, nil }}
	if _, e := NewHistoryIterator(&HistoryQuery{Sort: SortAsc}, cfg); e == nil {
		t.Fatal("unsupported ascending")
	}
	if _, e := NewHistoryIterator(&HistoryQuery{Limit: 11}, cfg); e == nil {
		t.Fatal("oversized limit")
	}
	if _, e := NewHistoryIterator[int](nil, HistoryPagination[int]{}); e == nil {
		t.Fatal("invalid cfg")
	}
	it2, e := NewHistoryIterator[int](nil, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if it2.Next(ctx) || it2.Err() == nil {
		t.Fatal("nil response")
	}
}
