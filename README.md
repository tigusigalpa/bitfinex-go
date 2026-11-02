# Bitfinex Go Client/SDK/Library

![Bitfinex Go client/sdk/library](https://i.postimg.cc/0N5hnNYp/bitfinex-go-hero-banner.jpg)

A Go client for the **Bitfinex API v2**, with REST services and WebSocket streams.

Bitfinex returns much of its data as positional arrays. This library turns those
arrays into Go types and handles request signing, local rate limits, read retries,
and socket reconnection. You can use it to collect market data, inspect an account,
manage orders, or build the exchange connection for a trading application.

Your application still decides when to trade, how much risk to take, and how to
recover its account state after an interruption. The sections below explain where
that responsibility meets the SDK.

> **Unofficial client library; not affiliated with, endorsed by, or sponsored by Bitfinex.**

| Project | Details |
|---|---|
| Module | `github.com/tigusigalpa/bitfinex-go` |
| Release | [v1.2.2](https://github.com/tigusigalpa/bitfinex-go/releases/tag/v1.2.2) |
| Go | Module minimum: 1.22. Release CI: Go 1.26 and 1.27. |
| Protocols | REST v2 and WebSocket v2 |
| License | [MIT](LICENSE), Igor Sazonov ([tigusigalpa](https://github.com/tigusigalpa)) |

## Find your starting point

- [Install and make a public request](#install-and-make-a-public-request)
- [Choose a service](#choose-a-service)
- [Connect an account](#connect-an-account)
- [Read responses and work with money](#read-responses-and-work-with-money)
- [Walk through history](#walk-through-history)
- [Submit and update orders](#submit-and-update-orders)
- [Follow a WebSocket book](#follow-a-websocket-book)
- [Configure timeouts, retries, and rate limits](#configure-timeouts-retries-and-rate-limits)
- [Share clients and API keys](#share-clients-and-api-keys)
- [Handle errors](#handle-errors)
- [Plan a production rollout](#plan-a-production-rollout)
- [Known gaps in v1.2.2](#known-gaps-in-v122)
- [Run checks and contribute](#run-checks-and-contribute)

## Install and make a public request

Pin a release so an upgrade is a deliberate change:

```sh
go get github.com/tigusigalpa/bitfinex-go@v1.2.2
```

Public market data needs no account or API key. Save this as `main.go` in your Go
application and run it with `go run .`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tigusigalpa/bitfinex-go"
)

func main() {
	client, err := bitfinex.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ticker, err := client.Market.Ticker(ctx, "tBTCUSD")
	if err != nil {
		log.Fatal(err)
	}
	if !ticker.Data.LastPrice.IsValid() {
		log.Fatal("ticker has no last price")
	}
	fmt.Printf("BTC/USD last price: %s\n", ticker.Data.LastPrice)
}
```

Symbols use the exchange's wire names: `tBTCUSD` for a trading pair, `fUSD` for
funding, and a name such as `tBTCF0:USTF0` for a derivative market. You can check a
symbol's syntax with `bitfinex.ParseSymbol`; that does not check whether the market
currently exists on the exchange.

For ready-to-run programs, see [examples](examples):

```sh
go run ./examples/public_quickstart
go run ./examples/websocket_ticker
```

These commands are run from this repository. The public examples require no keys.

## Choose a service

Create a `Client` once and reuse it. Its REST services support concurrent callers;
signed requests on the same API key are serialized as described below.

| Service | What you can do |
|---|---|
| `client.Market` | Read tickers, trades, books, candles, configurations, derivatives data, liquidations, rankings, funding statistics, and public calculations. |
| `client.Wallets` | Read wallet balances. |
| `client.Orders` | Submit, update, and cancel orders; retrieve active orders, executions, and history; send combined operations. |
| `client.Positions` | Read positions and margin information, manage derivative collateral, and inspect position history or previews. |
| `client.Funding` | Work with funding offers, credits, loans, trades, and funding information. |
| `client.Account` | Read account information and ledgers; manage settings, alerts, deposits, transfers, withdrawals, and related payment operations. |
| `client.NewWebSocket()` | Create an independent socket client that shares the parent client's endpoints, credentials, nonce provider, and logger. |

The [endpoint inventory](docs/endpoint-coverage.md) lists individual methods,
request and response types, test coverage, and official reference links. REST v1
is outside this library's scope.

Some older methods return raw data or compatibility adapters. Prefer the typed
replacements listed in the [v1.2.0 migration guide](docs/upgrade-v1.2.0.md) when
starting new code. A `Raw` field or a `map[string]any` is an escape hatch: its
contents do not receive the same validation as a typed request.

## Connect an account

Pass credentials when constructing the client. Keep them in your environment or
secret manager; use a read-only key for account inspection and grant additional
permissions only when the application needs them.

This helper reads balances using `BFX_API_KEY` and `BFX_API_SECRET`:

```go
package example

import (
	"context"
	"fmt"
	"os"

	"github.com/tigusigalpa/bitfinex-go"
)

func readWallets(ctx context.Context) error {
	client, err := bitfinex.NewClient(bitfinex.WithCredentials(
		os.Getenv("BFX_API_KEY"),
		os.Getenv("BFX_API_SECRET"),
	))
	if err != nil {
		return err
	}

	wallets, err := client.Wallets.All(ctx)
	if err != nil {
		return err
	}
	for _, wallet := range wallets.Data {
		if !wallet.Balance.IsValid() {
			return fmt.Errorf("missing balance for %s %s", wallet.Type, wallet.Currency)
		}
		fmt.Printf("%s %s balance=%s\n", wallet.Type, wallet.Currency, wallet.Balance)
	}
	return nil
}
```

The remaining Go examples are helper functions you can call from your application.
Pass them a context with a deadline and a client configured for the required scopes.
A client created without credentials can use public services; private calls return
`bitfinex.ErrCredentialsMissing`.

The built-in logger and SDK error messages redact authentication material. Your
own logs still need care: account responses, raw payloads, payment details, and
application configuration may contain sensitive information.

## Read responses and work with money

REST calls return `*types.Response[T]`. The useful parts are:

| Field | Meaning |
|---|---|
| `Data` | The decoded result, such as a ticker, a list of orders, or a notification. |
| `Meta.StatusCode` | HTTP response status. |
| `Meta.Attempts` | Number of HTTP attempts made for this call. |
| `Meta.Headers` | An allowlist of bounded response metadata, including rate-limit headers when supplied. |

For operations that return a notification, check its status or `Success()` as well
as the Go error. A decoded HTTP response can contain an exchange rejection.

Modeled prices, amounts, rates, balances, fees, and P/L use `Decimal`. Parse values
from strings rather than converting through `float64`:

```go
package example

import "github.com/tigusigalpa/bitfinex-go"

func notional(priceText, amountText string) (bitfinex.Decimal, error) {
	price, err := bitfinex.NewDecimal(priceText)
	if err != nil {
		return bitfinex.Decimal{}, err
	}
	amount, err := bitfinex.NewDecimal(amountText)
	if err != nil {
		return bitfinex.Decimal{}, err
	}
	return price.Mul(amount), nil
}
```

For example, `notional("65000.50", "0.01")` produces exactly `650.005`.
`String()` preserves a parsed value's original spelling; `CanonicalString()` and
outgoing JSON normalize it. Missing decimal fields marshal as `null`.

**Check `IsValid()` before using optional values in calculations.** `Decimal{}`
means absent data, and its arithmetic methods treat it as zero. Missing data and
a reported zero have different meanings in an account or market feed.

Use `MustDecimal` for constants you control; use `NewDecimal` for values arriving
from configuration or user input. Decimal input and expansion sizes are bounded.
Your application remains responsible for exchange tick sizes, amount steps, and
rounding decisions.

## Walk through history

A one-page history call is useful for a recent window. Use an iterator when you
need to continue through that window without writing the pagination loop yourself:

```go
package example

import (
	"context"
	"fmt"
	"time"

	"github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func printRecentTrades(ctx context.Context, client *bitfinex.Client) error {
	start := types.MillisFromTime(time.Now().Add(-time.Hour))
	trades, err := client.Market.TradesIterator("tBTCUSD", &types.HistoryQuery{
		Start: &start,
		Limit: 100,
		Sort:  types.SortAsc,
	})
	if err != nil {
		return err
	}
	defer trades.Close()

	for trades.Next(ctx) {
		trade := trades.Value()
		fmt.Printf("%d %s amount=%s price=%s\n",
			trade.ID, trade.MTS.Time(), trade.Amount, trade.Price)
	}
	return trades.Err()
}
```

Iterators fetch pages lazily and remember overlap identities at page boundaries.
Always check `Err()` after `Next` returns false: cancellation, a protocol failure,
and an exhausted history are different outcomes. Use each iterator from one
goroutine at a time.

Limits and supported sort directions vary by endpoint. A timestamp window that
remains full at the server's maximum produces `types.PaginationError`, preserving
the fact that completeness could not be established. Numbered and offset-based
lists can also change while you read them. Exchange retention still applies.
See the [pagination guide](docs/pagination.md) for the exact behavior and limits.

## Submit and update orders

Order writes need a key with the appropriate trading permissions. **Calling this
helper submits a real order.** The amount and price below illustrate the request
format; choose values and risk limits in your own application.

Supply a `ClientOrderID` that is unique for your account within its UTC day. Persist
the ID and the order intention before sending. `orders.NewClientOrderID` validates
a supplied ID/date pair; it does not allocate unique IDs for you.

```go
package example

import (
	"context"
	"fmt"

	"github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/orders"
)

func submitLimit(ctx context.Context, client *bitfinex.Client, cid orders.ClientOrderID) error {
	price := bitfinex.MustDecimal("65000.50")
	result, err := client.Orders.Submit(ctx, orders.SubmitRequest{
		Type:   orders.TypeExchangeLimit,
		Symbol: "tBTCUSD",
		Amount: bitfinex.MustDecimal("0.01"),
		Price:  &price,
		CID:    &cid,
	})
	if err != nil {
		return fmt.Errorf("submit cid=%d; reconcile before resubmitting: %w", cid.ID, err)
	}
	if !result.Data.Success() {
		return fmt.Errorf("order was not accepted: status=%s", result.Data.Notification.Status)
	}
	if order, ok := result.Data.FirstOrder(); ok {
		fmt.Printf("accepted order id=%d cid=%d\n", order.ID, order.CID)
	}
	return nil
}
```

The SDK never automatically retries order submissions, updates, cancellations,
or other writes. A timeout can happen after the exchange has acted. Check active
orders, history, and notifications using the original ID/CID before deciding what
to send next. Keep a consistent account state when combining REST and WS updates.

### Derivative updates and collateral

Derivative order updates require leverage. v1.2.2 exposes it in the same request
used by REST and WebSocket:

```go
package example

import (
	"context"
	"fmt"

	"github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/orders"
	"github.com/tigusigalpa/bitfinex-go/types"
)

func updateDerivative(ctx context.Context, client *bitfinex.Client, orderID, leverage int64, price types.Decimal) error {
	result, err := client.Orders.Update(ctx, orders.UpdateRequest{
		ID:       orderID,
		Price:    &price,
		Leverage: &leverage,
	})
	if err != nil {
		return err
	}
	if !result.Data.Success() {
		return fmt.Errorf("update was not accepted: status=%s", result.Data.Notification.Status)
	}
	return nil
}
```

`Leverage` accepts 1..100 and is omitted for spot updates. The SDK cannot infer a
market from an order ID, so the caller supplies leverage for derivative orders.
`ws.OrderUpdate(ctx, request)` uses the same `orders.UpdateRequest`.

For collateral changes, prefer `client.Positions.SetDerivativeCollateralAmount`.
Its third argument is an exact decimal **amount**. Check the returned
`Data.Success` flag. The [upgrade guide](docs/upgrade-v1.2.2.md) explains these
contracts and the compatibility methods.

## Follow a WebSocket book

Each socket has its own lifecycle. Create it with `NewWebSocket`, connect it,
subscribe, and always call `Close`. Canceling the initial `Connect*` context stops
that handshake; after a successful connection, `Close` owns socket shutdown.

Public subscriptions cover tickers, trades, books, candles, and status streams.
Data events expose channel-specific payload fields, alongside `Raw` for inspecting
fields the SDK does not model yet.

This helper prints an aggregated book stream and handles resets explicitly:

```go
package example

import (
	"context"
	"fmt"

	"github.com/tigusigalpa/bitfinex-go"
	"github.com/tigusigalpa/bitfinex-go/websocket"
)

func watchBook(ctx context.Context, client *bitfinex.Client) error {
	ws := client.NewWebSocket()
	defer ws.Close()

	if err := ws.ConnectPublic(ctx, websocket.FlagSeqAll|websocket.FlagChecksum); err != nil {
		return err
	}
	sub, err := ws.Subscribe(ctx, websocket.SubscribeBook("tBTCUSD", "P0", "F0", 25))
	if err != nil {
		return err
	}

	haveSnapshot := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-sub.Events():
			if !ok {
				return sub.Err()
			}
			switch event.Kind {
			case websocket.KindReset:
				haveSnapshot = false
				fmt.Printf("connection lost; discard book generation %d\n", event.Generation)
			case websocket.KindSnapshot:
				haveSnapshot = true
				fmt.Printf("fresh snapshot: generation=%d levels=%d\n",
					event.Generation, len(event.BookSnapshot))
			case websocket.KindUpdate:
				if haveSnapshot && event.BookLevel != nil && event.BookLevel.Entry != nil {
					level := event.BookLevel.Entry
					fmt.Printf("price=%s count=%d amount=%s\n", level.Price, level.Count, level.Amount)
				}
			case websocket.KindError:
				return event.Err
			}
		}
	}
}
```

This example reports events; an application maintaining a local book must apply
snapshots and updates to its own state. P0..P4 books use price levels; R0 books use
order IDs and different deletion rules. For an aggregated book, `Count == 0`
removes a level, and the amount's sign identifies the side.

`FlagSeqAll` checks connection sequence numbers. `FlagChecksum` enables CRC32
checks over the top 25 bids and asks for trading books. Integrity failures trigger
a reset and fresh subscriptions. Funding-book checksum behavior remains unverified.

### Reconnects, account streams, and slow consumers

A disconnect or failed write closes the affected socket and triggers reconnect
with backoff and fresh authentication. Attempts are paced at least three seconds
apart per client. The failed command is never replayed.

`KindReset` is sent to active subscriptions and `Lifecycle()`, replacing stale
buffered events. Clear derived state on reset. A newly connected socket can still
be waiting for subscription acknowledgements and fresh data: monitor `Lifecycle()`
and put a deadline on recovery. If you hand events to workers, carry
`Event.Generation` through your own queues so old work cannot restore a stale cache.

For account streams, call `ConnectAuth(ctx, websocket.AuthOptions{})`, then
`SubscribeAccount()`. Snapshots begin buffering before authentication returns.
Drain the stream promptly. Filters and the dead-man switch are explicit options;
`DeadManSwitch: true` asks the exchange to cancel orders when the socket dies.

Account data arrives through `Event.Auth`: its `AuthEvent` contains orders,
positions, trades, funding records, wallets, or notifications according to the
stream. Typed socket helpers also support order and funding commands. Inspect
the stream and populated payload rather than assuming every event is an order.

Account snapshots cover current state. Recover fills, closed orders, fees, and
ledger activity missed during an outage through REST history before resuming
trading decisions. A successful WS write still needs exchange confirmation through
notification and order events.

The default `BackpressureStop` policy ends a lagging subscription with
`ErrConsumerTooSlow`. Recover its state and subscribe again. `BackpressureBlock`
waits for the consumer and can stall the entire connection, including other channels.
Choose it only when your consumers can keep up.

## Configure timeouts, retries, and rate limits

Use a context deadline for each complete operation. The built-in HTTP timeout is
30 seconds; an operation can also spend time waiting for rate admission or its
API-key queue. `WithTimeout` configures the built-in HTTP client and does not
change an injected client's timeout.

| Option | When it is useful |
|---|---|
| `WithHTTPClient` / `WithHTTPDoer` | Use your transport, proxy configuration, or instrumentation. |
| `WithTimeout` | Change the built-in HTTP client's timeout. |
| `WithRetryPolicy` | Tune bounded retries for classified idempotent reads. |
| `WithRateLimitProfile` | Override budgets for named local rate buckets. |
| `WithRateLimiter` | Coordinate traffic through shared or distributed throttling. Passing `nil` disables local throttling. |
| `WithNonceProvider` | Share or persist nonce allocation for an API key. |
| `WithLogger` | Supply an `slog.Logger` for SDK diagnostics. |
| `WithWSDialer` | Supply a socket dialer for tests or transport customization. |

The default limiter uses endpoint-aware token buckets. Unknown/shared groups
receive a conservative 10 requests/minute budget; liquidations have a separate
3 requests/minute bucket. Multiple clients sharing an IP need to coordinate their
combined traffic even when their API keys differ.

After HTTP 429 or a recognized rate-limit error, the built-in limiter applies an
IP-wide cooldown of at least 60 seconds within that client, extended by a longer
`Retry-After`. Signed REST calls acquire their key's gate before rate admission,
so queued requests see a newly applied cooldown before allocating another nonce.
An endpoint rate wait therefore holds that key's gate and can delay later calls.

Classified transient failures of idempotent reads may be retried: selected network
errors and HTTP 408, 429, 500, 502, 503, or 504 responses. Defaults are three attempts
and a 30-second elapsed budget, including queue waits, rate waits, and I/O. A required cooldown
can exceed that budget and be returned to the caller. Authentication, validation,
nonce, and protocol errors are not automatically retried. Writes are never
retryable under this policy.

Signed redirects are refused for the built-in HTTP client and injected
`*http.Client` values. A custom `HTTPDoer` must enforce equivalent redirect,
cancellation, and credential-handling behavior. Built-in response and frame
limits reject data above 4 MiB. Custom transports and nonce providers must honor
contexts and avoid leaking sensitive data through their own diagnostics.

## Share clients and API keys

The exchange requires a strictly increasing nonce per API key. The default
`auth.MillisecondNonceProvider` uses `max(nowMillis, last+1)` and is safe for
concurrent callers sharing that provider instance.

A root client and the sockets created with its `NewWebSocket` share a provider.
If you create several root clients on the same key, create one provider and pass
that same instance to each with `WithNonceProvider`. REST exchanges and WS
authentication on the key are serialized through the response or auth acknowledgement.

The built-in gate and provider are process-local. Independent services are easier
to operate with separate API keys. If replicas must share a key, coordinate both
persistent nonce allocation and the order of complete signed exchanges externally.
A distributed counter alone cannot prevent requests from arriving out of order,
and restarting a process does not restore its previous nonce state.

## Handle errors

Use `errors.Is` for sentinel errors and `errors.As` for typed details:

```go
package example

import (
	"context"
	"errors"
	"log"

	"github.com/tigusigalpa/bitfinex-go"
)

func reportError(err error) {
	if err == nil {
		return
	}
	var limited *bitfinex.RateLimitError
	var protocol *bitfinex.ProtocolError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		log.Print("deadline exceeded; reconcile any write whose outcome is uncertain")
	case errors.Is(err, bitfinex.ErrCredentialsMissing):
		log.Print("this operation requires API credentials")
	case errors.As(err, &limited):
		log.Printf("rate limit: cooldown=%s attempts=%d", limited.RetryAfter, limited.Attempts)
	case errors.As(err, &protocol):
		log.Printf("unexpected exchange response: %v", protocol)
	default:
		log.Printf("SDK call failed: %v", err)
	}
}
```

Other error types include `ValidationError`, `AuthenticationError`, `TransportError`,
`NonceError`, `WebSocketError`, `SequenceGapError`, and `ChecksumError`.
`ErrConsumerTooSlow` and `ErrClosed` describe subscription and lifecycle failures.
An error handler should decide whether to stop, wait, reconnect, or reconcile;
putting every failed operation into a retry loop is unsafe for writes.

## Plan a production rollout

Start with public data, then an account reader using a narrowly scoped key. Verify
the private operations your application will use before enabling automated trading.
The SDK's tests cover documented contracts with local HTTP servers and fake sockets;
live account behavior and your recovery workflow need their own integration checks.

Before enabling writes, make sure the application has:

- Persistent order intentions and unique CIDs, with a process for uncertain outcomes.
- A recovery gate that waits for fresh data and REST reconciliation after reset.
- Monitoring for disconnected feeds, overdue snapshots, rate limits, and slow consumers.
- Shared traffic/nonce coordination appropriate to its deployment layout.
- Validity checks for required business data and application-level risk limits.

High-impact transfer, withdrawal, invoice, and Thalex requests require
`Confirm: true`; some also require additional destination or authentication fields.
Those flags express an explicit write intention. They do not establish that the
transaction is appropriate for your application.

### Known gaps in v1.2.2

The final review identified two remaining parsing gaps:

1. The [order decoder](orders/models.go) can accept `null` for some required fields,
   including symbol, creation time, amount, or status, without a `ProtocolError`.
   Validate the fields your application relies on before acting on an order row.
2. The [checksum handler](websocket/loop.go) can coerce a malformed checksum to zero,
   which matches an empty book's CRC. A checksum event alone is therefore not a
   complete feed-health guarantee. Close this parsing gap before relying on it
   as a trading safety gate.

The five earlier review findings were addressed in v1.2.2: WS multi-op framing,
socket write recovery, queued cooldown admission, required execution fields, and
leverage in derivative updates. See the [upgrade notes](docs/upgrade-v1.2.2.md)
for regression evidence and remaining operating boundaries.

## Run checks and contribute

From the repository root:

```sh
go build ./...
go vet ./...
go test ./...
go test -race ./...
gofmt -l .
```

The v1.2.2 release passed CI on Go 1.26.10 and 1.27.3, including unit/race tests,
builds, formatting and dependency checks, six active fuzz targets, Staticcheck,
Govulncheck, Gitleaks, and CodeQL. Total statement coverage was **76.4%**.
[Release CI results](https://github.com/tigusigalpa/bitfinex-go/actions/runs/41016370075).
The fuzz jobs are short parser smoke checks; sustained load and recovery testing
belong in the environment where you plan to run the SDK.

There is one external runtime dependency, `gorilla/websocket`. The code is organized
by service under `market/`, `orders/`, `positions/`, `funding/`, `wallets/`, and
`account/`; shared models live in `types/`, authentication in `auth/`, and streaming
in `websocket/`.

Contributions are welcome, especially real-world contract corrections with
sanitized fixtures and reproducible failure cases. Read [CONTRIBUTING.md](CONTRIBUTING.md)
for the workflow. Report suspected vulnerabilities privately as described in
[SECURITY.md](SECURITY.md); keep credentials and account details out of issues.

## Documentation and upgrades

- [API reference on pkg.go.dev](https://pkg.go.dev/github.com/tigusigalpa/bitfinex-go)
- [Endpoint coverage](docs/endpoint-coverage.md)
- [Pagination behavior and retention](docs/pagination.md)
- [Upgrade to v1.2.2](docs/upgrade-v1.2.2.md)
- [Upgrade to v1.2.1](docs/upgrade-v1.2.1.md)
- [Typed API and pagination migration from v1.2.0](docs/upgrade-v1.2.0.md)
- [Changelog](CHANGELOG.md)
- [Security policy](SECURITY.md)

The module follows v1 versioning without a `/v1` suffix. Breaking API changes
require a new major module path and an upgrade guide. Published tags are never
rewritten. Review the changelog when upgrading and use a supported, patched Go
toolchain for deployment.

## License and author

[MIT](LICENSE). Maintained by Igor Sazonov ([tigusigalpa](https://github.com/tigusigalpa)).
