package orders

import (
	"github.com/tigusigalpa/bitfinex-go/types"
	"testing"
	"time"
)

func TestOrderValidation(t *testing.T) {
	d := types.MustDecimal("1")
	absent := types.Decimal{}
	base := SubmitRequest{Type: TypeExchangeLimit, Symbol: "tBTCUSD", Amount: d, Price: &d}
	for _, mutate := range []func(*SubmitRequest){
		func(r *SubmitRequest) { r.Symbol = "fUSD" }, func(r *SubmitRequest) { r.Type = "LIMIT MARKET" },
		func(r *SubmitRequest) { r.Type = "exchange limit" }, func(r *SubmitRequest) { r.Price = &absent },
		func(r *SubmitRequest) { r.Amount = types.DecimalZero }, func(r *SubmitRequest) { r.CID = &ClientOrderID{ID: 1, Date: "bad-date"} },
		func(r *SubmitRequest) { r.PriceTrailing = &d }, func(r *SubmitRequest) { r.Type = "TRAILING STOP"; r.Price = nil },
		func(r *SubmitRequest) { r.Type = "STOP LIMIT" }, func(r *SubmitRequest) { r.PriceAuxLimit = &d },
		func(r *SubmitRequest) { n := int64(5); r.Leverage = &n }, func(r *SubmitRequest) { n := int64(-1); r.GID = &n },
	} {
		r := base
		mutate(&r)
		if r.Validate() == nil {
			t.Fatal("invalid order accepted")
		}
	}
	if base.Validate() != nil {
		t.Fatal("valid order rejected")
	}
	if _, err := NewClientOrderID(1<<45, time.Now()); err == nil {
		t.Fatal("unsafe CID accepted")
	}
	if (UpdateRequest{ID: 1, Amount: &d, Delta: &d}).Validate() == nil {
		t.Fatal("conflicting patch accepted")
	}
	if (CancelRequest{ID: 1, CID: &ClientOrderID{ID: 1, Date: "2026-10-29"}}).Validate() == nil {
		t.Fatal("conflicting selectors accepted")
	}
	for _, r := range []CancelMultiRequest{{All: true, IDs: []int64{1}}, {IDs: []int64{-1}}, {CIDs: []ClientOrderID{{ID: 1, Date: "bad"}}}, {}} {
		if r.Validate() == nil {
			t.Fatal("invalid cancel multi accepted")
		}
	}
	if (CancelRequest{}).Body() == nil {
		t.Fatal("empty body helper must be safe")
	}
}
