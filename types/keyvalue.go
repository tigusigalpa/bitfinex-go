package types

// KeyValueInfo is the loosely-typed info envelope returned by
// /auth/r/info/* endpoints: [TYPE, SYMBOL?, ...payload]. The
// payload shape varies per key, so the tail is preserved raw —
// forward-compatible by construction.
type KeyValueInfo struct {
	Type   string `json:"type"`
	Symbol string `json:"symbol,omitempty"`
	Data   []any  `json:"data,omitempty"`
}

// KeyValueInfoFromSlice decodes a positional info row without
// importing the internal wire package (public model boundary).
// Indices follow the documented envelope: 0=type, 1=symbol,
// 2..=payload.
func KeyValueInfoFromSlice(a []any) KeyValueInfo {
	kv := KeyValueInfo{}
	if len(a) == 0 {
		return kv
	}
	kv.Type, _ = a[0].(string)
	if len(a) > 1 {
		kv.Symbol, _ = a[1].(string)
	}
	if len(a) > 2 {
		kv.Data = append([]any(nil), a[2:]...)
	}
	return kv
}
