package bitfinex

import (
	"net/http"

	"github.com/tigusigalpa/bitfinex-go/account"
	"github.com/tigusigalpa/bitfinex-go/auth"
	"github.com/tigusigalpa/bitfinex-go/funding"
	"github.com/tigusigalpa/bitfinex-go/internal/httpx"
	"github.com/tigusigalpa/bitfinex-go/internal/ratelimit"
	"github.com/tigusigalpa/bitfinex-go/market"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/positions"
	"github.com/tigusigalpa/bitfinex-go/wallets"
	"github.com/tigusigalpa/bitfinex-go/websocket"
)

// Client is the entry point to the Bitfinex API v2 SDK. All
// services are safe for concurrent use. Construct with NewClient;
// every option is validated at construction.
type Client struct {
	// Market is the public market-data service (api-pub).
	Market *market.Service
	// Wallets exposes wallet balances.
	Wallets *wallets.Service
	// Orders exposes order endpoints (active, submit, update,
	// cancel, history, trades).
	Orders *orders.Service
	// Positions exposes position and derivatives endpoints.
	Positions *positions.Service
	// Funding exposes margin-funding endpoints.
	Funding *funding.Service
	// Account exposes user info, ledgers, alerts, settings and
	// money-moving endpoints (transfer/deposit/withdraw).
	Account *account.Service

	opts   options
	signer *auth.Signer
	nonce  NonceProvider
}

// String keeps client options and authentication dependencies out of logs.
func (c *Client) String() string { return "bitfinex.Client{<redacted>}" }

// GoString redacts debug formatting.
func (c *Client) GoString() string { return c.String() }

// NewClient builds a client from functional options. Without
// credentials the client is public-only — private calls return
// ErrCredentialsMissing.
func NewClient(opts ...Option) (*Client, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&o); err != nil {
			return nil, err
		}
	}

	doer := o.httpDoer
	if doer == nil {
		doer = &http.Client{Timeout: o.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else if hc, ok := doer.(*http.Client); ok {
		copyClient := *hc
		original := hc.CheckRedirect
		copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 && via[0].Header.Get("bfx-apikey") != "" {
				return http.ErrUseLastResponse
			}
			if original != nil {
				return original(req, via)
			}
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		}
		doer = &copyClient
	}

	var signer *auth.Signer
	var nonce NonceProvider
	if o.creds != nil {
		signer = auth.NewSigner(*o.creds)
		nonce = o.nonce
		if nonce == nil {
			nonce = auth.NewMillisecondNonceProvider(o.clock)
		}
	}

	limiter := o.limiter
	if limiter == nil && !o.limiterSet {
		limiter = ratelimit.New(o.rateLimits, 0)
	}

	transport := httpx.New(httpx.Config{
		Doer:          doer,
		PublicBase:    o.publicREST,
		PrivateBase:   o.privateREST,
		Signer:        signer,
		NonceProvider: nonce,
		Limiter:       limiter,
		Retry:         o.retry,
		UserAgent:     o.userAgent,
		Logger:        o.logger,
	})

	return &Client{
		Market:    market.New(transport),
		Wallets:   wallets.New(transport),
		Orders:    orders.New(transport),
		Positions: positions.New(transport),
		Funding:   funding.New(transport),
		Account:   account.New(transport),
		opts:      o,
		signer:    signer,
		nonce:     nonce,
	}, nil
}

// NewWebSocket creates an isolated WebSocket client sharing the
// client's endpoints, credentials, nonce provider and logger.
// The returned client has its own lifecycle — Connect*/Close.
func (c *Client) NewWebSocket() *websocket.Client {
	cfg := websocket.Config{
		PublicURL:     c.opts.publicWS,
		PrivateURL:    c.opts.privateWS,
		Signer:        c.signer,
		NonceProvider: c.nonce,
		Dialer:        c.opts.wsDialer,
		Logger:        c.opts.logger,
	}
	ws, _ := websocket.New(cfg)
	return ws
}
