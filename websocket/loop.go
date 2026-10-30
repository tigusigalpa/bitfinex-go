package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/internal/authflow"

	"github.com/tigusigalpa/bitfinex-go/internal/wire"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// readLoop owns the connection lifecycle: dial → handshake →
// (auth) → dispatch → reconnect with capped full-jitter backoff.
// It exits only on Close or an unrecoverable handshake failure
// (reported through ready on the first attempt).
func (c *Client) readLoop(url string, flags int, authOpts *AuthOptions, ready chan<- error) {
	defer close(c.done)
	defer close(c.lifeCh)

	attempt := 0
	for {
		attempt++
		generationStart := time.Now()
		first := attempt == 1
		var firstReady chan<- error
		if first {
			firstReady = ready
		}
		err := c.runConn(url, flags, authOpts, first, firstReady)
		c.mu.Lock()
		everReady := c.everReady
		c.mu.Unlock()
		if first && err != nil && !everReady {
			return // unrecoverable first-attempt failure
		}

		c.mu.Lock()
		stopped := c.closed
		c.mu.Unlock()
		if stopped {
			return
		}
		if everReady {
			c.invalidateConnection(err)
		}

		// Reconnect with capped full-jitter backoff.
		delay := reconnectDelay(c.cfg.ReconnectBase, c.cfg.ReconnectCap, attempt)
		// At most one new connection every three seconds per client.
		if floor := 3*time.Second - time.Since(generationStart); delay < floor {
			delay = floor
		}
		c.cfg.Logger.Debug("websocket reconnecting", "url", safeURL(url), "attempt", attempt, "delay", delay.String())
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-c.ctx.Done():
			timer.Stop()
			return
		}
	}
}

// runConn executes one connection generation: dial, handshake,
// optional auth, optional conf, resubscribe, then read until
// failure. On the FIRST attempt it reports the handshake outcome
// through ready (nil once the read loop is entered); later
// attempts signal readiness the same way.
func (c *Client) runConn(url string, flags int, authOpts *AuthOptions, first bool, ready chan<- error) error {
	report := func(err error) error {
		if ready != nil {
			ready <- err
		}
		return err
	}

	ctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
	defer cancel()

	conn, err := c.cfg.Dialer.Dial(ctx, url)
	if err != nil {
		return report(&types.WebSocketError{Op: "dial", Message: "dial failed", Err: err})
	}
	defer conn.Close()
	stopHandshake := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopHandshake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return report(types.ErrClosed)
	}
	c.conn = conn
	c.generation++
	c.seqInit = false
	c.paused = false
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.conn = nil; c.ready = false; c.authenticated = false; c.mu.Unlock() }()

	// 1) expect the {"event":"info"} handshake.
	info, err := c.readFrame(conn)
	if err != nil {
		conn.Close()
		return report(err)
	}
	if err := c.handleInfoHandshake(info); err != nil {
		conn.Close()
		return report(err)
	}

	// 2) auth handshake — fresh nonce on EVERY (re)connect.
	if authOpts != nil {
		if err := c.authenticate(ctx, conn, authOpts); err != nil {
			conn.Close()
			return report(err)
		}
		c.mu.Lock()
		c.authenticated = true
		c.mu.Unlock()
		// Install channel zero before any account snapshots arrive.
		if _, err := c.SubscribeAccount(); err != nil {
			return report(err)
		}
	}

	// 3) optional conf flags (public integrity mode).
	if flags != 0 {
		if err := c.sendJSON(map[string]any{"event": "conf", "flags": flags}); err != nil {
			conn.Close()
			return report(err)
		}
	}

	// 4) resubscribe every tracked channel (no-op on first
	// connect — reqs is empty then).
	if !first {
		c.resubscribeAll()
	}

	// Handshake complete — only the read loop remains.
	if !stopHandshake() || ctx.Err() != nil {
		return report(ctx.Err())
	}
	c.mu.Lock()
	c.ready = true
	c.everReady = true
	c.mu.Unlock()
	report(nil)

	// 5) read/dispatch until failure.
	for {
		msg, err := c.readFrame(conn)
		if err != nil {
			conn.Close()
			c.mu.Lock()
			closed := c.closed
			c.conn = nil
			c.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		c.dispatch(msg)
	}
}

