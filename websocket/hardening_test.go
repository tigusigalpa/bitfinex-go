package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestServerErrorTextIsRedacted(t *testing.T) {
	err := wsErrorEvent(map[string]any{"msg": "private-echo", "code": json.Number("10020")})
	if strings.Contains(err.Error(), "private-echo") || !strings.Contains(err.Error(), "10020") {
		t.Fatal("unsafe server error text")
	}
}

type generationDialer struct {
	mu    sync.Mutex
	conns []*fakeConn
	calls int
}

func (d *generationDialer) Dial(ctx context.Context, _ string) (Conn, error) {
	d.mu.Lock()
	n := d.calls
	d.calls++
	d.mu.Unlock()
	if n >= len(d.conns) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return d.conns[n], nil
}

func TestRepeatedReconnectReauth(t *testing.T) {
	creds, _ := auth.NewCredentials("fixture-key", "fixture-secret")
	dialer := &generationDialer{}
	for i := 0; i < 4; i++ {
		fc := newFakeConn()
		fc.in <- []byte(`{"event":"info","version":2}`)
		fc.in <- []byte(`{"event":"auth","status":"OK"}`)
		dialer.conns = append(dialer.conns, fc)
	}
	c, err := New(Config{PublicURL: "wss://example.invalid/ws/2", PrivateURL: "wss://example.invalid/ws/2", Dialer: dialer, Signer: auth.NewSigner(creds), NonceProvider: auth.NewMillisecondNonceProvider(nil), ReconnectBase: time.Nanosecond, ReconnectCap: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.ConnectAuth(ctx, AuthOptions{Filter: []string{"wallet"}}); err != nil {
		t.Fatal(err)
	}
	last := ""
	for i, fc := range dialer.conns {
		select {
		case raw := <-fc.out:
			var msg map[string]any
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatal(err)
			}
			nonce, _ := msg["authNonce"].(string)
			if nonce == "" || nonce == last || msg["authPayload"] != auth.AuthPayload(nonce) || msg["authSig"] != auth.NewSigner(creds).SignWebSocketAuth(nonce) {
				t.Fatal("missing, reused or incorrectly signed auth nonce")
			}
			if _, ok := msg["dms"]; ok {
				t.Fatal("DMS enabled implicitly")
			}
			last = nonce
		case <-ctx.Done():
			t.Fatalf("reconnect generation %d hung", i)
		}
		if i < 3 {
			_ = fc.Close()
		}
	}
}

func TestConnectCancellationClosesHandshake(t *testing.T) {
	c := testClient(t, newFakeConn())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := c.ConnectPublic(ctx, 0)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("handshake cancellation failed: %v", err)
	}
	select {
	case <-c.done:
	default:
		t.Fatal("read loop leaked after cancellation")
	}
}

