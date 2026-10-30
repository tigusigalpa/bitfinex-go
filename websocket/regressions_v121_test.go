package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/testutil"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func TestWebSocketAuthOrdersRESTUntilAcknowledged(t *testing.T) {
	for _, status := range []string{"OK", "ERROR", "disconnect"} {
		t.Run(status, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			creds, _ := auth.NewCredentials("ws-rest-order-"+status, "fixture-secret")
			signer := auth.NewSigner(creds)
			var nonces atomic.Int64
			provider := auth.FuncNonceProvider(func(context.Context) (string, error) { return strconv.FormatInt(nonces.Add(1), 10), nil })
			fc := newFakeConn()
			fc.in <- []byte(`{"event":"info","version":2}`)
			c, e := New(Config{PublicURL: "wss://example.invalid/ws/2", PrivateURL: "wss://example.invalid/ws/2", Dialer: &fakeDialer{conn: fc}, Signer: signer, NonceProvider: provider})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			done := make(chan error, 1)
			go func() { done <- c.ConnectAuth(ctx, AuthOptions{}) }()
			select {
			case <-fc.out:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			doer := &testutil.FakeDoer{}
			tr := httpx.New(httpx.Config{Doer: doer, PrivateBase: "https://example.invalid/v2", Signer: signer, NonceProvider: provider})
			queued, stop := context.WithTimeout(ctx, 30*time.Millisecond)
			_, e = tr.PostAuth(queued, "v2/auth/r/wallets", nil, httpx.RequestContext{})
			stop()
			if !errors.Is(e, context.DeadlineExceeded) || nonces.Load() != 1 {
				t.Fatalf("REST bypassed WS auth acknowledgement: %v, nonces=%d", e, nonces.Load())
			}
			if status == "disconnect" {
				_ = fc.Close()
			} else {
				fc.in <- []byte(`{"event":"auth","status":"` + status + `"}`)
			}
			select {
			case e = <-done:
				if (e == nil) != (status == "OK") {
					t.Fatalf("auth result: %v", e)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if _, e = tr.PostAuth(ctx, "v2/auth/r/wallets", nil, httpx.RequestContext{}); e != nil || nonces.Load() != 2 {
				t.Fatalf("WS auth leaked gate: %v", e)
			}
			if len(doer.Requests) != 1 || doer.Requests[0].Header.Get("bfx-nonce") != "2" {
				t.Fatal("wrong REST nonce after auth")
			}
		})
	}
}

type waitingRESTDoer struct {
	entered chan struct{}
	release chan struct{}
}

func (d *waitingRESTDoer) Do(r *http.Request) (*http.Response, error) {
	close(d.entered)
	select {
	case <-d.release:
		return testutil.JSONResponse(200, `[]`, nil), nil
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
}

func TestRESTOrdersWebSocketAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	creds, _ := auth.NewCredentials("rest-ws-order", "fixture-secret")
	signer := auth.NewSigner(creds)
	var nonces atomic.Int64
	provider := auth.FuncNonceProvider(func(context.Context) (string, error) { return strconv.FormatInt(nonces.Add(1), 10), nil })
	d := &waitingRESTDoer{entered: make(chan struct{}), release: make(chan struct{})}
	tr := httpx.New(httpx.Config{Doer: d, PrivateBase: "https://example.invalid/v2", Signer: signer, NonceProvider: provider})
	done := make(chan error, 1)
	go func() { _, e := tr.PostAuth(ctx, "v2/auth/r/wallets", nil, httpx.RequestContext{}); done <- e }()
	select {
	case <-d.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	fc := newFakeConn()
	fc.in <- []byte(`{"event":"info","version":2}`)
	c, e := New(Config{PublicURL: "wss://example.invalid/ws/2", PrivateURL: "wss://example.invalid/ws/2", Dialer: &fakeDialer{conn: fc}, Signer: signer, NonceProvider: provider})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	queued, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	e = c.ConnectAuth(queued, AuthOptions{})
	stop()
	if e == nil || nonces.Load() != 1 {
		t.Fatalf("auth bypassed pending REST response: %v", e)
	}
	select {
	case raw := <-fc.out:
		t.Fatalf("queued auth wrote %s", raw)
	default:
	}
	close(d.release)
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestDisconnectResetReplacesFullQueuesBeforeBackoff(t *testing.T) {
	fc := newFakeConn()
	fc.in <- []byte(`{"event":"info","version":2}`)
	c := testClient(t, fc)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := c.ConnectPublic(ctx, 0); e != nil {
		t.Fatal(e)
	}
	book := &Subscription{chanID: 5, req: SubscribeBook("tBTCUSD", "P0", "F0", 25), events: make(chan Event, 2)}
	account := &Subscription{req: SubscriptionRequest{Channel: "account"}, events: make(chan Event, 2)}
	pending := &Subscription{req: SubscribeTicker("tETHUSD"), events: make(chan Event, 2)}
	c.mu.Lock()
	c.subs[5], c.subs[0] = book, account
	c.pending["fixture"] = &pendAck{sub: pending}
	c.books[5] = newBookIntegrity()
	c.mu.Unlock()
	for _, s := range []*Subscription{book, account, pending} {
		s.events <- Event{Kind: KindUpdate}
		s.events <- Event{Kind: KindHeartbeat}
	}
	for i := 0; i < cap(c.lifeCh); i++ {
		c.lifeCh <- Event{Kind: KindInfo}
	}
	_ = fc.Close()
	select {
	case ev := <-c.Lifecycle():
		// The reader can race the reset drain, so find its retained marker.
		for ev.Kind != KindReset {
			select {
			case ev = <-c.Lifecycle():
			case <-ctx.Done():
				t.Fatal("missing disconnect reset")
			}
		}
		if ev.Generation != 1 || ev.Action != "disconnected" || ev.Err == nil {
			t.Fatalf("invalid reset: %+v", ev)
		}
	case <-ctx.Done():
		t.Fatal("disconnect notification delayed until reconnect")
	}
	for _, s := range []*Subscription{book, account, pending} {
		select {
		case ev := <-s.Events():
			if ev.Kind != KindReset || ev.Generation != 1 {
				t.Fatalf("stale subscription event: %+v", ev)
			}
		case <-ctx.Done():
			t.Fatal("missing subscription reset")
		}
		select {
		case ev := <-s.Events():
			t.Fatalf("old event after reset: %+v", ev)
		default:
		}
	}
	c.mu.Lock()
	ready, authenticated, n := c.ready, c.authenticated, len(c.books)
	c.mu.Unlock()
	if ready || authenticated || n != 0 {
		t.Fatal("disconnected state retained")
	}
	pending.closeEvents()
}

func TestBookRequiresFreshSnapshotAndStampsGeneration(t *testing.T) {
	fc := newFakeConn()
	c := testClient(t, fc)
	defer c.Close()
	s := &Subscription{chanID: 5, req: SubscribeBook("tBTCUSD", "P0", "F0", 25), events: make(chan Event, 8)}
	c.subs[5] = s
	c.books[5] = newBookIntegrity()
	c.conn = fc
	c.generation = 2
	dispatch := func(raw string) {
		t.Helper()
		v, e := wire.Decode([]byte(raw))
		if e != nil {
			t.Fatal(e)
		}
		c.dispatchUntagged(5, v)
	}
	dispatch(`[100,1,1]`)
	ev := <-s.Events()
	var pe *types.ProtocolError
	if ev.Kind != KindError || !errors.As(ev.Err, &pe) {
		t.Fatal("delta accepted before snapshot")
	}
	select {
	case <-fc.done:
	default:
		t.Fatal("missing resync")
	}
	// Next generation starts with a new integrity book.
	c.books[5] = newBookIntegrity()
	c.generation = 3
	dispatch(`[[100,1,1]]`)
	dispatch(`[100,1,2]`)
	snap, update := <-s.Events(), <-s.Events()
	if snap.Kind != KindSnapshot || update.Kind != KindUpdate || snap.Generation != 3 || update.Generation != 3 || update.BookLevel.Entry.Amount.String() != "2" {
		t.Fatal("fresh book did not recover")
	}
}

func TestChecksumCannotValidateBookBeforeSnapshot(t *testing.T) {
	fc := newFakeConn()
	c := testClient(t, fc)
	defer c.Close()
	c.conn = fc
	s := &Subscription{events: make(chan Event, 2), req: SubscribeBook("tBTCUSD", "P0", "F0", 25)}
	c.subs[5], c.books[5] = s, newBookIntegrity()
	c.handleChecksum(5, json.Number("0")) // CRC32 of an empty, uninitialized book
	ev := <-s.Events()
	var pe *types.ProtocolError
	if ev.Kind != KindError || !errors.As(ev.Err, &pe) {
		t.Fatal("uninitialized book passed checksum")
	}
}

func TestReconnectDeliversNewGenerationSnapshot(t *testing.T) {
	first, second := newFakeConn(), newFakeConn()
	first.in <- []byte(`{"event":"info","version":2}`)
	second.in <- []byte(`{"event":"info","version":2}`)
	c, e := New(Config{PublicURL: "wss://example.invalid/ws/2", Dialer: &generationDialer{conns: []*fakeConn{first, second}}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if e = c.ConnectPublic(ctx, 0); e != nil {
		t.Fatal(e)
	}
	req := SubscribeBook("tBTCUSD", "P0", "F0", 25)
	s := &Subscription{chanID: 5, req: req, events: make(chan Event, 8)}
	c.mu.Lock()
	c.subs[5], c.byKey[req.cacheKey()], c.reqs[req.cacheKey()] = s, s, req
	c.books[5] = newBookIntegrity()
	c.mu.Unlock()
	_ = first.Close()
	select {
	case ev := <-s.Events():
		if ev.Kind != KindReset || ev.Generation != 1 {
			t.Fatalf("invalid reset: %+v", ev)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-second.out:
	case <-ctx.Done():
		t.Fatal("resubscribe did not happen")
	}
	second.in <- []byte(`{"event":"subscribed","channel":"book","chanId":9,"symbol":"tBTCUSD","prec":"P0","freq":"F0","len":25}`)
	second.in <- []byte(`[9,[[100,1,1]]]`)
	second.in <- []byte(`[9,[100,1,2]]`)
	for _, kind := range []Kind{KindInfo, KindSnapshot, KindUpdate} {
		select {
		case ev := <-s.Events():
			if ev.Kind != kind || ev.Generation != 2 || ev.ChanID != 9 || ev.Err != nil {
				t.Fatalf("wrong new generation event: %+v", ev)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

// Done is consulted inside sendJSONGeneration, after SendAuthInput captured
// readiness and generation. This barrier avoids timing-dependent sleeps.
type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
	done     chan struct{}
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.done
}

func TestQueuedAuthInputCannotCrossConnectionState(t *testing.T) {
	for _, change := range []string{"generation", "maintenance", "unauthenticated"} {
		t.Run(change, func(t *testing.T) {
			fc := newFakeConn()
			c := testClient(t, fc)
			defer c.Close()
			c.conn = fc
			c.generation = 1
			c.ready = true
			c.authenticated = true
			c.writeOnce.Do(func() { c.writeGate = make(chan struct{}, 1) })
			c.writeGate <- struct{}{}
			ctx := &observedContext{Context: context.Background(), observed: make(chan struct{}), done: make(chan struct{})}
			done := make(chan error, 1)
			go func() { done <- c.SendAuthInput(ctx, "on", map[string]any{"cid": 42}) }()
			select {
			case <-ctx.observed:
			case <-time.After(time.Second):
				t.Fatal("writer did not queue")
			}
			c.mu.Lock()
			switch change {
			case "generation":
				c.generation = 2
				c.conn = newFakeConn()
			case "maintenance":
				c.paused = true
			case "unauthenticated":
				c.authenticated = false
			}
			c.mu.Unlock()
			<-c.writeGate
			select {
			case e := <-done:
				if e == nil {
					t.Fatal("stale authenticated input was sent")
				}
			case <-time.After(time.Second):
				t.Fatal("queued write hung")
			}
			select {
			case raw := <-fc.out:
				t.Fatalf("old order sent: %s", raw)
			default:
			}
		})
	}
}

func TestLiquidationNullableAcquiredPriceWebSocket(t *testing.T) {
	c := testClient(t, newFakeConn())
	defer c.Close()
	s := &Subscription{events: make(chan Event, 4), req: SubscribeStatus("liq:global")}
	c.subs[1] = s
	for _, price := range []string{"null", "0", "0.076197"} {
		v, e := wire.Decode([]byte(`[["pos",194840755,1793333832536,null,"tDOGEF0:USTF0",-1e-8,0.076197,null,0,1,null,` + price + `]]`))
		if e != nil {
			t.Fatal(e)
		}
		c.dispatchUntagged(1, v)
		ev := <-s.Events()
		if ev.Err != nil || len(ev.Liquidations) != 1 || ev.Liquidations[0].PriceAcquired.IsValid() != (price != "null") {
			t.Fatalf("price %s rejected: %+v", price, ev)
		}
	}
	// Malformed required entity IDs surface as typed stream errors, never ID=0.
	c.subs[0] = &Subscription{events: make(chan Event, 1)}
	c.dispatchAuthStream("te", []any{"bad-id", "tBTCUSD", json.Number("100"), json.Number("42"), json.Number("1"), json.Number("100")})
	ev := <-c.subs[0].Events()
	var pe *types.ProtocolError
	if ev.Kind != KindError || !errors.As(ev.Err, &pe) {
		t.Fatal("malformed account stream ID accepted")
	}
}