// readFrame reads one text frame and decodes it.
func (c *Client) readFrame(conn Conn) (any, error) {
	if d, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = d.SetReadDeadline(time.Now().Add(45 * time.Second))
	}
	mt, raw, err := conn.ReadMessage()
	if err != nil {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return nil, types.ErrClosed
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, &types.WebSocketError{Op: "read", Message: "read timeout", Err: err}
		}
		return nil, &types.WebSocketError{Op: "read", Message: "connection lost", Err: err}
	}
	if mt != 1 && mt != 2 { // not text/binary
		return nil, &types.WebSocketError{Op: "read", Message: "unexpected frame type"}
	}
	if len(raw) > 4<<20 {
		return nil, &types.ProtocolError{Message: "WebSocket frame exceeded 4 MiB limit"}
	}
	v, err := wire.Decode(raw)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// handleInfoHandshake validates the mandatory {"event":"info"}
// first frame.
func (c *Client) handleInfoHandshake(msg any) error {
	m, ok := msg.(map[string]any)
	if !ok {
		return &types.ProtocolError{Message: "expected info handshake object"}
	}
	if ev, _ := m["event"].(string); ev != "info" {
		return &types.ProtocolError{Message: "expected info event"}
	}
	if code, ok := m["code"].(json.Number); ok {
		// Server-driven info codes inside the handshake are
		// handled by handleEvent for consistency.
		_ = code
	}
	if pf, ok := m["platform"].(map[string]any); ok {
		if st, ok := pf["status"].(json.Number); ok && st.String() == "0" {
			return &types.WebSocketError{Op: "connect", Message: "platform in maintenance (status=0)"}
		}
	}
	return nil
}

// authenticate orders this handshake with REST and other WS auth on the key.
func (c *Client) authenticate(ctx context.Context, conn Conn, opts *AuthOptions) error {
	release, err := authflow.Acquire(ctx, c.cfg.Signer.Key())
	if err != nil {
		return err
	}
	defer release()
	if err = c.sendAuth(ctx, opts); err != nil {
		return err
	}
	return c.awaitAuthOK(conn)
}

// sendAuth emits the signed auth frame with a fresh nonce.
func (c *Client) sendAuth(ctx context.Context, opts *AuthOptions) error {
	nonce, err := c.cfg.NonceProvider.Next(ctx)
	if err != nil {
		return &types.NonceError{Message: "nonce provider failed for ws auth", Err: err}
	}
	if err := auth.ValidateNonce(nonce); err != nil {
		return err
	}
	msg := map[string]any{
		"event":       "auth",
		"apiKey":      c.cfg.Signer.Key(),
		"authSig":     c.cfg.Signer.SignWebSocketAuth(nonce),
		"authPayload": auth.AuthPayload(nonce),
		"authNonce":   nonce,
	}
	if len(opts.Filter) > 0 {
		msg["filter"] = opts.Filter
	}
	if opts.DeadManSwitch {
		msg["dms"] = 4
	}
	return c.sendJSONContext(ctx, msg)
}

// awaitAuthOK reads frames until the auth ack arrives.
func (c *Client) awaitAuthOK(conn Conn) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := c.readFrame(conn)
		if err != nil {
			return err
		}
		m, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		switch ev, _ := m["event"].(string); ev {
		case "auth":
			status, _ := m["status"].(string)
			if status != "OK" {
				return &types.AuthenticationError{APIError: types.APIError{
					Message: "websocket authentication rejected",
				}}
			}
			return nil
		case "error":
			return wsErrorEvent(m)
		}
	}
	return &types.WebSocketError{Op: "auth", Message: "auth ack timed out"}
}

