package types

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
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
