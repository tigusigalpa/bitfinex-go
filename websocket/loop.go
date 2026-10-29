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
	"github.com/tigusigalpa/bitfinex-go/funding"
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/market"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/types"
	"github.com/tigusigalpa/bitfinex-go/wallets"
)

// readLoop owns the connection lifecycle: dial → handshake →
// (auth) → dispatch → reconnect with capped full-jitter backoff.
// It exits only on Close or an unrecoverable handshake failure
// (reported through ready on the first attempt).
func (c *Client) readLoop(url string, flags int, authOpts *AuthOptions, ready chan<- error) {
	defer close(c.done)

	attempt := 0
	for {
		attempt++
		first := attempt == 1
		err := c.runConn(url, flags, authOpts, first, ready)
		if first && err != nil {
			return // unrecoverable first-attempt failure
		}

		c.mu.Lock()
		stopped := c.closed
		c.mu.Unlock()
		if stopped {
			return
		}

		// Reconnect with capped full-jitter backoff.
		delay := reconnectDelay(c.cfg.ReconnectBase, c.cfg.ReconnectCap, attempt)
		c.cfg.Logger.Debug("websocket reconnecting", "url", safeURL(url), "attempt", attempt, "delay", delay.String())
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-c.done:
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := c.cfg.Dialer.Dial(ctx, url)
	if err != nil {
		return report(&types.WebSocketError{Op: "dial", Message: "dial failed", Err: err})
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

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
		if err := c.sendAuth(authOpts); err != nil {
			conn.Close()
			return report(err)
		}
		if err := c.awaitAuthOK(conn); err != nil {
			conn.Close()
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
		return &types.ProtocolError{Message: "expected info event, got " + ev}
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

// sendAuth emits the signed auth frame with a fresh nonce.
func (c *Client) sendAuth(opts *AuthOptions) error {
	nonce, err := c.cfg.NonceProvider.Next(context.Background())
	if err != nil {
		return &types.NonceError{Message: "nonce provider failed for ws auth", Err: err}
	}
	msg := map[string]any{
		"event":       "auth",
		"apiKey":      c.cfg.Signer.Key(),
		"authSig":     c.cfg.Signer.SignWebSocketAuth(nonce),
		"authPayload": auth.AuthPayload(nonce),
	}
	if len(opts.Filter) > 0 {
		msg["filter"] = opts.Filter
	}
	if opts.DeadManSwitch {
		msg["dms"] = 4
	}
	return c.sendJSON(msg)
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
					Message: "websocket auth rejected: " + status,
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
	chanID := int64From(arr[0])
	payload := arr[1]

	// [chanId, "hb", (seq)] heartbeat.
	if tag, ok := payload.(string); ok && tag == "hb" {
		if len(arr) >= 3 {
			if seq, isNum := numeric(arr[2]); isNum {
				if gap := c.checkSeq(chanID, seq); gap != nil {
					c.deliverError(chanID, gap)
					c.resyncChannel(chanID)
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
		matchedKey = k
		matched = p
		break
	}
	if matched == nil {
		// Unknown ack — build a generic sub so events aren't lost.
		req := SubscriptionRequest{Channel: channel, Symbol: sym, Key: key, Prec: prec, Freq: freq}
		matchedKey = req.cacheKey()
		matched = &pendAck{key: matchedKey, req: req, sub: &Subscription{req: req, events: make(chan Event, c.cfg.EventBuffer)}}
		c.reqs[matchedKey] = req
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
		c.pending[k] = &pendAck{key: k, req: r, sub: c.byKey[k]}
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
	sub := c.subs[chanID]
	c.mu.Unlock()
	if sub == nil {
		return
	}
	_ = c.sendJSON(map[string]any{"event": "unsubscribe", "chanId": chanID})
	c.mu.Lock()
	delete(c.subs, chanID)
	delete(c.books, chanID)
	key := sub.req.cacheKey()
	c.pending[key] = &pendAck{key: key, req: sub.req, sub: sub}
	c.mu.Unlock()
	if err := c.sendJSON(sub.req.message()); err != nil {
		c.deliverTo(sub, Event{Kind: KindError, ChanID: chanID, Err: err})
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
	if seq == c.lastSeq {
		return nil // duplicate
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
	if book.verify(server, raw) {
		c.deliver(chanID, Event{Kind: KindChecksum, ChanID: chanID})
		return
	}
	c.deliverError(chanID, &types.ChecksumError{ChanID: chanID, Expected: int64(book.checksum(raw)), Got: server})
	c.resyncChannel(chanID)
}

// dispatchTagged handles [chanId, tag, payload] frames.
func (c *Client) dispatchTagged(chanID int64, tag string, payload any) {
	if chanID == 0 {
		c.dispatchAuthStream(tag, payload)
		return
	}
	c.mu.Lock()
	sub := c.subs[chanID]
	c.mu.Unlock()
	if sub == nil {
		return
	}
	ev := Event{Kind: KindUpdate, ChanID: chanID, Channel: sub.req.Channel, Symbol: sub.req.Symbol, Raw: payload}
	switch tag {
	case "te", "tu":
		if arr, ok := payload.([]any); ok {
			sym, _ := types.ParseSymbol(sub.req.Symbol)
			if tr, err := marketTradeFromWire(arr, sym); err == nil {
				ev.Trades = []market.Trade{tr}
			}
		}
	case "fte", "ftu":
		if arr, ok := payload.([]any); ok {
			if ft, err := funding.TradeFromWire(arr); err == nil {
				ev.FundingTrades = []funding.FundingTrade{ft}
			}
		}
	}
	c.deliverTo(sub, ev)
}

// dispatchUntagged handles [chanId, payload] channel data:
// snapshots then deltas, per channel type.
func (c *Client) dispatchUntagged(chanID int64, payload any) {
	c.mu.Lock()
	sub := c.subs[chanID]
	c.mu.Unlock()
	if sub == nil {
		return
	}
	arr, ok := payload.([]any)
	if !ok {
		return
	}

	switch sub.req.Channel {
	case "ticker":
		sym, _ := types.ParseSymbol(sub.req.Symbol)
		tk, err := marketTickerFromWire(arr, sym)
		if err != nil {
			c.deliverTo(sub, Event{Kind: KindError, ChanID: chanID, Err: err})
			return
		}
		c.deliverTo(sub, Event{Kind: KindUpdate, ChanID: chanID, Channel: "ticker", Symbol: sub.req.Symbol, Ticker: &tk, Raw: payload})

	case "trades":
		// payload is a list of trade rows (snapshot batch).
		sym, _ := types.ParseSymbol(sub.req.Symbol)
		trades := []market.Trade{}
		rows, err := wire.Rows(payload, "ws trades")
		if err == nil {
			for _, row := range rows {
				if tr, derr := marketTradeFromWire(row, sym); derr == nil {
					trades = append(trades, tr)
				}
			}
		}
		c.deliverTo(sub, Event{Kind: KindSnapshot, ChanID: chanID, Channel: "trades", Symbol: sub.req.Symbol, Trades: trades, Raw: payload})

	case "book":
		c.handleBookData(sub, chanID, arr, payload)

	case "candles":
		out := []market.Candle{}
		if rows, err := wire.Rows(payload, "ws candles"); err == nil && len(rows) > 0 {
			for _, row := range rows {
				if cd, derr := candleFromWire(row); derr == nil {
					out = append(out, cd)
				}
			}
		} else {
			if cd, derr := candleFromWire(arr); derr == nil {
				out = append(out, cd)
			}
		}
		c.deliverTo(sub, Event{Kind: KindUpdate, ChanID: chanID, Channel: "candles", Candles: out, Raw: payload})

	default: // status and anything else → raw passthrough
		c.deliverTo(sub, Event{Kind: KindUpdate, ChanID: chanID, Channel: sub.req.Channel, Status: payload, Raw: payload})
	}
}

// handleBookData applies snapshot/delta to the local integrity
// book and emits typed book events.
func (c *Client) handleBookData(sub *Subscription, chanID int64, arr []any, payload any) {
	c.mu.Lock()
	book := c.books[chanID]
	c.mu.Unlock()

	isRaw := sub.req.Prec == "R0"
	isFunding := len(sub.req.Symbol) > 0 && sub.req.Symbol[0] == 'f'

	// Snapshot: array of rows.
	if len(arr) > 0 {
		if _, nested := arr[0].([]any); nested {
			levels := []market.BookLevel{}
			if book != nil {
				book.clear()
			}
			for _, item := range arr {
				row, ok := item.([]any)
				if !ok {
					continue
				}
				lvl := decodeBookLevel(row, isRaw, isFunding)
				levels = append(levels, lvl)
				if book != nil {
					applyLevel(book, row, isRaw, isFunding)
				}
			}
			c.deliverTo(sub, Event{Kind: KindSnapshot, ChanID: chanID, Channel: "book", Symbol: sub.req.Symbol, BookSnapshot: levels, Raw: payload})
			return
		}
	}

	// Delta: single level row.
	lvl := decodeBookLevel(arr, isRaw, isFunding)
	if book != nil {
		applyLevel(book, arr, isRaw, isFunding)
	}
	c.deliverTo(sub, Event{Kind: KindUpdate, ChanID: chanID, Channel: "book", Symbol: sub.req.Symbol, BookLevel: &lvl, Raw: payload})
}

// decodeBookLevel decodes one row into the sum-type by channel
// precision/symbol class.
func decodeBookLevel(row []any, isRaw, isFunding bool) market.BookLevel {
	lvl := market.BookLevel{}
	switch {
	case isRaw:
		if e, err := rawBookFromWire(row); err == nil {
			lvl.Raw = &e
		}
	case isFunding:
		if e, err := fundingBookFromWire(row); err == nil {
			lvl.Funding = &e
		}
	default:
		if e, err := bookEntryFromWire(row); err == nil {
			lvl.Entry = &e
		}
	}
	return lvl
}

// applyLevel mutates the integrity book from one row.
func applyLevel(b *bookIntegrity, row []any, isRaw, isFunding bool) {
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

// dispatchAuthStream decodes a chanID-0 tagged stream and
// routes it to the account subscription.
func (c *Client) dispatchAuthStream(tag string, payload any) {
	c.mu.Lock()
	sub := c.subs[0]
	c.mu.Unlock()

	ae := &AuthEvent{Stream: tag, Raw: payload}
	kind := KindUpdate

	decodeOne := func(dec func([]any) error, arr []any) bool {
		return dec(arr) == nil
	}
	_ = decodeOne

	arr, isArr := payload.([]any)
	switch tag {
	case "os", "on", "ou", "oc":
		ordersOut, err := decodeEntityList(payload, orders.FromWire)
		if err == nil {
			ae.Orders = ordersOut
			if tag == "os" {
				kind = KindSnapshot
			}
		}
	case "ps", "pn", "pu", "pc":
		list, err := decodeEntityList(payload, positions.FromWire)
		if err == nil {
			ae.Positions = list
			if tag == "ps" {
				kind = KindSnapshot
			}
		}
	case "te", "tu":
		if isArr {
			if te, err := orders.TradeExecutionFromWire(arr); err == nil {
				ae.Trades = []orders.TradeExecution{te}
			}
		}
	case "fos", "fon", "fou", "foc":
		list, err := decodeEntityList(payload, funding.OfferFromWire)
		if err == nil {
			ae.Offers = list
			if tag == "fos" {
				kind = KindSnapshot
			}
		}
	case "fcs", "fcn", "fcu", "fcc":
		list, err := decodeEntityList(payload, funding.CreditFromWire)
		if err == nil {
			ae.Credits = list
			if tag == "fcs" {
				kind = KindSnapshot
			}
		}
	case "fls", "fln", "flu", "flc":
		list, err := decodeEntityList(payload, funding.LoanFromWire)
		if err == nil {
			ae.Loans = list
			if tag == "fls" {
				kind = KindSnapshot
			}
		}
	case "ws", "wu":
		list, err := decodeEntityList(payload, wallets.FromWire)
		if err == nil {
			ae.Wallets = list
			if tag == "ws" {
				kind = KindSnapshot
			}
		}
	case "fte", "ftu":
		if isArr {
			if ft, err := funding.TradeFromWire(arr); err == nil {
				ae.FundingTrades = []funding.FundingTrade{ft}
			}
		}
	case "bu", "miu", "fiu":
		ae.Info = payload
	case "n":
		if isArr {
			if n, err := orders.NotificationFromWire(arr); err == nil {
				ae.Notification = &n
			}
		}
	default:
		// Forward-compatible: unknown tags surface as Info+Raw.
		ae.Info = payload
	}

	ev := Event{Kind: kind, ChanID: 0, Channel: "auth", Auth: ae, Raw: []any{0, tag, payload}}
	if sub != nil {
		c.deliverTo(sub, ev)
	} else {
		c.emitLifecycle(ev)
	}
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
				continue
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
	sub.mu.Lock()
	if sub.closed {
		sub.mu.Unlock()
		return
	}
	sub.wg.Add(1)
	sub.mu.Unlock()
	defer sub.wg.Done()

	if c.cfg.Backpressure == BackpressureBlock {
		sub.events <- ev
		return
	}
	select {
	case sub.events <- ev:
	default:
		// Buffer full → terminal ErrConsumerTooSlow enqueued
		// behind the backlog (the consumer learns data was
		// lost), then the subscription closes once drained.
		// Async so a never-draining consumer cannot deadlock
		// the read loop; released on client Close via done.
		go func() {
			select {
			case sub.events <- Event{Kind: KindError, ChanID: sub.ChanID(), Err: types.ErrConsumerTooSlow}:
				sub.closeEvents()
			case <-c.done:
			}
		}()
	}
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
	return time.Duration(rand.Int63n(int64(d) + 1))
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
	msg, _ := m["msg"].(string)
	code := int64From(m["code"])
	return &types.WebSocketError{Op: "event", Message: msg + " (code " + itoa64(code) + ")"}
}

func safeURL(u string) string { return u }

// thin wrappers delegating to market decoders (the market
// package's decoders are unexported by design — these public
// shims keep the wire contract in one place).
func marketTickerFromWire(a []any, sym types.Symbol) (market.Ticker, error) {
	return market.TickerFromWire(a, sym)
}

func marketTradeFromWire(a []any, sym types.Symbol) (market.Trade, error) {
	return market.TradeFromWire(a, sym)
}

func candleFromWire(a []any) (market.Candle, error) {
	return market.CandleFromWire(a)
}

func bookEntryFromWire(a []any) (market.BookEntry, error) {
	return market.BookEntryFromWire(a)
}

func rawBookFromWire(a []any) (market.RawBookEntry, error) {
	return market.RawBookEntryFromWire(a)
}

func fundingBookFromWire(a []any) (market.FundingBookEntry, error) {
	return market.FundingBookEntryFromWire(a)
}
