package funding

import (
	"github.com/tigusigalpa/bitfinex-go/types"
	"testing"
)

func TestOfferContract(t *testing.T) {
	d := types.MustDecimal("+001.000")
	r := OfferRequest{Type: TypeLimit, Symbol: "fUSD", Amount: d, Rate: types.DecimalZero, Period: 2}
	if r.Validate() != nil || r.Body()["type"] != "LIMIT" || r.Body()["amount"] != "1" {
		t.Fatal("invalid funding offer wire contract")
	}
	for _, typ := range []string{TypeLend, TypeLoan, "MARKET"} {
		r.Type = typ
		if r.Validate() == nil {
			t.Fatal("unsupported offer type accepted")
		}
	}
	r.Type = TypeFRRDeltaFix
	r.Rate = types.MustDecimal("-0.001")
	if r.Validate() != nil {
		t.Fatal("negative fixed offset rejected")
	}
	r.Type = TypeFRRDeltaVar
	if r.Validate() == nil {
		t.Fatal("negative variable offset accepted")
	}
}
