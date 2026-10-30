package account

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestAccountRequiredIDs(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		decode    func([]any) (int64, error)
	}{
		{"user", `[42,"fixture@example.invalid","fixture",100,1,1]`, func(a []any) (int64, error) { v, e := userInfoFromWire(a); return v.ID, e }},
		{"ledger", `[42,"USD",null,100,null,1,2,null,"fixture"]`, func(a []any) (int64, error) { v, e := ledgerEntryFromWire(a); return v.ID, e }},
		{"movement", `[42,"USD","wire",null,null,100,101,null,null,"COMPLETED",null,null,1,0]`, func(a []any) (int64, error) { v, e := movementFromWire(a); return v.ID, e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, e := wire.Decode([]byte(tc.raw))
			if e != nil {
				t.Fatal(e)
			}
			a := v.([]any)
			for _, bad := range []any{nil, "", true, "invalid", json.Number("0"), json.Number("-1"), json.Number("1.1"), json.Number("9223372036854775808")} {
				a[0] = bad
				_, e := tc.decode(a)
				var pe *types.ProtocolError
				if !errors.As(e, &pe) {
					t.Fatalf("invalid ID accepted: %v %v", bad, e)
				}
			}
			a[0] = "9007199254740993"
			id, e := tc.decode(append(a, "tail"))
			if e != nil || id != 9007199254740993 {
				t.Fatalf("lost exact ID: %d %v", id, e)
			}
		})
	}
}
