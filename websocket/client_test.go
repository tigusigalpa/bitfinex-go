package websocket

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// fakeConn is a minimal scripted WS conn for package-internal
// tests (the public testutil fake is also available to users).
type fakeConn struct {
	in   chan []byte
	out  chan []byte
	done chan struct{}
	once sync.Once
}

func newFakeConn() *fakeConn {
	return &fakeConn{in: make(chan []byte, 64), out: make(chan []byte, 64), done: make(chan struct{})}
}

func (c *fakeConn) ReadMessage() (int, []byte, error) {
	select {
	case m := <-c.in:
		return 1, m, nil
	case <-c.done:
		return 0, nil, errClosed
	}
}
func (c *fakeConn) WriteMessage(_ int, data []byte) error {
	select {
	case c.out <- data:
	default:
	}
	return nil
}

func (c *fakeConn) WriteControl(_ int, _ []byte, _ time.Time) error { return nil }
func (c *fakeConn) Close() error                                    { c.once.Do(func() { close(c.done) }); return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error                { return nil }

var errClosed = errors.New("closed")

type fakeDialer struct{ conn *fakeConn }

func (d *fakeDialer) Dial(_ context.Context, _ string) (Conn, error) { return d.conn, nil }

func testClient(t *testing.T, conn *fakeConn) *Client {
	t.Helper()
	c, err := New(Config{
		PublicURL: "wss://example/ws/2",
		Dialer:    &fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPublicConnectSubscribeTicker(t *testing.T) {
	conn := newFakeConn()
	cl := testClient(t, conn)
	defer cl.Close()

	go func() {
		// handshake info frame
		conn.in <- []byte(`{"event":"info","version":2,"platform":{"status":1}}`)
	}()

	if err := cl.ConnectPublic(context.Background(), 0); err != nil {
		t.Fatalf("connect: %v", err)
	}

	go func() {
		// wait for the subscribe write, then ack + data
		<-conn.out
		conn.in <- []byte(`{"event":"subscribed","channel":"ticker","chanId":5,"symbol":"tBTCUSD","pair":"BTCUSD"}`)
		conn.in <- []byte(`[5,[65000,1.5,65010,2.0,100,0.01,65005,2500,66000,64000]]`)
		conn.in <- []byte(`[5,"hb"]`)
	}()

	sub, err := cl.Subscribe(context.Background(), SubscribeTicker("tBTCUSD"))
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.ChanID() != 5 {
		t.Fatalf("chanID = %d", sub.ChanID())
	}

	// expect subscribed info, ticker update, heartbeat
	var gotTicker bool
	var gotHB bool
	timeout := time.After(3 * time.Second)
	for !(gotTicker && gotHB) {
		select {
		case ev := <-sub.Events():
			switch ev.Kind {
			case KindUpdate:
				if ev.Ticker != nil && ev.Ticker.LastPrice.String() == "65005" {
					gotTicker = true
				}
			case KindHeartbeat:
				gotHB = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for events")
		}
	}
}

func TestSubscribeValidation(t *testing.T) {
	conn := newFakeConn()
	cl := testClient(t, conn)
	defer cl.Close()
	go func() { conn.in <- []byte(`{"event":"info","version":2,"platform":{"status":1}}`) }()
	if err := cl.ConnectPublic(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Subscribe(context.Background(), SubscribeBook("tBTCUSD", "X9", "F0", 0)); err == nil {
		t.Fatal("invalid precision must fail")
	}
	if _, err := cl.Subscribe(context.Background(), SubscriptionRequest{Channel: "bogus"}); err == nil {
		t.Fatal("unknown channel must fail")
	}
}

func TestMaintenanceHandshakeRejected(t *testing.T) {
	conn := newFakeConn()
	cl := testClient(t, conn)
	defer cl.Close()
	go func() { conn.in <- []byte(`{"event":"info","version":2,"platform":{"status":0}}`) }()
	if err := cl.ConnectPublic(context.Background(), 0); err == nil {
		t.Fatal("maintenance status must fail")
	}
}

func TestChecksumVerify(t *testing.T) {
	b := newBookIntegrity()
	// Build a small deterministic book.
	b.applyPrec("100", 1, "1")
	b.applyPrec("99", 1, "2")
	b.applyPrec("101", 1, "-1")
	got := b.checksum(false)
	if !b.verify(got, false) {
		t.Fatal("self-verify failed")
	}
	if b.verify(got+1, false) {
		t.Fatal("wrong checksum accepted")
	}
	// count=0 removes the level.
	b.applyPrec("100", 0, "0")
	if _, ok := b.bids["100"]; ok {
		t.Fatal("level not removed")
	}
}

func TestSequenceGap(t *testing.T) {
	c := &Client{seqAll: true}
	if err := c.checkSeq(1, 10); err != nil {
		t.Fatal(err)
	}
	if err := c.checkSeq(1, 11); err != nil {
		t.Fatal(err)
	}
	var gap *types.SequenceGapError
	if err := c.checkSeq(1, 15); !errors.As(err, &gap) {
		t.Fatalf("want gap, got %v", err)
	}
}

func TestConsumerTooSlow(t *testing.T) {
	c := &Client{cfg: Config{Backpressure: BackpressureStop, EventBuffer: 1}}
	sub := &Subscription{events: make(chan Event, 1)}
	c.deliverTo(sub, Event{Kind: KindUpdate})
	c.deliverTo(sub, Event{Kind: KindUpdate}) // buffer full → error + close
	var sawErr bool
	for ev := range sub.Events() {
		if errors.Is(ev.Err, types.ErrConsumerTooSlow) {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("expected ErrConsumerTooSlow")
	}
}

func TestCloseIdempotent(t *testing.T) {
	conn := newFakeConn()
	cl := testClient(t, conn)
	go func() { conn.in <- []byte(`{"event":"info","version":2,"platform":{"status":1}}`) }()
	if err := cl.ConnectPublic(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := cl.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cl.Close(); err != nil {
		t.Fatal("second Close must be nil")
	}
}
