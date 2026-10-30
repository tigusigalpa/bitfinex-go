# Upgrading to v1.2.2

Use `go get github.com/tigusigalpa/bitfinex-go@v1.2.2`. Existing method signatures
remain unchanged; `orders.UpdateRequest.Leverage` is an additive optional field.

## Five review findings addressed

| Finding | Behavior in v1.2.2 | Regression evidence |
|---|---|---|
| WS multi-op wrapped operations in an unsupported object | `OrderMulti` writes `[0,"ox_multi",null,[[op,args],...]]`, validating 1..75 operations before I/O. | `TestOrderMultiWebSocketWireContract` |
| Fatal writes left the socket ready and could prevent reconnect | Write/deadline failures immediately clear readiness/authentication and close the failed socket. The reader delivers reset and reconnects with fresh auth; failed commands are never replayed. Old writers cannot invalidate a newer generation. | `TestWriteFailureResetsReauthenticatesAndNeverReplaysCommand`, `TestOldWriteFailureCannotInvalidateNewGeneration`, `TestWriteDeadlineFailureInvalidatesBeforeSending` |
| Queued signed requests bypassed a newly applied cooldown | Limiter admission now occurs inside the key gate, before nonce allocation. The next queued request observes the previous response's penalty, including the global IP cooldown and Retry-After. | `TestQueuedSignedRequestObservesNewCooldownBeforeNonce`, `TestLimiterFailureNeverRetriesAndReleasesAuthGate` |
| Null required execution fields were accepted without error | The shared decoder requires symbol, integer timestamp, amount and price; malformed or absent required values produce ProtocolError through REST and WS. Optional fee/currency fields may still be null in te events. | `TestExecutionRejectsMissingAndMalformedRequiredFields`, `TestExecutionKeepsExactValuesAndOptionalFees`, REST/WS propagation regressions |
| Typed derivative updates could not send lev | `UpdateRequest.Leverage` validates 1..100 and is transmitted as an integer in both REST and WS. Existing spot updates omit it. | `TestDerivativeUpdateRESTExactBodyAndNoWriteRetry`, `TestOrderUpdateLeverageWebSocket`, `TestUpdateLeverageValidationAndLegacyOmission` |

## Derivative order updates

Pass the leverage required by the exchange for derivative order updates:

```go
lev := int64(25)
price := types.MustDecimal("65000.25")
request := orders.UpdateRequest{ID: orderID, Price: &price, Leverage: &lev}
result, err := client.Orders.Update(ctx, request)
```

The same request works with `ws.OrderUpdate(ctx, request)`. Omit `Leverage` for
spot orders. The SDK cannot determine an order's market from its numeric ID;
the caller must supply leverage for derivative updates. Range errors are rejected
before I/O. REST callers check the notification's `Success()` along with `err`;
WS callers reconcile notification/order events after a successful socket write.

## Queue and recovery semantics

Rate-limit admission now happens after acquiring the signed-exchange gate.
A request waiting for local endpoint capacity holds that API key's gate. This
prevents calls admitted before an earlier request's 429 from bypassing its new
cooldown, without consuming a second rate token. Public calls and other API keys
use independent gates, while the configured limiter may share an IP cooldown.
Retry backoff remains outside the gate. Use a context deadline to bound queue,
rate-limit and network time together; the HTTP timeout alone bounds the HTTP call.
Custom limiter failures are returned directly and never retried as HTTP failures.

A failed WS write immediately makes the failed connection unavailable. Closing
that socket wakes the read loop, which owns `KindReset`, reconnection and fresh
authentication. Encoding, validation and queue-cancellation errors that occur
before a socket write do not invalidate a healthy connection. The failed command
may have reached the exchange: reconcile IDs/CIDs and account history before
resubmitting. New authentication and subscriptions do not replay the failed command.
The application still owns cache invalidation, account recovery and trading readiness.

## Verification and scope

Regressions use local HTTP servers and fake WS connections; no private exchange
transaction or real funds are used. They cover exact outgoing contracts, errors,
cancellation/cleanup, fresh reconnect generations, no write retry/replay, required
execution fields, exact decimals and optional fee compatibility.

Relevant official contracts: [WS multi-op](https://docs.bitfinex.com/reference/ws-auth-input-order-multi-op),
[REST update](https://docs.bitfinex.com/reference/rest-auth-update-order),
[WS update](https://docs.bitfinex.com/reference/ws-auth-input-order-update),
[executions](https://docs.bitfinex.com/reference/ws-auth-trades), and
[requirements/limits](https://docs.bitfinex.com/docs/requirements-and-limitations).

The earlier [nonce/recovery boundaries](upgrade-v1.2.1.md) and
[pagination constraints](pagination.md) still apply: separate keys or external
ordering across processes, application reconciliation after outages, exchange
retention and saturation limits, and unverified funding-book checksum behavior.
