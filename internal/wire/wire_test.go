package wire

import (
	"errors"
	"testing"

	"github.com/tigusigalpa/bitfinex-go/types"
)

func decodeArr(t *testing.T, s string) []any {
	t.Helper()
	v, err := Decode([]byte(s))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	a, err := AsArr(v, "test")
	if err != nil {
		t.Fatalf("AsArr: %v", err)
	}
	return a
}

func TestPositionalBasics(t *testing.T) {
	a := decodeArr(t, `[123, "tBTCUSD", "0.5", null, 1700000000000, [1,2]]`)
	if v := IntOr(a, 0, -1); v != 123 {
		t.Errorf("int = %d", v)
	}
	if s, err := Str(a, 1); err != nil || s != "tBTCUSD" {
		t.Errorf("str = %q, %v", s, err)
	}
	if d, err := Dec(a, 2); err != nil || d != "0.5" {
		t.Errorf("dec = %q, %v", d, err)
	}
	if _, ok, _ := Int(a, 3); ok {
		t.Error("null must read as absent")
	}
	if m, ok, _ := Mts(a, 4); !ok || m.Int64() != 1700000000000 {
		t.Errorf("mts = %v", m)
	}
	if n, ok, _ := Nested(a, 5); !ok || len(n) != 2 {
		t.Error("nested wrong")
	}
}

func TestMinLengthAndTrailing(t *testing.T) {
	short := decodeArr(t, `[1, 2]`)
	if err := AssertMinLength(short, 3, "x"); err == nil {
		t.Fatal("short array must fail")
	}
	// Trailing unknown fields are tolerated, never an error.
	long := decodeArr(t, `[1, 2, 3, "unexpected", {"future": true}]`)
	if err := AssertMinLength(long, 3, "x"); err != nil {
		t.Fatalf("trailing fields must be tolerated: %v", err)
	}
}

func TestTypeViolations(t *testing.T) {
	a := decodeArr(t, `[{"obj": 1}]`)
	if _, err := Str(a, 0); err == nil {
		t.Fatal("object must not decode as string")
	}
	var pe *types.ProtocolError
	if _, err := Str(a, 0); !errors.As(err, &pe) {
		t.Fatalf("want ProtocolError, got %T", err)
	}
	b := decodeArr(t, `["1.2.3"]`)
	if _, err := Dec(b, 0); err == nil {
		t.Fatal("invalid decimal lexeme must fail")
	}
}

func TestDecodeMalformedJSON(t *testing.T) {
	if _, err := Decode([]byte(`{nope`)); err == nil {
		t.Fatal("malformed JSON must fail")
	}
}

// FuzzWireDecoder ensures arbitrary JSON never panics the
// positional decoder and errors are typed.
func FuzzWireDecoder(f *testing.F) {
	seeds := []string{
		`[]`, `[1]`, `[1,"a",0.5,null,true,{"k":[]}]`, `["error",10010,"ERR_RATE_LIMIT"]`,
		`null`, `""`, `"x"`, `{}`, `[[[["deep"]]]]`, `[9007199254740993, "x".5]`, `[-1e308]`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := Decode([]byte(s))
		if err != nil {
			return
		}
		arr, err := AsArr(v, "fuzz")
		if err != nil {
			return
		}
		_ = AssertMinLength(arr, 1, "fuzz")
		for i := 0; i < len(arr)+2; i++ {
			_, _ = Str(arr, i)
			_, _, _ = Int(arr, i)
			_, _ = Dec(arr, i)
			_, _ = Bool(arr, i)
			_, _, _ = Mts(arr, i)
			_, _, _ = Nested(arr, i)
		}
		_, _ = Rows(v, "fuzz")
	})
}