// dispatch routes one decoded frame.
func (c *Client) dispatch(msg any) {
	// Event objects {event: ...}
	if m, ok := msg.(map[string]any); ok {
		c.handleEvent(m)
		return
	}

	arr, ok := msg.([]any)
	if !ok || len(arr) < 2 {
		c.emitLifecycle(Event{Kind: KindError, Err: &types.ProtocolError{Message: "WS frame has neither event nor chanId"}})
		return
	}
	chanID, validID := numeric(arr[0])
	if !validID || chanID < 0 {
		c.emitLifecycle(Event{Kind: KindError, Err: &types.ProtocolError{Message: "invalid channel id"}})
		return
	}
	payload := arr[1]
	c.mu.Lock()
	seqAll := c.seqAll
	c.mu.Unlock()
	if seqAll {
		_, tagged := payload.(string)
		minimum := 3
		if tagged && payload != "hb" {
			minimum = 4
		}
		if _, ok := numeric(arr[len(arr)-1]); !ok || len(arr) < minimum {
			c.deliverError(chanID, &types.ProtocolError{Message: "missing sequence number in SEQ_ALL frame"})
			c.resyncChannel(chanID)
			return
		}
	}

	// [chanId, "hb", (seq)] heartbeat.
	if tag, ok := payload.(string); ok && tag == "hb" {
		if len(arr) >= 3 {
			if seq, isNum := numeric(arr[2]); isNum {
				if gap := c.checkSeq(chanID, seq); gap != nil {
					c.deliverError(chanID, gap)
					c.resyncChannel(chanID)
					return
				}
			}
		}
		c.deliver(chanID, Event{Kind: KindHeartbeat, ChanID: chanID})
		return
	}

	// [chanId, "cs", checksum, (seq)] checksum frame — the
	// checksum value is itself numeric, so this branch must be
	// checked BEFORE generic seq stripping.
	if tag, ok := payload.(string); ok && tag == "cs" && len(arr) >= 3 {
		if len(arr) >= 4 {
			if seq, isNum := numeric(arr[3]); isNum {
				if gap := c.checkSeq(chanID, seq); gap != nil {
					c.deliverError(chanID, gap)
					c.resyncChannel(chanID)
					return
				}
			}
		}
		c.handleChecksum(chanID, arr[2])
		return
	}

	// Generic trailing sequence (SEQ_ALL): [id, payload, seq]
	// for untagged data, [id, tag, payload, seq] for tagged.
	if len(arr) >= 3 {
		if seq, isNum := numeric(arr[len(arr)-1]); isNum {
			_, secondIsArr := arr[1].([]any)
			_, secondIsStr := arr[1].(string)
			if secondIsArr || (secondIsStr && len(arr) >= 4) {
				arr = arr[:len(arr)-1]
				if gap := c.checkSeq(chanID, seq); gap != nil {
					c.deliverError(chanID, gap)
					c.resyncChannel(chanID)
					return
				}
			}
		}
	}

	if len(arr) < 2 {
		return
	}
	payload = arr[1]

	// [chanId, tag, payload] tagged updates (te/tu/fte/ftu and
	// every auth stream).
	if tag, ok := payload.(string); ok && len(arr) >= 3 {
		c.dispatchTagged(chanID, tag, arr[2])
		return
	}

	// [chanId, payload] untagged channel data.
	c.dispatchUntagged(chanID, payload)
}

// handleEvent processes {event:...} objects.
func (c *Client) handleEvent(m map[string]any) {
	ev, _ := m["event"].(string)
	switch ev {
	case "info":
		code := int64From(m["code"])
		switch code {
		case infoHaltReconnect, infoMaintenanceEnd:
			c.emitLifecycle(Event{Kind: KindInfo, Action: "reconnect", Raw: m})
			c.mu.Lock()
			conn := c.conn
			c.conn = nil
			c.mu.Unlock()
			if conn != nil {
				conn.Close()
			}
		case infoMaintenanceStart:
			c.mu.Lock()
			c.paused = true
			c.mu.Unlock()
			c.emitLifecycle(Event{Kind: KindInfo, Action: "paused", Raw: m})
		default:
			c.emitLifecycle(Event{Kind: KindInfo, Action: "info", Raw: m})
		}
	case "subscribed":
		c.handleSubscribed(m)
	case "unsubscribed":
		chanID := int64From(m["chanId"])
		c.mu.Lock()
		sub := c.subs[chanID]
		if sub != nil {
			delete(c.subs, chanID)
			delete(c.byKey, sub.req.cacheKey())
			delete(c.reqs, sub.req.cacheKey())
			delete(c.books, chanID)
		}
		c.mu.Unlock()
		if sub != nil {
			c.deliverTo(sub, Event{Kind: KindInfo, ChanID: chanID, Action: "unsubscribed"})
			sub.closeEvents()
		}
	case "conf":
		c.emitLifecycle(Event{Kind: KindInfo, Action: "conf", Raw: m})
	case "pong":
		c.emitLifecycle(Event{Kind: KindInfo, Action: "pong", Raw: m})
	case "error":
		err := wsErrorEvent(m)
		chanID := int64From(m["chanId"])
		if chanID != 0 {
			c.deliverError(chanID, err)
		} else {
			c.routePendingError(err)
			c.emitLifecycle(Event{Kind: KindError, Err: err})
		}
	case "auth":
		c.emitLifecycle(Event{Kind: KindInfo, Action: "auth", Raw: m})
	default:
		c.emitLifecycle(Event{Kind: KindInfo, Action: ev, Raw: m})
	}
}

