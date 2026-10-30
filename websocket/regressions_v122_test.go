package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestOrderMultiWebSocketWireContract(t *testing.T) {
	fc := newFakeConn()
	c := testClient(t, fc)
	defer c.Close()
	c.conn, c.ready, c.authenticated = fc, true, true
	ops := []orders.MultiOp{
		{Op: "on", Args: map[string]any{"type": "EXCHANGE LIMIT", "symbol": "tBTCUSD", "amount": "0.01", "price": "100"}},
		{Op: "oc", Args: map[string]any{"id": int64(42)}},
	}
	if err := c.OrderMulti(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	raw := <-fc.out
	const want = `[0,"ox_multi",null,[["on",{"amount":"0.01","price":"100","symbol":"tBTCUSD","type":"EXCHANGE LIMIT"}],["oc",{"id":42}]]]`
	if string(raw) != want {
		t.Fatalf("wrong multi-op frame: %s", raw)
	}
	// The documented maximum is 75; errors must not write any frame.
	max := make([]orders.MultiOp, 75)
	for i := range max {
		max[i] = ops[1]
	}
	if err := c.OrderMulti(context.Background(), max); err != nil {
		t.Fatal(err)
	}
	<-fc.out
	for _, invalid := range [][]orders.MultiOp{nil, append(max, ops[1]), {{Op: "unknown", Args: map[string]any{"id": 42}}}} {
		var ve *types.ValidationError
		if err := c.OrderMulti(context.Background(), invalid); !errors.As(err, &ve) {
			t.Fatalf("invalid multi accepted: %v", err)
		}
	}
	select {
	case raw := <-fc.out:
		t.Fatalf("invalid input wrote %s", raw)
	default:
	}
}

type faultConnV122 struct {
	*fakeConn
	fail         atomic.Bool
	failedWrites atomic.Int32
	entered      chan struct{}
	release      <-chan struct{}
}

func (c *faultConnV122) WriteMessage(kind int, b []byte) error {
	if !c.fail.Load() {
		return c.fakeConn.WriteMessage(kind, b)
	}
	c.failedWrites.Add(1)
	if c.entered != nil {
		close(c.entered)
	}
	if c.release != nil {
		<-c.release
	}
	return io.ErrClosedPipe
}

type connDialerV122 struct {
	mu    sync.Mutex
	conns []Conn
}

func (d *connDialerV122) Dial(ctx context.Context, _ string) (Conn, error) {
	d.mu.Lock()
	if len(d.conns) > 0 {
		conn := d.conns[0]
		d.conns = d.conns[1:]
		d.mu.Unlock()
		return conn, nil
	}
	d.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestWriteFailureResetsReauthenticatesAndNeverReplaysCommand(t *testing.T) {
	first := &faultConnV122{fakeConn: newFakeConn()}
	second := newFakeConn()
	for _, fc := range []*fakeConn{first.fakeConn, second} {
		fc.in <- []byte(`{"event":"info","version":2}`)
		fc.in <- []byte(`{"event":"auth","status":"OK"}`)
	}
	creds, _ := auth.NewCredentials("write-recovery-fixture", "fixture-secret")
	c, err := New(Config{PublicURL: "wss://example.invalid/ws/2", PrivateURL: "wss://example.invalid/ws/2", Dialer: &connDialerV122{conns: []Conn{first, second}}, Signer: auth.NewSigner(creds), NonceProvider: auth.NewMillisecondNonceProvider(nil)})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = c.ConnectAuth(ctx, AuthOptions{}); err != nil {
		t.Fatal(err)
	}
	sub, err := c.SubscribeAccount()
	if err != nil {
		t.Fatal(err)
	}
	firstAuth := <-first.out
	first.fail.Store(true)
	if err = c.OrderCancel(ctx, orders.ByID(42)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error lost: %v", err)
	}
	c.mu.Lock()
	ready, authenticated := c.ready, c.authenticated
	c.mu.Unlock()
	if ready || authenticated {
		t.Fatal("failed socket remains usable")
	}
	select {
	case <-first.done:
	default:
		t.Fatal("failed socket was not closed")
	}
	select {
	case ev := <-sub.Events():
		if ev.Kind != KindReset || ev.Generation != 1 {
			t.Fatalf("missing reset: %+v", ev)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var secondAuth []byte
	select {
	case secondAuth = <-second.out:
	case <-ctx.Done():
		t.Fatal("write failure did not reconnect")
	}
	var a, b map[string]any
	if err = json.Unmarshal(firstAuth, &a); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(secondAuth, &b); err != nil {
		t.Fatal(err)
	}
	if a["authNonce"] == b["authNonce"] || b["event"] != "auth" {
		t.Fatal("missing fresh authentication")
	}
	second.in <- []byte(`[0,"ws",[]]`)
	select {
	case ev := <-sub.Events():
		if ev.Kind != KindSnapshot || ev.Generation != 2 {
			t.Fatalf("fresh snapshot missing: %+v", ev)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case raw := <-second.out:
		t.Fatalf("failed command replayed: %s", raw)
	default:
	}
	if first.failedWrites.Load() != 1 {
		t.Fatal("failed command retried")
	}
	if err = c.OrderCancel(ctx, orders.ByID(43)); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-second.out:
		if string(raw) != `[0,"oc",null,{"id":43}]` {
			t.Fatalf("wrong new command: %s", raw)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestOldWriteFailureCannotInvalidateNewGeneration(t *testing.T) {
	release := make(chan struct{})
	old := &faultConnV122{fakeConn: newFakeConn(), entered: make(chan struct{}), release: release}
	old.fail.Store(true)
	c := testClient(t, old.fakeConn)
	defer c.Close()
	c.conn, c.generation, c.ready, c.authenticated = old, 1, true, true
	done := make(chan error, 1)
	go func() { done <- c.OrderCancel(context.Background(), orders.ByID(42)) }()
	select {
	case <-old.entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("writer did not start")
	}
	fresh := newFakeConn()
	c.mu.Lock()
	c.conn, c.generation, c.ready, c.authenticated = fresh, 2, true, true
	c.mu.Unlock()
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("writer hung")
	}
	if err := c.OrderCancel(context.Background(), orders.ByID(43)); err != nil {
		t.Fatalf("old failure corrupted new socket: %v", err)
	}
	select {
	case <-fresh.done:
		t.Fatal("new socket closed by old writer")
	default:
	}
	select {
	case <-old.done:
	default:
		t.Fatal("old socket not closed")
	}
}

type deadlineFailureV122 struct {
	*fakeConn
	writes int
}

func (c *deadlineFailureV122) SetWriteDeadline(time.Time) error { return io.ErrClosedPipe }
func (c *deadlineFailureV122) WriteMessage(int, []byte) error   { c.writes++; return nil }

func TestWriteDeadlineFailureInvalidatesBeforeSending(t *testing.T) {
	fc := &deadlineFailureV122{fakeConn: newFakeConn()}
	c := testClient(t, fc.fakeConn)
	defer c.Close()
	c.conn, c.ready, c.authenticated = fc, true, true
	if err := c.OrderCancel(context.Background(), orders.ByID(42)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if fc.writes != 0 || c.ready || c.authenticated {
		t.Fatal("deadline failure did not stop authenticated writes")
	}
	select {
	case <-fc.done:
	default:
		t.Fatal("failed socket not closed")
	}
}

func TestOrderUpdateLeverageWebSocket(t *testing.T) {
	fc := newFakeConn()
	c := testClient(t, fc)
	defer c.Close()
	c.conn, c.ready, c.authenticated = fc, true, true
	price := types.MustDecimal("123.4500")
	for _, lev := range []int64{1, 100} {
		if err := c.OrderUpdate(context.Background(), orders.UpdateRequest{ID: 42, Price: &price, Leverage: &lev}); err != nil {
			t.Fatal(err)
		}
		v, err := wire.Decode(<-fc.out)
		if err != nil {
			t.Fatal(err)
		}
		frame := v.([]any)
		payload := frame[3].(map[string]any)
		if frame[1] != "ou" || payload["lev"] != json.Number(itoa64(lev)) || payload["price"] != "123.45" || payload["id"] != json.Number("42") || len(payload) != 3 {
			t.Fatalf("wrong update frame: %+v", frame)
		}
	}
	for _, lev := range []int64{-1, 0, 101} {
		var ve *types.ValidationError
		if err := c.OrderUpdate(context.Background(), orders.UpdateRequest{ID: 42, Leverage: &lev}); !errors.As(err, &ve) {
			t.Fatal("invalid leverage accepted")
		}
	}
	select {
	case raw := <-fc.out:
		t.Fatalf("invalid update sent: %s", raw)
	default:
	}
	if !c.ready {
		t.Fatal("validation failure invalidated healthy socket")
	}
}

func TestMalformedExecutionFieldsBecomeWebSocketProtocolErrors(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	s := &Subscription{events: make(chan Event, 1)}
	c.subs[0] = s
	for _, tag := range []string{"te", "tu"} {
		for _, index := range []int{1, 2, 4, 5} {
			v, err := wire.Decode([]byte(`[42,"tBTCUSD",100,43,1,100,"LIMIT",100,1,null,null]`))
			if err != nil {
				t.Fatal(err)
			}
			a := v.([]any)
			a[index] = nil
			c.dispatchAuthStream(tag, a)
			ev := <-s.Events()
			var pe *types.ProtocolError
			if ev.Kind != KindError || !errors.As(ev.Err, &pe) {
				t.Fatalf("%s field %d was accepted", tag, index)
			}
		}
	}
}
