package orders

import (
	"fmt"
	"strings"
	"time"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// ClientOrderID is the cid/cid_date pair used for idempotent
// order reconciliation. Use it on every submit so an ambiguous
// outcome can be resolved against active orders / history
// instead of retried.
type ClientOrderID struct {
	ID   int64  `json:"cid"`
	Date string `json:"cid_date"` // YYYY-MM-DD
}

// NewClientOrderID builds a ClientOrderID for the current date.
func NewClientOrderID(id int64, day time.Time) (ClientOrderID, error) {
	if id <= 0 {
		return ClientOrderID{}, &types.ValidationError{Field: "cid", Message: "client order id must be positive"}
	}
	return ClientOrderID{ID: id, Date: day.UTC().Format("2006-01-02")}, nil
}

// SubmitRequest is POST /auth/w/order/submit. Validation is
// fail-closed: required fields and dependent/mutually-exclusive
// parameters are checked before any network I/O.
type SubmitRequest struct {
	Type          string         // e.g. TypeExchangeLimit — required
	Symbol        string         // e.g. "tBTCUSD" — required
	Amount        types.Decimal  // signed; positive buy, negative sell — required non-zero
	Price         *types.Decimal // required unless MARKET/EXCHANGE MARKET
	CID           *ClientOrderID // strongly recommended for reconciliation
	GID           *int64
	Flags         *int64 // bitmask of Flag* constants
	Leverage      *int64 // "lev" — margin only
	PriceTrailing *types.Decimal
	PriceAuxLimit *types.Decimal
	PriceOCOStop  *types.Decimal
	Tif           *time.Time // time-in-force moment → "Y-m-d H:i:s"
	TifDate       *string    // explicit tif string wins over Tif
	Meta          map[string]any
}

// Validate enforces the local contract before sending.
func (r SubmitRequest) Validate() error {
	if _, err := types.ParseSymbol(r.Symbol); err != nil {
		return err
	}
	t := strings.ToUpper(strings.TrimSpace(r.Type))
	if t == "" {
		return &types.ValidationError{Field: "type", Message: "order type is required"}
	}
	if !r.Amount.IsValid() || r.Amount.IsZero() {
		return &types.ValidationError{Field: "amount", Message: "amount must be a non-zero decimal"}
	}
	isMarket := strings.Contains(t, "MARKET")
	if !isMarket && r.Price == nil && !strings.Contains(t, "TRAILING STOP") {
		return &types.ValidationError{Field: "price", Message: "price is required for non-market order types"}
	}
	if isMarket && r.Price != nil {
		return &types.ValidationError{Field: "price", Message: "price must be omitted for market order types"}
	}
	if r.PriceTrailing != nil && !strings.Contains(t, "TRAILING STOP") {
		return &types.ValidationError{Field: "price_trailing", Message: "only valid for trailing stop orders"}
	}
	return nil
}

// Body renders the exact JSON-serializable request body.
// Decimals marshal as canonical strings via their String form —
// never through float64.
func (r SubmitRequest) Body() map[string]any {
	body := map[string]any{
		"type":   r.Type,
		"symbol": r.Symbol,
		"amount": r.Amount.String(),
	}
	if r.Price != nil {
		body["price"] = r.Price.String()
	}
	if r.CID != nil {
		body["cid"] = r.CID.ID
		body["cid_date"] = r.CID.Date
	}
	if r.GID != nil {
		body["gid"] = *r.GID
	}
	if r.Flags != nil {
		body["flags"] = *r.Flags
	}
	if r.Leverage != nil {
		body["lev"] = *r.Leverage
	}
	if r.PriceTrailing != nil {
		body["price_trailing"] = r.PriceTrailing.String()
	}
	if r.PriceAuxLimit != nil {
		body["price_aux_limit"] = r.PriceAuxLimit.String()
	}
	if r.PriceOCOStop != nil {
		body["price_oco_stop"] = r.PriceOCOStop.String()
	}
	if r.Tif != nil {
		body["tif"] = r.Tif.UTC().Format("2006-01-02 15:04:05")
	}
	if r.TifDate != nil {
		body["tif"] = *r.TifDate
	}
	if r.Meta != nil {
		body["meta"] = r.Meta
	}
	return body
}

// UpdateRequest is POST /auth/w/order/update. Only Id is
// required; every other field is an optional patch.
type UpdateRequest struct {
	ID            int64 // required
	Amount        *types.Decimal
	Price         *types.Decimal
	CID           *ClientOrderID
	GID           *int64
	Flags         *int64
	Delta         *types.Decimal
	PriceAuxLimit *types.Decimal
	PriceTrailing *types.Decimal
	Tif           *string
}

// Validate enforces the local contract before sending.
func (r UpdateRequest) Validate() error {
	if r.ID <= 0 {
		return &types.ValidationError{Field: "id", Message: "order id must be positive"}
	}
	return nil
}

// Body renders the request body.
func (r UpdateRequest) Body() map[string]any {
	body := map[string]any{"id": r.ID}
	if r.Amount != nil {
		body["amount"] = r.Amount.String()
	}
	if r.Price != nil {
		body["price"] = r.Price.String()
	}
	if r.CID != nil {
		body["cid"] = r.CID.ID
		body["cid_date"] = r.CID.Date
	}
	if r.GID != nil {
		body["gid"] = *r.GID
	}
	if r.Flags != nil {
		body["flags"] = *r.Flags
	}
	if r.Delta != nil {
		body["delta"] = r.Delta.String()
	}
	if r.PriceAuxLimit != nil {
		body["price_aux_limit"] = r.PriceAuxLimit.String()
	}
	if r.PriceTrailing != nil {
		body["price_trailing"] = r.PriceTrailing.String()
	}
	if r.Tif != nil {
		body["tif"] = *r.Tif
	}
	return body
}

// CancelRequest is POST /auth/w/order/cancel — either by server
// Id or by ClientOrderID (cid/cid_date).
type CancelRequest struct {
	ID  int64          // server order id; takes precedence when >0
	CID *ClientOrderID // alternative selector
}

// ByID builds a CancelRequest selecting the server order id.
func ByID(id int64) CancelRequest { return CancelRequest{ID: id} }

// ByClientOrderID builds a CancelRequest selecting cid/cid_date.
func ByClientOrderID(cid ClientOrderID) CancelRequest {
	return CancelRequest{CID: &cid}
}

// Validate enforces exactly one selector.
func (r CancelRequest) Validate() error {
	if r.ID > 0 {
		return nil
	}
	if r.CID != nil && r.CID.ID > 0 && r.CID.Date != "" {
		return nil
	}
	return &types.ValidationError{Field: "id", Message: "cancel requires either a server order id or a cid/cid_date pair"}
}

// Body renders the request body.
func (r CancelRequest) Body() map[string]any {
	if r.ID > 0 {
		return map[string]any{"id": r.ID}
	}
	return map[string]any{"cid": r.CID.ID, "cid_date": r.CID.Date}
}

// MultiOp is one entry for POST /auth/w/order/multi — the "on",
// "ou", "oc" and "oc_multi" verbs in one call. Build the args
// map with the same field names as the corresponding single-op
// bodies (see SubmitRequest.Body, UpdateRequest.Body,
// CancelRequest.Body).
type MultiOp struct {
	Op   string // "on", "ou", "oc", "oc_multi"
	Args map[string]any
}

// Validate enforces a known verb and non-empty args.
func (op MultiOp) Validate() error {
	switch op.Op {
	case "on", "ou", "oc", "oc_multi", "ox_multi":
	default:
		return &types.ValidationError{Field: "op", Message: fmt.Sprintf("unknown multi op %q (want on/ou/oc/oc_multi/ox_multi)", op.Op)}
	}
	if len(op.Args) == 0 {
		return &types.ValidationError{Field: "args", Message: "multi op args must not be empty"}
	}
	return nil
}
