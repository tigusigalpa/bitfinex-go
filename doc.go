// Package bitfinex is an unofficial, production-grade Go client
// library for the Bitfinex API v2 (REST + WebSocket).
//
// Unofficial client library; not affiliated with, endorsed by,
// or sponsored by Bitfinex.
//
// Quickstart:
//
//	c, err := bitfinex.NewClient()
//	res, err := c.Market.Ticker(ctx, "tBTCUSD")
//
// With credentials (private endpoints enabled):
//
//	c, err := bitfinex.NewClient(
//	    bitfinex.WithCredentials(key, secret),
//	)
//
// Design invariants:
//   - Bitfinex API v2 only (REST v1 is intentionally excluded).
//   - All money/prices/amounts/rates are exact Decimal lexemes —
//     never float64.
//   - Every I/O honors context.Context cancellation/deadlines.
//   - Signed bodies are serialized once; signature input is the
//     exact transmitted bytes.
//   - Nonces are strictly increasing per API key; the default
//     provider is process-local — inject a distributed provider
//     for multi-process deployments.
//   - Writes are NEVER automatically retried; reconcile via cid.
//   - No credential, signature, nonce or bfx-* header is ever
//     logged or embedded in an error.
package bitfinex
