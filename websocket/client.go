package websocket

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/tigusigalpa/bitfinex-go/funding"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// Server info codes (numeric — never match on message text).
const (
	infoHaltReconnect    = 20051 // reconnect + resubscribe
	infoMaintenanceStart = 20060 // pause activity
	infoMaintenanceEnd   = 20061 // resume + resubscribe
)

// Subscription is one channel's typed event stream. Events()
// is closed when the subscription ends (unsubscribe ack,
// consumer-too-slow, or client Close). Order is preserved per
// channel.
type Subscription struct {
	req    SubscriptionRequest
	events chan Event

	mu       sync.Mutex
	stopInit sync.Once
	stopOnce sync.Once
	stop     chan struct{}
	chanID   int64
	closed   bool
	err      error
}

// ChanID returns the current server channel id (it changes on
// resubscribe).
func (s *Subscription) ChanID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chanID
}

// Request returns the original subscription request.
func (s *Subscription) Request() SubscriptionRequest { return s.req }

// Events yields ordered events until the subscription ends.
func (s *Subscription) Events() <-chan Event { return s.events }

// Err reports the terminal subscription failure, if any.
func (s *Subscription) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }

func (s *Subscription) closeEvents() {
	s.stopDelivery()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	close(s.events)
}

func (s *Subscription) deliveryDone() <-chan struct{} {
	s.stopInit.Do(func() { s.stop = make(chan struct{}) })
	return s.stop
}
func (s *Subscription) stopDelivery() { _ = s.deliveryDone(); s.stopOnce.Do(func() { close(s.stop) }) }

// pendAck tracks a subscription waiting for its ack. ack is
// signaled (non-blocking) by the read loop; a nil ack channel
// means an internal resubscribe with no waiter.
type pendAck struct {
	key string
	req SubscriptionRequest
	sub *Subscription
	ack chan error
}

// Client is one WebSocket connection (public or authenticated)
// with typed subscriptions. Construct via New, drive with
// ConnectPublic/ConnectAuth, release with Close.
type Client struct {
	cfg       Config
	ctx       context.Context
	writeOnce sync.Once
	writeGate chan struct{}

	mu            sync.Mutex
	conn          Conn
	closed        bool
	authed        bool
	seqAll        bool
	started       bool
	ready         bool
	everReady     bool
	generation    uint64
	authenticated bool
	done          chan struct{}
	cancel        context.CancelFunc
	lifeCh        chan Event
	pingSeq       int64
	subs          map[int64]*Subscription
	byKey         map[string]*Subscription
	pending       map[string]*pendAck
	reqs          map[string]SubscriptionRequest
	books         map[int64]*bookIntegrity
	lastSeq       int64
	seqInit       bool
	paused        bool
}

// String keeps authentication dependencies out of diagnostics.
func (c *Client) String() string { return "websocket.Client{<redacted>}" }

// GoString redacts debug formatting.
func (c *Client) GoString() string { return c.String() }

// New builds a Client from cfg. The connection is NOT opened
// until ConnectPublic/ConnectAuth.
func New(cfg Config) (*Client, error) {
	for _, raw := range []string{cfg.PublicURL, cfg.PrivateURL} {
		if raw == "" {
			continue
		}
		u, e := url.Parse(raw)
		if e != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" || u.User != nil {
			return nil, &types.ValidationError{Field: "ws_url", Message: "invalid WebSocket endpoint"}
		}
	}
	if cfg.EventBuffer < 0 || cfg.MaxSubscriptions < 0 || (cfg.Backpressure != BackpressureStop && cfg.Backpressure != BackpressureBlock) {
		return nil, &types.ValidationError{Field: "websocket", Message: "invalid buffer, subscription cap or backpressure policy"}
	}
	cfg.withDefaults()
	if cfg.PublicURL == "" {
		return nil, &types.ValidationError{Field: "public_ws_url", Message: "required"}
	}
	if cfg.PrivateURL == "" {
		cfg.PrivateURL = cfg.PublicURL
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	lifeCtx, cancel := context.WithCancel(context.Background())
	return &Client{
		ctx:     lifeCtx,
		cfg:     cfg,
		done:    make(chan struct{}),
		lifeCh:  make(chan Event, 64),
		subs:    map[int64]*Subscription{},
		byKey:   map[string]*Subscription{},
		pending: map[string]*pendAck{},
		reqs:    map[string]SubscriptionRequest{},
		books:   map[int64]*bookIntegrity{},
		cancel:  cancel,
	}, nil
}

