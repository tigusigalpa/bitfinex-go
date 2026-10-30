package websocket

import (
	"errors"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
	"testing"
)

func TestTypedAccountCalculations(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	sub := &Subscription{events: make(chan Event, 10)}
	c.subs[0] = sub
	cases := []struct {
		tag, raw string
		check    func(*AuthEvent) bool
	}{
		{"bu", `[123.4567890123456789,100,"tail"]`, func(a *AuthEvent) bool {
			return a.Balance != nil && a.Balance.Assets.String() == "123.4567890123456789" && a.Balance.NetAssets.String() == "100"
		}},
		{"miu", `["base",[1,null,3,4,5]]`, func(a *AuthEvent) bool {
			return a.Margin != nil && a.Margin.Base.Net.String() == "4" && !a.Margin.Base.Swaps.IsValid()
		}},
		{"miu", `["sym","tBTCUSD",[1,2,3,4]]`, func(a *AuthEvent) bool { return a.Margin != nil && a.Margin.Market.Buy.String() == "3" }},
		{"fiu", `["sym","fUSD",[0.001,0.002,2,3]]`, func(a *AuthEvent) bool { return a.Funding != nil && a.Funding.YieldLend.String() == "0.002" }},
	}
	for _, tc := range cases {
		p, e := wire.Decode([]byte(tc.raw))
		if e != nil {
			t.Fatal(e)
		}
		c.dispatchAuthStream(tc.tag, p)
		ev := <-sub.Events()
		if ev.Kind != KindUpdate || ev.Auth == nil || !tc.check(ev.Auth) {
			t.Fatalf("%s: %+v", tc.tag, ev)
		}
	}
	for _, tag := range []string{"bu", "miu", "fiu"} {
		c.dispatchAuthStream(tag, []any{})
		ev := <-sub.Events()
		var pe *types.ProtocolError
		if ev.Kind != KindError || !errors.As(ev.Err, &pe) {
			t.Fatalf("invalid %s accepted", tag)
		}
	}
}

func TestTypedStatusChannels(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	sub := &Subscription{events: make(chan Event, 10), req: SubscriptionRequest{Channel: "status", Key: "deriv:tBTCF0:USTF0"}}
	c.subs[1] = sub
	p, e := wire.Decode([]byte(`[100,null,101,102,null,103,null,200,0.1,0.2,null,0.3,null,null,104,null,null,105,null,null,null,0.4,0.5,"tail"]`))
	if e != nil {
		t.Fatal(e)
	}
	c.dispatchUntagged(1, p)
	ev := <-sub.Events()
	if ev.Err != nil || ev.DerivativeStatus == nil || ev.DerivativeStatus.MarkPrice.String() != "104" || ev.DerivativeStatus.Key != "tBTCF0:USTF0" {
		t.Fatalf("derivative: %+v", ev)
	}
	sub.req.Key = "liq:global"
	p, _ = wire.Decode([]byte(`[["pos",42,100.1234,null,"tBTCUSD",1,100,null,1,0,null,99]]`))
	c.dispatchUntagged(1, p)
	ev = <-sub.Events()
	if ev.Err != nil || len(ev.Liquidations) != 1 || ev.Liquidations[0].Timestamp.String() != "100.1234" {
		t.Fatalf("liquidation: %+v", ev)
	}
	c.dispatchUntagged(1, []any{true})
	ev = <-sub.Events()
	if ev.Kind != KindError {
		t.Fatal("invalid status accepted")
	}
}
