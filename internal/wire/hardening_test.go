package wire

import (
	"encoding/json"
	"math"
	"testing"
)

func TestStrictWireNumbersAndJSON(t *testing.T) {
	for _, s := range []string{`[] []`, `null true`, `{} trailing`} {
		if _, e := Decode([]byte(s)); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	for _, v := range []any{true, json.Number("1.5"), "1x"} {
		if _, _, e := Int([]any{v}, 0); e == nil {
			t.Fatal("invalid integer accepted")
		}
	}
	if itoa(math.MinInt64) != "-9223372036854775808" {
		t.Fatal("integer overflow")
	}
	if _, err := RequiredDecimal([]any{nil}, 0); err == nil {
		t.Fatal("null required decimal accepted")
	}
}