// Lifecycle yields connection-level advisory events (info,
// errors that reach no subscription, pong). Most events are advisory. A
// KindReset replaces buffered lifecycle events so the latest invalidation is
// retained even for a slow consumer; it is also sent to every active subscription.
func (c *Client) Lifecycle() <-chan Event { return c.lifeCh }

// ConnectPublic opens the public endpoint and optionally sends
// {"event":"conf","flags":N} with FlagSeqAll|FlagChecksum to
// enable book integrity mode.
func (c *Client) ConnectPublic(ctx context.Context, flags int) error {
	if flags & ^(FlagSeqAll|FlagChecksum) != 0 {
		return &types.ValidationError{Field: "flags", Message: "only SEQ_ALL and OB_CHECKSUM are supported"}
	}
	return c.connect(ctx, c.cfg.PublicURL, flags, nil)
}

// ConnectAuth opens the authenticated endpoint and performs the
// signed auth handshake with a FRESH nonce. AuthOptions.Filter
// and DeadManSwitch (dms:4) are strictly opt-in.
func (c *Client) ConnectAuth(ctx context.Context, opts AuthOptions) error {
	if c.cfg.Signer == nil || c.cfg.NonceProvider == nil {
		return types.ErrCredentialsMissing
	}
	return c.connect(ctx, c.cfg.PrivateURL, 0, &opts)
}

func (c *Client) connect(ctx context.Context, url string, flags int, authOpts *AuthOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if authOpts != nil {
		copyOpts := *authOpts
		copyOpts.Filter = append([]string(nil), authOpts.Filter...)
		authOpts = &copyOpts
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return types.ErrClosed
	}
	if c.started {
		c.mu.Unlock()
		return &types.WebSocketError{Op: "connect", Message: "client already connected"}
	}
	c.started = true
	c.authed = authOpts != nil
	c.seqAll = flags&FlagSeqAll != 0
	c.mu.Unlock()

	ready := make(chan error, 1)
	go c.readLoop(url, flags, authOpts, ready)

	select {
	case <-ctx.Done():
		_ = c.Close()
		return ctx.Err()
	case err := <-ready:
		return err
	}
}

// Subscribe registers a public channel subscription and waits
// for the server's subscribed ack (or ctx). The returned
// Subscription starts receiving snapshots, then deltas.
func (c *Client) Subscribe(ctx context.Context, req SubscriptionRequest) (*Subscription, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	key := req.cacheKey()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, types.ErrClosed
	}
	if c.conn == nil || !c.ready {
		c.mu.Unlock()
		return nil, &types.WebSocketError{Op: "subscribe", Message: "not connected"}
	}
	if c.paused {
		c.mu.Unlock()
		return nil, &types.WebSocketError{Op: "subscribe", Message: "server is in maintenance"}
	}
	if _, dup := c.reqs[key]; dup {
		c.mu.Unlock()
		return nil, &types.ValidationError{Field: "channel", Message: "already subscribed to " + key}
	}
	limit := c.cfg.MaxSubscriptions
	if c.authed {
		limit--
	}
	if len(c.reqs) >= limit {
		c.mu.Unlock()
		return nil, &types.ValidationError{Field: "channel", Message: "max subscriptions reached (default 25 — see connection limits)"}
	}
	sub := &Subscription{req: req, events: make(chan Event, c.cfg.EventBuffer)}
	ack := make(chan error, 1)
	c.pending[key] = &pendAck{key: key, req: req, sub: sub, ack: ack}
	c.reqs[key] = req
	c.mu.Unlock()

	if err := c.sendJSONContext(ctx, req.message()); err != nil {
		c.removePending(key)
		sub.closeEvents()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.removePending(key)
		sub.closeEvents()
		return nil, ctx.Err()
	case err := <-ack:
		if err != nil {
			c.removePending(key)
			sub.closeEvents()
			return nil, err
		}
		return sub, nil
	case <-c.done:
		c.removePending(key)
		sub.closeEvents()
		return nil, types.ErrClosed
	}
}

