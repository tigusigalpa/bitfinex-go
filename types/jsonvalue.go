package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// JSONValue is an immutable, exact JSON value for user-defined setting values
// and opaque bank/browser metadata. It is not a replacement for endpoint models.
// Number access returns Decimal, never float64. Accessors return detached copies.
type JSONValue struct{ raw string }

// ParseJSONValue validates a single JSON value, preserving numeric bytes.
func ParseJSONValue(raw []byte) (JSONValue, error) {
	if len(raw) > 1<<20 {
		return JSONValue{}, fmt.Errorf("bitfinex: JSON value exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return JSONValue{}, fmt.Errorf("bitfinex: invalid JSON value")
	}
	var more any
	if d.Decode(&more) != io.EOF {
		return JSONValue{}, fmt.Errorf("bitfinex: trailing JSON data")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return JSONValue{}, fmt.Errorf("bitfinex: invalid JSON value")
	}
	return JSONValue{raw: compact.String()}, nil
}

// MarshalJSON preserves the exact stored value; the zero value is null.
func (v JSONValue) MarshalJSON() ([]byte, error) {
	if v.raw == "" {
		return []byte("null"), nil
	}
	return []byte(v.raw), nil
}

// UnmarshalJSON parses a setting/metadata value.
func (v *JSONValue) UnmarshalJSON(b []byte) error {
	x, e := ParseJSONValue(b)
	if e == nil {
		*v = x
	}
	return e
}

// String redacts potentially sensitive setting values from logs.
func (v JSONValue) String() string { return "JSONValue{<redacted>}" }

// GoString redacts debug output.
func (v JSONValue) GoString() string { return v.String() }

// Text returns a JSON string value and whether its type is string.
func (v JSONValue) Text() (string, bool) {
	var s string
	if v.raw == "" || v.raw[0] != '"' {
		return "", false
	}
	e := json.Unmarshal([]byte(v.raw), &s)
	return s, e == nil
}

// Number returns an exact JSON numeric value and whether its type is number.
func (v JSONValue) Number() (Decimal, bool) {
	if v.raw == "" || v.raw[0] == '"' {
		return Decimal{}, false
	}
	d, e := NewDecimal(v.raw)
	return d, e == nil
}

// Boolean returns a JSON boolean and whether its type is boolean.
func (v JSONValue) Boolean() (bool, bool) {
	switch v.raw {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// IsNull reports an absent or JSON null value.
func (v JSONValue) IsNull() bool { return v.raw == "" || v.raw == "null" }

// Array returns detached typed elements, or false for a non-array value.
func (v JSONValue) Array() ([]JSONValue, bool) {
	if v.raw == "" || v.raw[0] != '[' {
		return nil, false
	}
	var a []JSONValue
	e := json.Unmarshal([]byte(v.raw), &a)
	return a, e == nil
}

// Object returns detached typed property values, or false for a non-object value.
func (v JSONValue) Object() (map[string]JSONValue, bool) {
	if v.raw == "" || v.raw[0] != '{' {
		return nil, false
	}
	var o map[string]JSONValue
	e := json.Unmarshal([]byte(v.raw), &o)
	return o, e == nil
}
