# Upgrading from v1.0.0 to v1.1.0

Run `go get github.com/tigusigalpa/bitfinex-go@v1.1.0`. The module path and existing
service method signatures are retained. Review these corrected behaviors:

- Funding offer `Type` is `funding.TypeLimit`, `TypeFRRDeltaFix` or
  `TypeFRRDeltaVar`. `lend`/`loan` were invalid order types in v1.0.0; direction is
  represented by the sign of Amount. Negative offsets are accepted for FRRDELTAFIX.
- `Decimal.String()` preserves its lexeme; JSON and request bodies use canonical
  strings. Exact numeric value is unchanged. Absent decimal fields encode as null.
  Inputs or expanded representations over 1024 characters, or with exponent
  magnitude over 1024, are rejected.
- Invalid order combinations and cancel selectors now fail before I/O. Specify one
  cancel selector mode, valid UTC CID dates and documented price dependencies.
- REST `Book` maps `HistoryQuery.Limit` to `len`; history timestamps/sort are not
  valid book options. Raw funding books use the additive `BookLevel.RawFunding`.
- Error messages omit arbitrary server/body/transport text. `errors.As` still
  exposes API status/code and wrapped errors. Do not log unwrapped transport errors:
  a custom dependency can put sensitive data in them.
- Response headers are an allowlist of HTTP content/date/rate-limit metadata.
  Signed redirects are refused for built-in/injected `*http.Client` transports.
  Custom HTTPDoer implementations own their redirect/cancellation policy.
- A rate-limit penalty cools every default-limiter bucket in the client. Grouped
  private/unknown endpoints default to 10 requests per minute. Override deliberately
  if you have independently verified endpoint budgets. `WithRateLimiter(nil)` now
  explicitly disables local throttling, as originally documented.
- A cancelled initial WebSocket connection is closed; create a new client to retry.
  Close interrupts reconnect delays. Application sends honor maintenance pauses.
- Account snapshots start buffering immediately after authentication. Drain them
  promptly with SubscribeAccount. Every socket write is serialized.
- On `ErrConsumerTooSlow`, discard buffered derived state and subscribe again.
  The stream closes with one terminal error; `Subscription.Err()` also reports it.
  The terminal error may replace a queued event to avoid blocking the read loop.
- Integrity gaps/checksum mismatches reset the connection and recreate all tracked
  subscriptions. Treat new snapshots as authoritative replacements. Funding-book
  checksum semantics and non-default flags beyond SEQ_ALL/OB_CHECKSUM are unsupported.

See [endpoint coverage](endpoint-coverage.md) for remaining raw contracts and
[the review](review-v1.1.0.md) for evidence and limitations. No claim of complete
fulfilment of the original production-grade specification is made.