// handleSubscribed matches the ack to a pending request and
// binds the chanID.
func (c *Client) handleSubscribed(m map[string]any) {
	chanID := int64From(m["chanId"])
	channel, _ := m["channel"].(string)
	sym, _ := m["symbol"].(string)
	key, _ := m["key"].(string)
	prec, _ := m["prec"].(string)
	freq, _ := m["freq"].(string)

	c.mu.Lock()
	var matchedKey string
	var matched *pendAck
	for k, p := range c.pending {
		r := p.req
		if r.Channel != channel {
			continue
		}
		if r.Symbol != "" && r.Symbol != sym {
			continue
		}
		if r.Key != "" && r.Key != key {
			continue
		}
		if r.Prec != "" && prec != "" && r.Prec != prec {
			continue
		}
		if r.Freq != "" && freq != "" && r.Freq != freq {
			continue
		}
		if length := int64From(m["len"]); r.Len > 0 && length > 0 && int64(r.Len) != length {
			continue
		}
		matchedKey = k
		matched = p
		break
	}
	if matched == nil {
		// A late ack for a cancelled subscription must not create an orphan.
		c.mu.Unlock()
		_ = c.sendJSON(map[string]any{"event": "unsubscribe", "chanId": chanID})
		return
	}
	sub := matched.sub
	sub.mu.Lock()
	oldChan := sub.chanID
	sub.chanID = chanID
	sub.mu.Unlock()
	if oldChan != 0 && oldChan != chanID {
		delete(c.subs, oldChan)
		delete(c.books, oldChan)
	}
	c.subs[chanID] = sub
	c.byKey[matched.req.cacheKey()] = sub
	if channel == "book" {
		c.books[chanID] = newBookIntegrity()
	}
	delete(c.pending, matchedKey)
	ack := matched.ack
	c.mu.Unlock()

	c.deliverTo(sub, Event{Kind: KindInfo, ChanID: chanID, Channel: channel, Symbol: sym, Action: "subscribed", Raw: m})
	if ack != nil {
		select {
		case ack <- nil:
		default:
		}
	}
}

// resubscribeAll re-arms every tracked subscription on the new
// connection (books are rebuilt from fresh snapshots).
func (c *Client) resubscribeAll() {
	c.mu.Lock()
	reqs := make([]SubscriptionRequest, 0, len(c.reqs))
	for k, r := range c.reqs {
		reqs = append(reqs, r)
		if p := c.pending[k]; p != nil {
			continue
		}
		if sub := c.byKey[k]; sub != nil {
			c.pending[k] = &pendAck{key: k, req: r, sub: sub}
		}
	}
	account := c.subs[0]
	c.subs = map[int64]*Subscription{}
	if account != nil {
		c.subs[0] = account
	}
	c.books = map[int64]*bookIntegrity{}
	c.seqInit = false
	c.mu.Unlock()

	for _, r := range reqs {
		if err := c.sendJSON(r.message()); err != nil {
			c.emitLifecycle(Event{Kind: KindError, Err: err})
		}
	}
}

// resyncChannel drops and re-subscribes one channel after a
// sequence gap or checksum mismatch.
func (c *Client) resyncChannel(chanID int64) {
	c.mu.Lock()
	conn := c.conn
	c.books = map[int64]*bookIntegrity{}
	c.mu.Unlock()
	// Sequence numbers cover the connection, so every channel may be stale.
	if conn != nil {
		_ = conn.Close()
	}
}

