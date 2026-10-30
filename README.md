# bitfinex-go

Unofficial Go client library for the **Bitfinex API v2** (REST + WebSocket).

> **Unofficial client library; not affiliated with, endorsed by, or sponsored by Bitfinex.**

- Module: `github.com/tigusigalpa/bitfinex-go`
- Go: **1.22+**
- License: MIT — Igor Sazonov ([tigusigalpa](https://github.com/tigusigalpa))
- API coverage and status per endpoint: [docs/endpoint-coverage.md](docs/endpoint-coverage.md)
- Only **API v2** is supported. REST v1 is intentionally excluded.

The v1.2.0 release adds typed contracts for the previously deferred endpoints,
correct derivative collateral management, and lazy history iterators.
See the [review](docs/review-v1.2.0.md), [upgrade notes](docs/upgrade-v1.2.0.md)
and [pagination guide](docs/pagination.md). Legacy signatures remain available.

## Install

```bash
go get github.com/tigusigalpa/bitfinex-go@v1.2.0
```

## Quickstart — public, read-only

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/tigusigalpa/bitfinex-go"
)

func main() {
	client, err := bitfinex.NewClient()
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ticker, err := client.Market.Ticker(ctx, "tBTCUSD")
	if err != nil {
		panic(err)
	}
	fmt.Println("last:", ticker.Data.LastPrice)
}
```

## Credentials (private endpoints)

```go
client, err := bitfinex.NewClient(
	bitfinex.WithCredentials(os.Getenv("BFX_API_KEY"), os.Getenv("BFX_API_SECRET")),
)
wallets, err := client.Wallets.All(ctx)
```

**Minimum-permissions warning:** use the least-privileged API key for the job. For read-only
use, create a key with read scopes only and **no** withdrawals permission. The client never
logs your key, secret, signature or nonce — treat them accordingly on your side too.
A client without credentials works for all public endpoints; private calls return
`bitfinex.ErrCredentialsMissing`.

## Exact decimals — never float64

All money, prices, amounts, rates, fees, balances and P/L use `bitfinex.Decimal`, an immutable
value that preserves the exact wire lexeme. JSON decoding uses `UseNumber` throughout; nothing
money-related ever passes through `float64`. Marshal produces canonical decimal strings.
`String()` preserves the input lexeme; `CanonicalString()` and outgoing JSON remove
redundant signs, zeros and exponent notation. Inputs are bounded to 1024 characters
and exponents in [-1024, 1024], with an expanded representation of at most 1024
characters, to prevent excessive allocation from untrusted data.
Absent decimal fields marshal as `null`.

```go
price, _ := bitfinex.NewDecimal("65000.50")
amount, _ := bitfinex.NewDecimal("0.01")
total := price.Mul(amount) // exact, no float rounding
```

## Nonce / multi-process warning

Bitfinex requires a **strictly increasing nonce per API key**. The default
`MillisecondNonceProvider` is process-local and concurrency-safe
(`max(nowMillis, last+1)`), but it does **not** coordinate across processes, machines or
restarts. If you run multiple services or replicas on one key, inject a
persistent/distributed `NonceProvider` via `WithNonceProvider` — or, as the official docs
recommend, issue **separate API keys per independent client**.

## Cancellation and timeouts

Every method takes `context.Context` first and honors cancellation/deadlines — HTTP calls,
rate-limiter waits, retries, WS connect/subscribe and sends. Default HTTP timeout is 30s
(`WithTimeout`, or configure your own `http.Client`/`HTTPDoer`).

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
resp, err := client.Orders.History(ctx, "", query)
```

## Rate limits and retries

- Endpoint-aware token buckets (unknown/shared endpoint groups default to 10 req/min;
  override via `WithRateLimitProfile` or inject `WithRateLimiter`).
- After `ERR_RATE_LIMIT`/HTTP 429 all buckets in this client cool down **≥ 60 seconds**
  (`Retry-After` honored when longer).
- Retries: only **classified idempotent reads** (DNS/connect/read timeouts, HTTP
  408/429/5xx), capped exponential backoff with full jitter, bounded attempts and elapsed
  time.
- **Writes are never automatically retried** — order submit/update/cancel, funding writes,
  transfers, withdrawals, invoices, alerts, settings. An ambiguous outcome may already have
  mutated state. Reconcile with `orders.ClientOrderID` (`cid`/`cid_date`) instead of
  resubmitting.

Limits are local to each client; applications sharing an IP/key must coordinate
their aggregate traffic. Retry elapsed budgets include limiter waits and I/O.
Signed redirects are refused by the built-in and injected `*http.Client`; custom
`HTTPDoer` implementations must enforce equivalent redirect and context behavior.

## WebSocket

```go
ws := client.NewWebSocket()
defer ws.Close() // always — releases socket and goroutines

if err := ws.ConnectPublic(ctx, websocket.FlagSeqAll|websocket.FlagChecksum); err != nil {
	log.Fatal(err)
}
sub, err := ws.Subscribe(ctx, websocket.SubscribeBook("tBTCUSD", "P0", "F0", 25))
if err != nil {
	log.Fatal(err)
}
for ev := range sub.Events() { /* snapshots → deltas, heartbeats, checksums */ }
if err := sub.Err(); err != nil { /* handle terminal subscription failure */ }
```

- Public channels: `ticker`, `trades`, `book` (P0–P4 aggregated **and** R0 raw — different
  update semantics, never mixed), `candles`, `status`.
- Authenticated: `ConnectAuth(ctx, websocket.AuthOptions{})` — fresh nonce and
  `AUTH+nonce` HMAC-SHA384 per (re)connect; `filter` and dead-man-switch (`dms: 4`) are
  strictly opt-in. All chanId-0 streams (`os/on/…`, `ps/…`, `te/tu`, `fo*`, `fc*`, `fl*`,
  `ws/wu`, `bu`, `miu`, `fiu`, `fte/ftu`, `n`) arrive as typed `AuthEvent`s; typed inputs
  `on/ou/oc/oc_multi/ox_multi/fon/foc/calc` are provided.
- Integrity mode (for trading books): `FlagSeqAll` (65536) + `FlagChecksum` (131072)
  give sequence-gap detection and CRC32 top-25 book validation; gaps/mismatches trigger an
  a connection reset and fresh snapshots for all tracked channels. Discard local
  derived state after integrity errors and rebuild it from the new snapshots.
  Funding-book checksum semantics remain unverified.
- Reconnects use capped full-jitter backoff with a three-second minimum between
  connection attempts per client, fresh-nonce authentication, and subscription ack
  tracking. Multiple clients require application-level connection pacing.
- `BackpressurePolicy`: the default `BackpressureStop` emits terminal
  `ErrConsumerTooSlow`, invalidates buffered data and closes a lagging subscription;
  a queued event may be replaced by the terminal error. `Subscription.Err()` also
  exposes the failure. `BackpressureBlock` applies real
  backpressure (blocks the shared read loop — use with care). Nothing is silently dropped.

Canceling `Connect*` during the initial handshake closes the client. After successful
connection, its lifetime is owned by `Close()`. Account snapshots are buffered before
`ConnectAuth` returns; call `SubscribeAccount()` promptly and drain the stream.

## Errors

Typed, `errors.Is`/`errors.As`-friendly: `APIError`, `RateLimitError`,
`AuthenticationError`, `TransportError`, `ProtocolError`, `NonceError`, `WebSocketError`,
`SequenceGapError`, `ChecksumError`, `ValidationError`; sentinels `ErrCredentialsMissing`,
`ErrConsumerTooSlow`, `ErrClosed`.

## Layout

```
client.go, options.go, aliases.go, doc.go   root package `bitfinex`
types/     Decimal, Symbol, Millis, Response[T], HistoryQuery, errors
auth/      credentials, HMAC-SHA384 signer, nonce providers
market/    public REST (tickers/trades/book/candles/stats/conf/deriv/liq/rankings/funding-stats/vasps/calc)
orders/    order models + submit/update/cancel/multi/history/trades
wallets/   wallet balances
positions/ positions + derivatives + margin info
funding/   margin funding offers/loans/credits/trades
account/   user info, ledgers, transfers, deposits, withdraw, alerts, settings
websocket/ lifecycle, subscriptions, typed events, book integrity
internal/  wire decoders, httpx transport, retry, ratelimit, testutil
examples/  runnable, credential-free examples
docs/      endpoint coverage, audit
```

## Support matrix / SemVer

This is **v1** (`github.com/tigusigalpa/bitfinex-go`, no `/vN` suffix). Breaking changes
will require a `/v2` module path, tag `v2.x.y`, updated examples and an upgrade guide —
per SemVer and the project's [CHANGELOG.md](CHANGELOG.md). Tags are never rewritten.
The module declares Go 1.22; current CI tests the latest patches of Go 1.26 and 1.27.
Use a supported, patched Go toolchain in production.

## Security

See [SECURITY.md](SECURITY.md). Never commit credentials; report vulnerabilities privately.
