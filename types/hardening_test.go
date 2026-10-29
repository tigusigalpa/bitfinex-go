package types

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestDecimalCanonicalAndBounded(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"+001.2000", "1.2"}, {".5", "0.5"}, {"1e-8", "0.00000001"}, {"-0.0", "0"}} {
		d, err := NewDecimal(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(d)
		if err != nil || string(b) != `"`+tc.want+`"` {
			t.Fatalf("marshal %s: %s %v", tc.in, b, err)
		}
		if d.String() != tc.in || d.CanonicalString() != tc.want {
			t.Fatal("lexeme or canonical value changed")
		}
		if d.Neg().Add(d).Sign() != 0 {
			t.Fatal("negation lost exactness")
		}
	}
	for _, s := range []string{"1e999999999999999999999", "1e-1025", "1e-1024", "1e1024", strings.Repeat("1", 1025)} {
		if _, err := NewDecimal(s); err == nil {
			t.Fatal("unbounded decimal accepted")
		}
	}
	var d Decimal
	for _, raw := range []string{`"1""`, `+1`, `NaN`, `"1e999999999999999"`, `true`} {
		if d.UnmarshalJSON([]byte(raw)) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if (Decimal{}).Neg().IsValid() {
		t.Fatal("negation made absent value valid")
	}
}

func TestHistoryWindowValidation(t *testing.T) {
	a, b := Millis(20), Millis(10)
	for _, q := range []*HistoryQuery{{Start: &a, End: &b}, {Limit: -1}, {Sort: 2}} {
		if q.Validate() == nil {
			t.Fatal("invalid history accepted")
		}
	}
	if itoa(math.MinInt64) != "-9223372036854775808" {
		t.Fatal("integer formatting overflow")
	}
}

func TestWrappedAPIErrors(t *testing.T) {
	for _, err := range []error{&RateLimitError{APIError: APIError{Code: 10010}}, &AuthenticationError{APIError: APIError{Code: 10100}}} {
		var api *APIError
		if !errors.As(err, &api) || api.Code == 0 {
			t.Fatal("APIError unavailable through wrapping")
		}
	}
}

func FuzzDecimal(f *testing.F) {
	for _, s := range []string{"1e-8", "+1", "-0", "1e99999999999999999999", ".123"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := NewDecimal(s)
		if err != nil {
			return
		}
		if d.Add(d.Neg()).Sign() != 0 {
			t.Fatal("arithmetic inconsistency")
		}
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		var back Decimal
		if err = json.Unmarshal(b, &back); err != nil || d.Cmp(back) != 0 {
			t.Fatal("inexact roundtrip")
		}
	})
}
