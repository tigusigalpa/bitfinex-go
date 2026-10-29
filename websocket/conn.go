// Package websocket implements the isolated Bitfinex WebSocket
// v2 subsystem: connection lifecycle, typed subscriptions,
// heartbeats, reconnect with fresh re-auth, and optional
// order-book integrity (SEQ_ALL + OB_CHECKSUM) with automatic
// resync on sequence gaps or checksum mismatches.
//
// Nothing in this package ever logs credentials, signatures or
// nonces.
package websocket

import (
	"context"
	"net/http"
	"time"

	gorilla "github.com/gorilla/websocket"
)

// Conn is the narrow connection contract the client needs —
// *gorilla/websocket.Conn satisfies it, and tests can fake it.
type Conn interface {
	// ReadMessage blocks for the next frame. ctx cancellation is
	// realized by the client closing the conn or by an
	// implementation honoring ctx internally.
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	Close() error
}

// Dialer creates WebSocket connections.
type Dialer interface {
	Dial(ctx context.Context, url string) (Conn, error)
}

// GorillaDialer adapts gorilla/websocket to Dialer.
type GorillaDialer struct {
	// HandshakeTimeout bounds the HTTP upgrade; zero = 10s.
	HandshakeTimeout time.Duration
	// Header allows extra request headers (proxy auth, etc.).
	Header http.Header
}

// Dial implements Dialer.
func (d GorillaDialer) Dial(ctx context.Context, url string) (Conn, error) {
	timeout := d.HandshakeTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	gd := gorilla.Dialer{
		HandshakeTimeout: timeout,
		Proxy:            http.ProxyFromEnvironment,
	}
	c, resp, err := gd.DialContext(ctx, url, d.Header)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	c.SetReadLimit(4 << 20)
	return c, nil
}