// removePending drops a pending request and its tracked req.
func (c *Client) removePending(key string) {
	c.mu.Lock()
	sub := c.byKey[key]
	if p := c.pending[key]; p != nil && sub == nil {
		sub = p.sub
	}
	delete(c.pending, key)
	delete(c.reqs, key)
	delete(c.byKey, key)
	var id int64
	if sub != nil {
		id = sub.ChanID()
		if id != 0 {
			delete(c.subs, id)
			delete(c.books, id)
		}
	}
	c.mu.Unlock()
	if sub != nil {
		sub.closeEvents()
	}
	if id != 0 {
		_ = c.sendJSON(map[string]any{"event": "unsubscribe", "chanId": id})
	}
}

// Unsubscribe removes a channel subscription and waits for the
// server's unsubscribed ack.
func (c *Client) Unsubscribe(ctx context.Context, chanID int64) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return types.ErrClosed
	}
	_, ok := c.subs[chanID]
	c.mu.Unlock()
	if !ok {
		return &types.ValidationError{Field: "chanId", Message: "unknown channel id"}
	}
	if err := c.sendJSONContext(ctx, map[string]any{"event": "unsubscribe", "chanId": chanID}); err != nil {
		return err
	}
	// Wait briefly for the ack; absence is not fatal — the
	// connection may already be reconnecting.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return types.ErrClosed
		case <-time.After(50 * time.Millisecond):
			c.mu.Lock()
			_, still := c.subs[chanID]
			c.mu.Unlock()
			if !still {
				return nil
			}
		}
	}
	return &types.WebSocketError{Op: "unsubscribe", Message: "acknowledgement timed out"}
}

// SubscribeAccount returns the account-stream subscription on
// the authenticated connection (chanID 0 — os/on/ps/te/ws/n/…
// events land here as AuthEvent). Callable after ConnectAuth;
// repeated calls return the same subscription.
func (c *Client) SubscribeAccount() (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, types.ErrClosed
	}
	if !c.authenticated || c.conn == nil {
		return nil, &types.WebSocketError{Op: "subscribe", Message: "account stream requires ConnectAuth"}
	}
	if s, ok := c.subs[0]; ok {
		return s, nil
	}
	s := &Subscription{
		req:    SubscriptionRequest{Channel: "auth"},
		events: make(chan Event, c.cfg.EventBuffer),
		chanID: 0,
	}
	c.subs[0] = s
	return s, nil
}

// Ping sends {"event":"ping"}; the server's pong arrives on the
// Lifecycle channel as a KindInfo event.
func (c *Client) Ping() error {
	c.mu.Lock()
	c.pingSeq++
	n := c.pingSeq
	c.mu.Unlock()
	return c.sendJSON(map[string]any{"event": "ping", "cid": n})
}

// ----- authenticated channel-0 inputs -----

// SendAuthInput writes a raw [0, stream, nil, payload] frame on
// the authenticated connection. Prefer the typed helpers.
func (c *Client) SendAuthInput(ctx context.Context, stream string, payload any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	authed, paused, generation := c.authenticated && c.ready, c.paused, c.generation
	c.mu.Unlock()
	if !authed {
		return &types.WebSocketError{Op: "write", Message: "auth inputs require ConnectAuth"}
	}
	if paused {
		return &types.WebSocketError{Op: "write", Message: "server is in maintenance"}
	}
	return c.sendJSONGeneration(ctx, []any{0, stream, nil, payload}, generation, true)
}

// OrderNew sends the "on" input — a new order. Reuses
// orders.SubmitRequest validation/body; never reordered or
// retried by the client.
func (c *Client) OrderNew(ctx context.Context, req orders.SubmitRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	return c.SendAuthInput(ctx, "on", req.Body())
}

// OrderUpdate sends the "ou" input.
func (c *Client) OrderUpdate(ctx context.Context, req orders.UpdateRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	return c.SendAuthInput(ctx, "ou", req.Body())
}

// OrderCancel sends the "oc" input.
func (c *Client) OrderCancel(ctx context.Context, req orders.CancelRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	return c.SendAuthInput(ctx, "oc", req.Body())
}

// OrderCancelMulti sends the "oc_multi" input.
func (c *Client) OrderCancelMulti(ctx context.Context, req orders.CancelMultiRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	return c.SendAuthInput(ctx, "oc_multi", req.Body())
}

