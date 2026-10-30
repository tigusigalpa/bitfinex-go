# Upgrading to v1.2.1

Use `go get github.com/tigusigalpa/bitfinex-go@v1.2.1`. Exported method signatures
and existing event kind numeric values remain unchanged. `KindReset` and
`Event.Generation` are additive; applications must handle the reset signal.

## Four review findings addressed

| Finding | Behavior in v1.2.1 | Regression evidence |
|---|---|---|
| Concurrent signed requests can arrive in reversed nonce order | A process-wide gate per API key covers nonce generation, REST response-body consumption and WS authentication acknowledgement. Different keys and public calls remain independent. | `internal/httpx/nonce_order_test.go`, `TestWebSocketAuthOrdersRESTUntilAcknowledged`, `TestRESTOrdersWebSocketAuth` |
| A disconnect leaves cached/queued state apparently valid | `KindReset` is delivered before backoff to lifecycle and active/pending subscription streams, including account channel zero. Old buffers and integrity books are cleared. Data events expose the new connection generation. Authenticated inputs queued on an old generation or state fail locally. | `TestDisconnectResetReplacesFullQueuesBeforeBackoff`, `TestReconnectDeliversNewGenerationSnapshot`, `TestQueuedAuthInputCannotCrossConnectionState` |
| Valid initial liquidations with a null acquired price fail decoding | `PriceAcquired.IsValid()` is false for null and true for an actual zero. Invalid non-null values still fail. REST, iterator and WS share the decoder. | `TestLiquidationNullableAcquiredPriceRESTAndIterator`, `TestLiquidationNullableAcquiredPriceWebSocket` |
| Malformed required entity IDs silently become zero | Primary IDs fail with `ProtocolError` on missing, malformed, fractional, out-of-range, zero or negative values. Exact int64 numeric strings remain supported. Nullable related IDs keep their legacy absence semantics but propagate type errors. | `TestRequiredEntityIDs`, `TestAccountRequiredIDs`, `TestOptionalRelatedIDsRetainTypeErrors` |

## Reset handling

Handle `KindReset` alongside `KindError`. On reset, mark all state derived from
that connection invalid; discard cached books/account state and stop decisions
based on those caches. A new snapshot replaces prior state. Keep generation
numbers when offloading events to application workers, so already consumed
events from an old generation cannot repopulate the new cache. The SDK can
discard its own queues, but cannot retract events already handed to an application.

Order-book deltas and checksum confirmations are rejected until a valid snapshot
arrives. An empty snapshot is valid. Tickers provide fresh state via their next
full update; subscriptions without a complete snapshot need application-specific
recovery. Lifecycle advisory events may coalesce; its latest reset replaces older
buffered messages. A subscription terminated by backpressure still requires a
new subscription and full state recovery.

On account reconnect, fresh active-order/position/wallet snapshots do not replay
every historical fill, closed order or ledger movement during the outage.
Reconcile those through REST histories before resuming dependent trading. The SDK
does not decide when application recovery is complete and does not replay writes.

## Nonce and operational boundaries

- One SDK client shares its nonce provider between REST and WS clients. If
  multiple clients in one process share a key, pass the same provider instance
  to all of them. The gate orders exchanges; it does not repair an invalid,
  decreasing or independently initialized provider.
- The gate is process-local. Replicas need separate API keys or external
  coordination covering both persistent nonce allocation and the full signed
  exchange. A shared monotonic counter alone does not order network arrivals.
- Serialization intentionally reduces private-request concurrency per key. Rate
  waits and retry backoff occur outside the gate. Custom transports/providers
  must honor context cancellation; an unresponsive implementation can stall the key.
- A transport failure can leave a server-side write outcome uncertain, even with
  serialization. Writes are never automatically retried. Reconcile IDs, CIDs,
  notifications and account history before submitting again.
- Shared-IP rate limits, connection pacing and exchange-specific funding-book
  checksum behavior retain the constraints documented in earlier releases.
  These regressions use local HTTP/fake WS fixtures and the captured public
  liquidation shape; they do not certify live private trading or withdrawals.

The primary IDs hardened by this patch are order and trade IDs (including related
execution order IDs), funding offer/credit/loan/trade IDs, public trade IDs, and
account profile/ledger/movement IDs. Active-position IDs remain optional for legacy
short rows. Other optional integer metadata retains its existing compatibility
rules; this patch does not claim strict schema validation of every response field.
