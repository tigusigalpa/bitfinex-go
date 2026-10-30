package wire

import (
	"github.com/tigusigalpa/bitfinex-go/types"
	"testing"
)

func TestTypedNotificationFailure(t *testing.T) {
	v, e := Decode([]byte(`[100,"transfer",null,null,null,10020,"ERROR","declined","tail"]`))
	if e != nil {
		t.Fatal(e)
	}
	n, e := Notification(v, func(any) (int, error) { t.Fatal("decoded absent failure payload"); return 0, nil })
	if e != nil || n.Success() || n.Code != 10020 {
		t.Fatalf("%+v %v", n, e)
	}
	for _, raw := range []string{`[]`, `[null]`, `[2]`, `["invalid"]`, `{}`} {
		v, _ := Decode([]byte(raw))
		if _, e := Status(v); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{`[0]`, `[false]`, `[1]`, `[true]`} {
		v, _ := Decode([]byte(raw))
		if _, e := Status(v); e != nil {
			t.Fatal(e)
		}
	}
	var parsed struct {
		Amount types.Decimal `json:"amount"`
	}
	parsed, e = Into[struct {
		Amount types.Decimal `json:"amount"`
	}](map[string]any{"amount": true})
	if e == nil {
		t.Fatal(parsed)
	}
}

func FuzzTypedCalculations(f *testing.F) {
	for _, raw := range []string{`["base",[1,2,3,4,5]]`, `["sym","fUSD",[1,null,3,4]]`, `[]`, `{"x":1}`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, e := Decode([]byte(s))
		if e != nil {
			return
		}
		_, _ = MarginInfo(v)
		_, _ = FundingInfo(v)
		_, _ = Status(v)
		_, _ = Notification(v, func(v any) (types.JSONValue, error) { return Into[types.JSONValue](v) })
	})
}