// checkSeq tracks per-connection sequence numbers; a gap returns
// a SequenceGapError.
func (c *Client) checkSeq(chanID, seq int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.seqAll {
		return nil
	}
	if !c.seqInit {
		c.seqInit = true
		c.lastSeq = seq
		return nil
	}
	if seq != c.lastSeq+1 {
		gap := &types.SequenceGapError{ChanID: chanID, Expected: c.lastSeq + 1, Got: seq}
		c.lastSeq = seq
		return gap
	}
	c.lastSeq = seq
	return nil
}

// handleChecksum verifies the local book CRC32.
func (c *Client) handleChecksum(chanID int64, rawCS any) {
	server := int64From(rawCS)
	c.mu.Lock()
	book := c.books[chanID]
	sub := c.subs[chanID]
	raw := sub != nil && sub.req.Prec == "R0"
	c.mu.Unlock()
	if book == nil {
		return
	}
	if !book.hasSnapshot {
		c.deliverError(chanID, &types.ProtocolError{Message: "book checksum arrived before a fresh snapshot"})
		c.resyncChannel(chanID)
		return
	}
	if book.verify(server, raw) {
		c.deliver(chanID, Event{Kind: KindChecksum, ChanID: chanID})
		return
	}
	c.deliverError(chanID, &types.ChecksumError{ChanID: chanID, Expected: int64(book.checksum(raw)), Got: server})
	c.resyncChannel(chanID)
}

// applyLevel mutates the integrity book from one row.
func applyLevel(b *bookIntegrity, row []any, isRaw, isFunding bool) {
	if isFunding {
		return
	}
	if isRaw {
		id := int64From(wire.At(row, 0))
		price := decLexeme(wire.At(row, 1))
		amount := decLexeme(wire.At(row, 2))
		b.applyRaw(id, price, amount)
		return
	}
	if isFunding {
		// Funding books are not part of the checksum spec.
		return
	}
	price := decLexeme(wire.At(row, 0))
	count := int64From(wire.At(row, 1))
	amount := decLexeme(wire.At(row, 2))
	b.applyPrec(price, count, amount)
}

