// Package wire holds positional-array decoder primitives.
//
// Bitfinex v2 packs models into JSON arrays by index and may
// append new fields at the tail without a version bump. So:
//   - we validate a documented MINIMUM length and the types of
//     the mandatory indices we read;
//   - unknown tail indices are ignored, never an error;
//   - exact length is never part of the contract.
//
// All numbers are decoded as json.Number (Decoder.UseNumber), so
// money lexemes are preserved exactly and floats can never leak
// into domain values.
package wire

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// Decode parses JSON with numbers preserved as json.Number.
func Decode(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, &types.ProtocolError{Message: "undecodable JSON body: " + err.Error()}
	}
	return v, nil
}

// DecodeString is Decode for string input (WebSocket frames).
func DecodeString(s string) (any, error) { return Decode([]byte(s)) }

// Encode marshals a request body. Inputs are trusted library
// structs; a failure is a bug, not user error.
func Encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("bitfinex: failed to encode request body: %w", err)
	}
	return b, nil
}

// Arr is a decoded positional array.
type Arr []any

// AssertMinLength enforces the documented minimum arity.
func AssertMinLength(a []any, min int, what string) error {
	if len(a) < min {
		return &types.ProtocolError{Message: fmt.Sprintf(
			"wire format violation for %s: expected at least %d elements, got %d", what, min, len(a))}
	}
	return nil
}

// AsArr casts v to a positional array.
func AsArr(v any, what string) (Arr, error) {
	a, ok := v.([]any)
	if !ok {
		return nil, &types.ProtocolError{Message: fmt.Sprintf("expected array for %s, got %s", what, kind(v))}
	}
	return a, nil
}

// At returns a[i] or nil when out of range.
func At(a []any, i int) any {
	if i < 0 || i >= len(a) {
		return nil
	}
	return a[i]
}

// Str reads an optional string at index i. Null → "".
func Str(a []any, i int) (string, error) {
	v := At(a, i)
	if v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", &types.ProtocolError{Message: fmt.Sprintf("expected string at index %d, got %s (%s)", i, kind(v), abbrev(v))}
	}
	return s, nil
}

// StrOr is Str with a default for null/absent.
func StrOr(a []any, i int, def string) string {
	s, err := Str(a, i)
	if err != nil || s == "" {
		return def
	}
	return s
}

// Int reads an optional int64 at index i. Null → 0,false.
// Accepts json.Number and numeric strings; rejects non-integral
// values loudly.
func Int(a []any, i int) (int64, bool, error) {
	v := At(a, i)
	if v == nil {
		return 0, false, nil
	}
	switch n := v.(type) {
	case json.Number:
		if iv, err := n.Int64(); err == nil {
			return iv, true, nil
		}
		return 0, false, &types.ProtocolError{Message: fmt.Sprintf("non-integral number at index %d: %s", i, n.String())}
	case string:
		if n == "" {
			return 0, false, nil
		}
		var iv int64
		if _, err := fmt.Sscanf(n, "%d", &iv); err == nil && itoa(iv) == n {
			return iv, true, nil
		}
		return 0, false, &types.ProtocolError{Message: fmt.Sprintf("non-integral string at index %d: %s", i, abbrev(v))}
	case bool:
		if n {
			return 1, true, nil
		}
		return 0, true, nil
	default:
		return 0, false, &types.ProtocolError{Message: fmt.Sprintf("expected integer at index %d, got %s (%s)", i, kind(v), abbrev(v))}
	}
}

// IntOr is Int with a default and error collapse for optional fields.
func IntOr(a []any, i int, def int64) int64 {
	v, ok, err := Int(a, i)
	if err != nil || !ok {
		return def
	}
	return v
}

// MustInt reads a required int64 at index i.
func MustInt(a []any, i int, what string) (int64, error) {
	v, ok, err := Int(a, i)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, &types.ProtocolError{Message: fmt.Sprintf("missing required integer at index %d for %s", i, what)}
	}
	return v, nil
}

// Dec reads an optional decimal at index i and returns its exact
// lexeme ("" when null/absent). json.Number, string and bool-typed
// values are handled; a nested array/object is an error.
func Dec(a []any, i int) (string, error) {
	v := At(a, i)
	if v == nil {
		return "", nil
	}
	switch n := v.(type) {
	case json.Number:
		s := n.String()
		if s == "" {
			return "", nil
		}
		if _, err := types.NewDecimal(s); err != nil {
			return "", &types.ProtocolError{Message: fmt.Sprintf("invalid decimal at index %d: %s", i, abbrev(s))}
		}
		return s, nil
	case string:
		if n == "" {
			return "", nil
		}
		if _, err := types.NewDecimal(n); err != nil {
			return "", &types.ProtocolError{Message: fmt.Sprintf("invalid decimal at index %d: %s", i, abbrev(n))}
		}
		return n, nil
	default:
		return "", &types.ProtocolError{Message: fmt.Sprintf("expected decimal at index %d, got %s (%s)", i, kind(v), abbrev(v))}
	}
}

// Decimal reads an optional decimal into types.Decimal.
// Returns (Decimal{}, false, nil) when absent.
func Decimal(a []any, i int) (types.Decimal, bool, error) {
	s, err := Dec(a, i)
	if err != nil || s == "" {
		return types.Decimal{}, false, err
	}
	d, derr := types.NewDecimal(s)
	if derr != nil {
		return types.Decimal{}, false, &types.ProtocolError{Message: fmt.Sprintf("invalid decimal at index %d", i)}
	}
	return d, true, nil
}

// DecOr is Decimal with a default for null/absent.
func DecOr(a []any, i int, def types.Decimal) (types.Decimal, error) {
	d, ok, err := Decimal(a, i)
	if err != nil {
		return types.Decimal{}, err
	}
	if !ok {
		return def, nil
	}
	return d, nil
}

// Bool reads a 0/1 flag at index i. Null → false.
func Bool(a []any, i int) (bool, error) {
	v, ok, err := Int(a, i)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return v != 0, nil
}

// Mts reads an optional timestamp (ms) at index i.
// Returns (0, false, nil) when absent.
func Mts(a []any, i int) (types.Millis, bool, error) {
	v, ok, err := Int(a, i)
	if err != nil || !ok {
		return 0, false, err
	}
	return types.Millis(v), true, nil
}

// Nested reads an optional nested array at index i.
func Nested(a []any, i int) (Arr, bool, error) {
	v := At(a, i)
	if v == nil {
		return nil, false, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, false, &types.ProtocolError{Message: fmt.Sprintf("expected array at index %d, got %s", i, kind(v))}
	}
	return arr, true, nil
}

// Rows casts v into a list of positional arrays, rejecting
// non-array rows with a ProtocolError.
func Rows(v any, what string) ([]Arr, error) {
	list, err := AsArr(v, what)
	if err != nil {
		return nil, err
	}
	out := make([]Arr, 0, len(list))
	for i, item := range list {
		row, ok := item.([]any)
		if !ok {
			return nil, &types.ProtocolError{Message: fmt.Sprintf("%s row %d is %s, expected array", what, i, kind(item))}
		}
		out = append(out, row)
	}
	return out, nil
}

func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "bool"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func abbrev(v any) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
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
