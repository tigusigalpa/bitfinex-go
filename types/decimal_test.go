package types

import (
	"encoding/json"
	"testing"
)

func TestDecimalLexemePreserved(t *testing.T) {
	for _, s := range []string{"123.45", "-0.001", ".5", "1e-8", "0", "-0", "9007199254740991", "0.00000001"} {
		d, err := NewDecimal(s)
		if err != nil {
			t.Fatalf("NewDecimal(%q): %v", s, err)
		}
		if d.String() != s {
			t.Errorf("lexeme not preserved: %q → %q", s, d.String())
		}
	}
}

func TestDecimalRejects(t *testing.T) {
	for _, s := range []string{"", "abc", "1.2.3", "NaN", "Inf", "0x10", "1e", "--1", " 1", "1 "} {
		if _, err := NewDecimal(s); err == nil {
			t.Errorf("NewDecimal(%q) should fail", s)
		}
	}
}

func TestDecimalArithmetic(t *testing.T) {
	cases := []struct{ a, op, b, want string }{
		{"0.1", "+", "0.2", "0.3"},
		{"1.005", "+", "0.005", "1.01"},
		{"10", "-", "0.001", "9.999"},
		{"0.1", "*", "0.1", "0.01"},
		{"-2.5", "*", "4", "-10"},
		{"1e-8", "+", "1e-8", "0.00000002"},
	}
	for _, c := range cases {
		a, _ := NewDecimal(c.a)
		b, _ := NewDecimal(c.b)
		var got Decimal
		switch c.op {
		case "+":
			got = a.Add(b)
		case "-":
			got = a.Sub(b)
		case "*":
			got = a.Mul(b)
		}
		want, _ := NewDecimal(c.want)
		if got.Cmp(want) != 0 {
			t.Errorf("%s %s %s = %s, want %s", c.a, c.op, c.b, got, c.want)
		}
	}
}

func TestDecimalCmpSignNeg(t *testing.T) {
	a, _ := NewDecimal("-1.5")
	b, _ := NewDecimal("2")
	if a.Cmp(b) != -1 || b.Cmp(a) != 1 || a.Cmp(a) != 0 {
		t.Fatal("Cmp wrong")
	}
	if !a.IsNegative() || a.IsZero() {
		t.Fatal("sign wrong")
	}
	if a.Neg().String() != "1.5" {
		t.Fatal("Neg wrong")
	}
	if a.Abs().String() != "1.5" {
		t.Fatal("Abs wrong")
	}
	z, _ := NewDecimal("0.000")
	if !z.IsZero() {
		t.Fatal("zero detect wrong")
	}
}

func TestDecimalJSONRoundTrip(t *testing.T) {
	d, _ := NewDecimal("123.456789012345678901234567890")
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	// Marshals as a JSON string preserving the exact lexeme.
	if string(raw) != `"123.456789012345678901234567890"` {
		t.Fatalf("marshal = %s", raw)
	}
	var back Decimal
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.String() != d.String() {
		t.Fatal("round-trip lost precision")
	}
	// Bare numbers decode too.
	var n Decimal
	if err := json.Unmarshal([]byte(`0.00000001`), &n); err != nil {
		t.Fatal(err)
	}
	if n.String() != "0.00000001" {
		t.Fatalf("bare number decode = %q", n.String())
	}
}

func TestDecimalZeroValueSafe(t *testing.T) {
	var d Decimal
	if d.IsValid() {
		t.Fatal("zero value must be invalid")
	}
	if !d.IsZero() || d.Sign() != 0 {
		t.Fatal("zero value must behave as zero")
	}
}