func TestCloseInterruptsReconnectDelay(t *testing.T) {
	fc := newFakeConn()
	c := testClient(t, fc)
	fc.in <- []byte(`{"event":"info","version":2}`)
	if err := c.ConnectPublic(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	_ = fc.Close()
	start := time.Now()
	if err := c.Close(); err != nil || time.Since(start) > time.Second {
		t.Fatal("Close blocked in reconnect backoff")
	}
}

type checkedWriter struct {
	*fakeConn
	active  atomic.Int32
	overlap atomic.Bool
}

func (c *checkedWriter) WriteMessage(_ int, _ []byte) error {
	if c.active.Add(1) != 1 {
		c.overlap.Store(true)
	}
	time.Sleep(100 * time.Microsecond)
	c.active.Add(-1)
	return nil
}
func TestConcurrentWritesAreSerialized(t *testing.T) {
	c := testClient(t, newFakeConn())
	conn := &checkedWriter{fakeConn: newFakeConn()}
	c.conn = conn
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Ping(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if conn.overlap.Load() {
		t.Fatal("concurrent WriteMessage calls")
	}
	_ = c.Close()
}

func TestOverflowTerminatesOnceWithoutWaitingForConsumer(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	sub := &Subscription{events: make(chan Event, 1)}
	for i := 0; i < 1000; i++ {
		c.deliverTo(sub, Event{Kind: KindUpdate})
	}
	n := 0
	for ev := range sub.Events() {
		if errors.Is(ev.Err, types.ErrConsumerTooSlow) {
			n++
		}
	}
	if n != 1 || !errors.Is(sub.Err(), types.ErrConsumerTooSlow) {
		t.Fatal("terminal backpressure error lost or duplicated")
	}
}

func TestBlockBackpressureStopsOnClose(t *testing.T) {
	c := testClient(t, newFakeConn())
	c.cfg.Backpressure = BackpressureBlock
	sub := &Subscription{events: make(chan Event, 1)}
	c.deliverTo(sub, Event{Kind: KindUpdate})
	done := make(chan struct{})
	go func() { c.deliverTo(sub, Event{Kind: KindUpdate}); close(done) }()
	_ = c.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked consumer leaked goroutine")
	}
	sub.closeEvents()
}

func TestChecksumGoldenAndRawOrdering(t *testing.T) {
	b := newBookIntegrity()
	b.applyPrec("100", 1, "1")
	b.applyPrec("99", 1, "2")
	b.applyPrec("101", 1, "-1")
	want := int64(int32(crc32.ChecksumIEEE([]byte("100:1:101:-1:99:2"))))
	if b.checksum(false) != want {
		t.Fatal("aggregated checksum not in documented order")
	}
	b.applyRaw(10, "100", "1")
	b.applyRaw(2, "100", "2")
	b.applyRaw(5, "101", "-3")
	want = int64(int32(crc32.ChecksumIEEE([]byte("2:2:5:-3:10:1"))))
	if b.checksum(true) != want {
		t.Fatal("raw checksum does not sort numeric IDs at same price")
	}
}

func TestMalformedAccountStreamReportsProtocolError(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	sub := &Subscription{events: make(chan Event, 10)}
	c.subs[0] = sub
	for _, tag := range []string{"on", "pn", "te", "fon", "fcn", "fln", "wu", "fte", "n"} {
		c.dispatchAuthStream(tag, []any{json.Number("1")})
		ev := <-sub.Events()
		var pe *types.ProtocolError
		if ev.Kind != KindError || !errors.As(ev.Err, &pe) {
			t.Fatalf("malformed %s emitted as valid event", tag)
		}
	}
}

func FuzzWebSocketParser(f *testing.F) {
	for _, s := range []string{`[1,[]]`, `[1,[100,1,2]]`, `[0,"on",[]]`, `[1,"hb",1]`, `{"event":"subscribed","channel":"book","chanId":1}`, `[null,"hb"]`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<16 {
			return
		}
		v, err := wire.Decode([]byte(s))
		if err != nil {
			return
		}
		c := testClient(t, newFakeConn())
		defer c.Close()
		for id, ch := range []string{"auth", "book", "ticker", "trades", "candles"} {
			c.subs[int64(id)] = &Subscription{req: SubscriptionRequest{Channel: ch, Symbol: "tBTCUSD", Prec: "P0"}, events: make(chan Event, 10)}
		}
		c.books[1] = newBookIntegrity()
		c.dispatch(v)
	})
}

func TestSequenceGapDropsCorruptDelta(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	c.seqAll = true
	sub := &Subscription{req: SubscribeBook("tBTCUSD", "P0", "F0", 25), events: make(chan Event, 10)}
	c.subs[1] = sub
	c.books[1] = newBookIntegrity()
	for _, s := range []string{`[1,[[100,1,2]],1]`, `[1,[100,1,3],3]`} {
		v, _ := wire.DecodeString(s)
		c.dispatch(v)
	}
	if ev := <-sub.Events(); ev.Kind != KindSnapshot {
		t.Fatal("no snapshot")
	}
	ev := <-sub.Events()
	var gap *types.SequenceGapError
	if !errors.As(ev.Err, &gap) {
		t.Fatal("no sequence gap error")
	}
	select {
	case ev := <-sub.Events():
		if ev.BookLevel != nil {
			t.Fatal("corrupt delta delivered")
		}
	default:
	}
}

func TestSequenceModeRejectsMissingSequence(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	c.seqAll = true
	sub := &Subscription{req: SubscribeBook("tBTCUSD", "P0", "F0", 25), events: make(chan Event, 4)}
	c.subs[1] = sub
	v, _ := wire.DecodeString(`[1,[100,1,2]]`)
	c.dispatch(v)
	ev := <-sub.Events()
	var pe *types.ProtocolError
	if ev.Kind != KindError || !errors.As(ev.Err, &pe) {
		t.Fatal("unsequenced data accepted in integrity mode")
	}
}

func TestBackoffNeverExceedsCap(t *testing.T) {
	for _, n := range []int{0, 1, 2, 64, 1000} {
		if d := reconnectDelay(time.Second, 5*time.Second, n); d < 0 || d > 5*time.Second {
			t.Fatal(fmt.Sprint(d))
		}
	}
}
