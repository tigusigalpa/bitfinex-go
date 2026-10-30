package wire

import (
	"encoding/json"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Into decodes a previously UseNumber-parsed value into a concrete model.
// Re-encoding json.Number preserves its exact decimal lexeme; float64 is never used.
func Into[T any](v any) (T, error) {
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		return out, &types.ProtocolError{Message: "invalid typed response"}
	}
	if err = json.Unmarshal(b, &out); err != nil {
		return out, &types.ProtocolError{Message: "response does not match its typed schema"}
	}
	return out, nil
}

// Object checks required object keys before typed decoding.
func Object(v any, required ...string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, &types.ProtocolError{Message: "expected response object"}
	}
	for _, key := range required {
		if x, ok := m[key]; !ok || x == nil {
			return nil, &types.ProtocolError{Message: "missing required response object field"}
		}
	}
	return m, nil
}

// RequiredString rejects null, absent, and empty string fields.
func RequiredString(a []any, i int) (string, error) {
	s, e := Str(a, i)
	if e != nil {
		return "", e
	}
	if s == "" {
		return "", &types.ProtocolError{Message: "missing required string"}
	}
	return s, nil
}

// RequiredID rejects missing, malformed, zero and negative entity identifiers.
// Numeric strings remain supported without converting through float64.
func RequiredID(a []any, i int) (int64, error) {
	n, e := MustInt(a, i, "entity ID")
	if e != nil {
		return 0, e
	}
	if n <= 0 {
		return 0, &types.ProtocolError{Message: "entity ID must be positive"}
	}
	return n, nil
}

// OptionalInt permits absent/null integer metadata while retaining type errors.
func OptionalInt(a []any, i int) (int64, error) { n, _, e := Int(a, i); return n, e }

// DecimalMap decodes a currency/risk coefficient object without float conversions.
func DecimalMap(v any) (map[string]types.Decimal, error) {
	m, e := Object(v)
	if e != nil {
		return nil, e
	}
	out := make(map[string]types.Decimal, len(m))
	for k, v := range m {
		d, e := RequiredDecimal([]any{v}, 0)
		if e != nil {
			return nil, e
		}
		out[k] = d
	}
	return out, nil
}