// OrderMulti sends the "ox_multi" input — combined ops.
func (c *Client) OrderMulti(ctx context.Context, ops []orders.MultiOp) error {
	if len(ops) == 0 || len(ops) > 75 {
		return &types.ValidationError{Field: "ops", Message: "multi requires 1..75 operations"}
	}
	raw := make([]any, len(ops))
	for i, op := range ops {
		if err := op.Validate(); err != nil {
			return err
		}
		raw[i] = []any{op.Op, op.Args}
	}
	return c.SendAuthInput(ctx, "ox_multi", raw)
}

// OfferNew sends the "fon" input — a new funding offer.
func (c *Client) OfferNew(ctx context.Context, req funding.OfferRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	return c.SendAuthInput(ctx, "fon", req.Body())
}

// OfferCancel sends the "foc" input.
func (c *Client) OfferCancel(ctx context.Context, offerID int64) error {
	if offerID <= 0 {
		return &types.ValidationError{Field: "id", Message: "offer id must be positive"}
	}
	return c.SendAuthInput(ctx, "foc", map[string]any{"id": offerID})
}

// Calc sends the "calc" input — e.g. Calc(ctx, []string{"margin_base"}, []string{"balance"}).
func (c *Client) Calc(ctx context.Context, requests ...[]string) error {
	if len(requests) == 0 {
		return &types.ValidationError{Field: "calc", Message: "at least one calc request required"}
	}
	inner := make([]any, len(requests))
	for i, r := range requests {
		inner[i] = r
	}
	return c.SendAuthInput(ctx, "calc", inner)
}

// Close releases the connection and all goroutines/subscription
// channels. Idempotent.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	started := c.started
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	c.cancel()
	if conn != nil {
		_ = conn.Close()
	}
	if started {
		<-c.done // readLoop closes done on exit
	} else {
		close(c.done)
		close(c.lifeCh)
	}

	c.mu.Lock()
	for _, s := range c.subs {
		s.closeEvents()
	}
	c.subs = map[int64]*Subscription{}
	c.byKey = map[string]*Subscription{}
	c.mu.Unlock()
	return nil
}

// sendJSON serializes and writes a frame under the write lock.
func (c *Client) sendJSON(v any) error {
	return c.sendJSONContext(context.Background(), v)
}

func (c *Client) sendJSONContext(ctx context.Context, v any) error {
	return c.sendJSONGeneration(ctx, v, 0, false)
}

func (c *Client) sendJSONGeneration(ctx context.Context, v any, generation uint64, authInput bool) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c.writeOnce.Do(func() { c.writeGate = make(chan struct{}, 1) })
	select {
	case c.writeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.writeGate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return &types.WebSocketError{Op: "write", Message: "encode failed", Err: err}
	}
	c.mu.Lock()
	conn := c.conn
	connectionGeneration := c.generation
	closed := c.closed
	usable := !authInput || (c.generation == generation && c.authenticated && c.ready && !c.paused)
	c.mu.Unlock()
	if closed || conn == nil {
		return types.ErrClosed
	}
	if !usable {
		return &types.WebSocketError{Op: "write", Message: "connection state changed before authenticated send"}
	}
	if d, ok := conn.(interface{ SetWriteDeadline(time.Time) error }); ok {
		deadline, _ := ctx.Deadline()
		if err := d.SetWriteDeadline(deadline); err != nil {
			c.failWriteConnection(conn, connectionGeneration)
			return &types.WebSocketError{Op: "write", Message: "set deadline failed", Err: err}
		}
	}
	stop := context.AfterFunc(ctx, func() { c.failWriteConnection(conn, connectionGeneration) })
	defer stop()
	if err := conn.WriteMessage(1 /* text */, b); err != nil {
		c.failWriteConnection(conn, connectionGeneration)
		return &types.WebSocketError{Op: "write", Message: "send failed", Err: err}
	}
	return nil
}

// A failed write is terminal for the socket. Closing it wakes the read loop,
// which owns reset delivery and reconnect. An old writer must not invalidate a
// newer connection, and the failed command is never replayed.
func (c *Client) failWriteConnection(conn Conn, generation uint64) {
	c.mu.Lock()
	if c.generation == generation {
		c.conn = nil
		c.ready = false
		c.authenticated = false
	}
	c.mu.Unlock()
	_ = conn.Close()
}
