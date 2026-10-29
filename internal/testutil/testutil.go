// Package testutil provides deterministic fakes for tests:
// a fixed clock, a scripted HTTPDoer and a scripted WebSocket
// conn/dialer. No real network is ever touched.
package testutil

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"time"
)

// FixedClock returns a preset time from Now and advances by
// Sleep durations.
type FixedClock struct {
	mu  sync.Mutex
	Now time.Time
}

// Now implements the clock contract.
func (c *FixedClock) Time() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Now
}

// Advance moves the clock forward.
func (c *FixedClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Now = c.Now.Add(d)
}

// RecordedRequest captures a sent HTTP request (safe fields
// only in tests — tests may assert headers explicitly).
type RecordedRequest struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

// FakeDoer answers scripted responses in order and records
// every request.
type FakeDoer struct {
	mu        sync.Mutex
	Responses []*http.Response
	Requests  []RecordedRequest
	FailErr   error // when set, Do returns this transport error
}

// Do implements HTTPDoer.
func (f *FakeDoer) Do(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	f.mu.Lock()
	f.Requests = append(f.Requests, RecordedRequest{
		Method: req.Method, URL: req.URL.String(), Header: req.Header.Clone(), Body: body,
	})
	if f.FailErr != nil {
		err := f.FailErr
		f.mu.Unlock()
		return nil, err
	}
	if len(f.Responses) == 0 {
		f.mu.Unlock()
		return JSONResponse(200, `null`, nil), nil
	}
	resp := f.Responses[0]
	f.Responses = f.Responses[1:]
	f.mu.Unlock()
	return resp, nil
}

// JSONResponse builds a fake JSON HTTP response.
func JSONResponse(status int, body string, headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}

// FakeConn is a scripted WebSocket connection: queued inbound
// frames, recorded outbound frames. It satisfies the narrow
// websocket.Conn contract structurally — this package does not
// import websocket so REST-side tests can't form an import
// cycle; websocket tests may define their own equivalent fake
// or wrap this one via an adapter in the websocket package's
// own test files.
type FakeConn struct {
	mu      sync.Mutex
	Inbound [][]byte
	Writes  [][]byte
	Closed  bool
	// ReadErr ends reads when the queue is empty (nil → block
	// until Close, which returns io.EOF).
	ReadErr error
}

// ReadMessage pops the next scripted frame.
func (c *FakeConn) ReadMessage() (int, []byte, error) {
	for {
		c.mu.Lock()
		if len(c.Inbound) > 0 {
			m := c.Inbound[0]
			c.Inbound = c.Inbound[1:]
			c.mu.Unlock()
			return 1, m, nil
		}
		if c.ReadErr != nil {
			err := c.ReadErr
			c.mu.Unlock()
			return 0, nil, err
		}
		if c.Closed {
			c.mu.Unlock()
			return 0, nil, io.EOF
		}
		c.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
}

// Push appends an inbound frame (safe while running).
func (c *FakeConn) Push(raw string) {
	c.mu.Lock()
	c.Inbound = append(c.Inbound, []byte(raw))
	c.mu.Unlock()
}

// WriteMessage records an outbound frame.
func (c *FakeConn) WriteMessage(_ int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Writes = append(c.Writes, append([]byte(nil), data...))
	return nil
}

// WriteControl is a no-op (control frames).
func (c *FakeConn) WriteControl(_ int, _ []byte, _ time.Time) error { return nil }

// Close marks the connection closed, unblocking readers.
func (c *FakeConn) Close() error {
	c.mu.Lock()
	c.Closed = true
	c.mu.Unlock()
	return nil
}

// SetWriteDeadline is accepted for gorilla parity.
func (c *FakeConn) SetWriteDeadline(time.Time) error { return nil }

// SentMessages returns recorded outbound frames.
func (c *FakeConn) SentMessages() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.Writes...)
}