// decodeEntityList decodes either a single entity row or a
// snapshot list of rows.
func decodeEntityList[T any](payload any, dec func([]any) (T, error)) ([]T, error) {
	arr, ok := payload.([]any)
	if !ok {
		return nil, &types.ProtocolError{Message: "expected entity array payload"}
	}
	if len(arr) == 0 {
		return []T{}, nil
	}
	if _, nested := arr[0].([]any); nested {
		out := make([]T, 0, len(arr))
		for _, item := range arr {
			row, ok := item.([]any)
			if !ok {
				return nil, &types.ProtocolError{Message: "invalid entity snapshot row"}
			}
			v, err := dec(row)
			if err != nil {
				return out, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	v, err := dec(arr)
	if err != nil {
		return nil, err
	}
	return []T{v}, nil
}

// routePendingError forwards a server error to waiting
// subscribe callers.
func (c *Client) routePendingError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.pending {
		if p.ack != nil {
			select {
			case p.ack <- err:
			default:
			}
		}
	}
}

// emitLifecycle pushes an advisory event to the lifecycle
// channel (non-blocking; advisory events may coalesce under
// pressure — subscription data is never dropped this way).
func (c *Client) emitLifecycle(ev Event) {
	if ev.Generation == 0 {
		c.mu.Lock()
		ev.Generation = c.generation
		c.mu.Unlock()
	}
	if ev.Kind == KindReset {
		// Connection state replaces older advisory state. The read loop owns
		// lifecycle closure, so this never sends concurrently with close.
		for {
			select {
			case <-c.lifeCh:
				continue
			default:
			}
			break
		}
	}
	select {
	case c.lifeCh <- ev:
	default:
	}
}

// deliver routes an event to the subscription bound to chanID.
func (c *Client) deliver(chanID int64, ev Event) {
	c.mu.Lock()
	sub := c.subs[chanID]
	c.mu.Unlock()
	if sub != nil {
		c.deliverTo(sub, ev)
	}
}

// deliverError delivers a KindError event.
func (c *Client) deliverError(chanID int64, err error) {
	c.deliver(chanID, Event{Kind: KindError, ChanID: chanID, Err: err})
}

// deliverTo enforces the BackpressurePolicy — messages are
// never silently dropped. The wg protocol prevents
// send-on-closed panics.
func (c *Client) deliverTo(sub *Subscription, ev Event) {
	c.mu.Lock()
	ev.Generation = c.generation
	c.mu.Unlock()
	stop := sub.deliveryDone()
	sub.mu.Lock()
	slow := false
	defer func() {
		sub.mu.Unlock()
		if slow {
			c.mu.Lock()
			delete(c.subs, sub.chanID)
			delete(c.books, sub.chanID)
			delete(c.byKey, sub.req.cacheKey())
			delete(c.reqs, sub.req.cacheKey())
			delete(c.pending, sub.req.cacheKey())
			c.mu.Unlock()
			_ = c.sendJSON(map[string]any{"event": "unsubscribe", "chanId": sub.chanID})
		}
	}()
	if sub.closed {
		return
	}

	if c.cfg.Backpressure == BackpressureBlock {
		select {
		case sub.events <- ev:
		case <-c.ctx.Done():
		case <-stop:
		}
		return
	}
	select {
	case sub.events <- ev:
	default:
		// Reserve space for a terminal failure. No goroutine waits on a slow
		// consumer, and the failure explicitly invalidates the queued data.
		select {
		case <-sub.events:
		default:
		}
		sub.events <- Event{Kind: KindError, ChanID: sub.chanID, Err: types.ErrConsumerTooSlow}
		sub.err = types.ErrConsumerTooSlow
		sub.closed = true
		sub.stopDelivery()
		slow = true
		close(sub.events)
	}
}

// invalidateConnection runs before reconnect/backoff. It cannot block on a
// full event buffer: old-generation events are replaced by an explicit reset.
func (c *Client) invalidateConnection(cause error) {
	c.mu.Lock()
	c.ready = false
	c.authenticated = false
	c.books = map[int64]*bookIntegrity{}
	generation := c.generation
	seen := map[*Subscription]bool{}
	for _, s := range c.subs {
		seen[s] = true
	}
	for _, p := range c.pending {
		if p.sub != nil {
			seen[p.sub] = true
		}
	}
	c.mu.Unlock()
	if cause == nil {
		cause = &types.WebSocketError{Op: "read", Message: "connection closed"}
	}
	ev := Event{Kind: KindReset, Action: "disconnected", Generation: generation, Err: cause}
	for s := range seen {
		s.mu.Lock()
		if !s.closed {
			for {
				select {
				case <-s.events:
					continue
				default:
				}
				break
			}
			reset := ev
			reset.ChanID = s.chanID
			reset.Channel = s.req.Channel
			reset.Symbol = s.req.Symbol
			select {
			case s.events <- reset:
			default:
			}
		}
		s.mu.Unlock()
	}
	c.emitLifecycle(ev)
}

// reconnectDelay returns full-jitter backoff:
// uniform in [0, min(cap, base*2^(attempt-1))].
func reconnectDelay(base, cap time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base << (attempt - 1)
	if d <= 0 || d > cap {
		d = cap
	}
	if d <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(d)))
}

// numeric extracts an int64 from a json.Number-bearing value.
func numeric(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	if err != nil {
		return 0, false
	}
	return i, true
}

// int64From coerces json.Number/string/nil to int64.
func int64From(v any) int64 {
	switch n := v.(type) {
	case nil:
		return 0
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

// decLexeme extracts the exact decimal lexeme of a wire value.
func decLexeme(v any) string {
	switch n := v.(type) {
	case nil:
		return "0"
	case json.Number:
		return n.String()
	case string:
		if n == "" {
			return "0"
		}
		return n
	default:
		return "0"
	}
}

// wsErrorEvent builds a typed error from a server error object.
func wsErrorEvent(m map[string]any) error {
	code := int64From(m["code"])
	return &types.WebSocketError{Op: "event", Message: "server rejected event (code " + itoa64(code) + ")"}
}

func safeURL(u string) string { return "[websocket endpoint]" }
