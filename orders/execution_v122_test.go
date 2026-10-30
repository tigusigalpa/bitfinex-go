package orders

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func executionRowV122(t *testing.T) []any {
	t.Helper()
	v, err := wire.Decode([]byte(`[42,"tBTCUSD",1700000000000,43,-0.010000000000000001,100.123456789012345678,"MARKET",0,-1,null,null,7,"future"]`))
	if err != nil {
		t.Fatal(err)
	}
	return v.([]any)
}

func TestExecutionRejectsMissingAndMalformedRequiredFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index int
		bad   []any
	}{
		{"symbol", 1, []any{nil, "", true, json.Number("1"), []any{}}},
		{"timestamp", 2, []any{nil, "", true, json.Number("1.1"), json.Number("9223372036854775808"), json.Number("-1")}},
		{"amount", 4, []any{nil, "", true, "invalid", []any{}}},
		{"price", 5, []any{nil, "", true, "invalid", map[string]any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, bad := range tc.bad {
				a := executionRowV122(t)
				a[tc.index] = bad
				_, err := TradeExecutionFromWire(a)
				var pe *types.ProtocolError
				if !errors.As(err, &pe) {
					t.Fatalf("required %s accepted %v: %v", tc.name, bad, err)
				}
			}
		})
	}
	for n := 0; n < 6; n++ {
		if _, err := TradeExecutionFromWire(make([]any, n)); err == nil {
			t.Fatal("truncated trade accepted")
		}
	}
}

func TestExecutionKeepsExactValuesAndOptionalFees(t *testing.T) {
	a := executionRowV122(t)
	v, err := TradeExecutionFromWire(a)
	if err != nil {
		t.Fatal(err)
	}
	if v.Pair != "tBTCUSD" || v.MTSCreate != 1700000000000 || v.ExecAmount.String() != "-0.010000000000000001" || v.ExecPrice.String() != "100.123456789012345678" || v.Fee.IsValid() || v.FeeCurrency != "" {
		t.Fatalf("changed valid te contract: %+v", v)
	}
	a[9], a[10] = "0.000000000000000001", "USD"
	v, err = TradeExecutionFromWire(a)
	if err != nil || v.Fee.String() != "0.000000000000000001" || v.FeeCurrency != "USD" {
		t.Fatalf("changed valid tu contract: %+v %v", v, err)
	}
}

func TestUpdateLeverageValidationAndLegacyOmission(t *testing.T) {
	for _, lev := range []int64{-1, 0, 101, 1 << 62} {
		r := UpdateRequest{ID: 42, Leverage: &lev}
		var ve *types.ValidationError
		if err := r.Validate(); !errors.As(err, &ve) || ve.Field != "lev" {
			t.Fatalf("invalid leverage accepted: %d %v", lev, err)
		}
	}
	for _, lev := range []int64{1, 100} {
		r := UpdateRequest{ID: 42, Leverage: &lev}
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		if r.Body()["lev"] != lev {
			t.Fatal("leverage omitted")
		}
	}
	r := UpdateRequest{ID: 42}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Body()["lev"]; ok {
		t.Fatal("leverage added to legacy spot update")
	}
}
