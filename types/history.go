package types

import "strconv"

// Sort order values accepted by Bitfinex history endpoints.
const (
	SortAsc  = 1
	SortDesc = -1
)

// HistoryQuery is the shared start/end/limit/sort window for
// history endpoints. Zero fields are omitted from the request.
//
// Server-side retention (documented): order history ~2 weeks,
// order trades ~10 days, funding loan/credit history ~3 days,
// ledgers up to ~6 years.
type HistoryQuery struct {
	Start *Millis `json:"start,omitempty"`
	End   *Millis `json:"end,omitempty"`
	Limit int     `json:"limit,omitempty"`
	Sort  int     `json:"sort,omitempty"` // SortAsc or SortDesc
}

// Params renders the query as URL parameters.
func (q *HistoryQuery) Params() map[string]string {
	m := map[string]string{}
	if q == nil {
		return m
	}
	if q.Start != nil {
		m["start"] = itoa(q.Start.Int64())
	}
	if q.End != nil {
		m["end"] = itoa(q.End.Int64())
	}
	if q.Limit > 0 {
		m["limit"] = itoa(int64(q.Limit))
	}
	if q.Sort != 0 {
		m["sort"] = itoa(int64(q.Sort))
	}
	return m
}

// Body merges the query into a request body map (for POST
// history endpoints where the window goes in the JSON body).
func (q *HistoryQuery) Body() map[string]any {
	m := map[string]any{}
	if q == nil {
		return m
	}
	if q.Start != nil {
		m["start"] = q.Start.Int64()
	}
	if q.End != nil {
		m["end"] = q.End.Int64()
	}
	if q.Limit > 0 {
		m["limit"] = q.Limit
	}
	if q.Sort != 0 {
		m["sort"] = q.Sort
	}
	return m
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

// Validate checks the common history window; endpoint-specific maxima are separate.
func (q *HistoryQuery) Validate() error {
	if q == nil {
		return nil
	}
	if q.Limit < 0 || (q.Sort != 0 && q.Sort != SortAsc && q.Sort != SortDesc) {
		return &ValidationError{Field: "history", Message: "limit must be non-negative and sort must be 1 or -1"}
	}
	if (q.Start != nil && *q.Start < 0) || (q.End != nil && *q.End < 0) || (q.Start != nil && q.End != nil && *q.Start > *q.End) {
		return &ValidationError{Field: "history", Message: "invalid start/end window"}
	}
	return nil
}
