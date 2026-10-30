# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.0] - 2026-10-30

### Added

- Lazy context-aware iterators for 22 timestamp histories, numbered deposit
  address pages and offset-based Lightning invoice/payment lists. Inclusive
  boundaries are de-duplicated; saturated windows fail explicitly.
- Typed public configurations (20 variants), VASPs, rankings, margin/funding
  calculations, position previews and collateral limits, account summaries,
  permissions, redacted tokens, audit entries, settings, movement details,
  alerts, payment acknowledgements and Thalex transfers with OTP/U2F.
- Typed WebSocket derivative/liquidation status and balance/margin/funding info.
- Exact immutable JSON values for self-defined settings and opaque metadata.

### Fixed

- SetDerivativeCollateral accepts an amount and parses the native status result;
  SetDerivativeCollateralAmount accepts Decimal. The old return type is retained
  through an explicitly documented adapter.
- Correct position-history, login, movement, OTC, derivative-status, liquidation,
  funding-statistics and ranking schemas, invoice result shape, settings/token
  bodies, required deposit method, average-price POST query and endpoint limits.
- Preserve exact fractional liquidation timestamps; throttle liquidations at the
  documented 3 requests/minute. Money-moving writes remain non-retryable.

### Verification

- Expanded exact-body/success/malformed/server-error fixtures and HTTP pagination
  tests; active fuzzing includes new typed calculations and exact JSON values.
- Migration details and API constraints are documented in docs/upgrade-v1.2.0.md
  and docs/pagination.md.

## [1.1.0] - 2026-10-30

### Fixed

- WebSocket authentication now includes `authNonce`. Reconnect survives the first
  disconnect and repeated generations, re-authenticates with fresh nonces, and
  keeps account snapshots. Cancellation and Close interrupt handshakes/backoff.
- Serialize concurrent socket writes; bound frames, handshake and idle reads;
  propagate decoder failures; drop corrupt deltas and rebuild connections after
  integrity failures. Backpressure terminates once without unbounded goroutines.
- Correct ticker-history indexes, REST book `len`, raw funding-book layout,
  funding offer types and canonical monetary request strings.
- Validate order dependencies, CID dates/ranges, cancel selectors, history windows,
  nonces, invalid options and typed nil dependencies before I/O.
- Redact server-error bodies/messages, restrict HTTP response metadata, block signed
  redirects, bound responses and retry elapsed time, parse HTTP-date Retry-After,
  expose wrapped APIError metadata, and avoid retrying permanent transport errors.
- Recheck limiter state after waits, preserve extended penalties and apply server
  cooldown across all buckets; use conservative shared-group defaults.
- Bound decimal parsing and fix signed negation, strict JSON framing and integer
  formatting at the int64 boundary.

### Added

- Cross-service offline REST success/error/routing tests and WebSocket lifecycle,
  concurrency, integrity and backpressure regressions; active fuzzing of four parsers.
- Current Go 1.26/1.27 CI, valid pinned actions, dependency review, security checks,
  and full checks on release tags. Coverage threshold remains 55%.
- Versioned review and upgrade notes; concrete endpoint reference links and honest
  deferred statuses for raw/partial contracts.

## [1.0.0] - 2026-10-29

Original supplied SDK snapshot. The entries below describe its intended scope;
the v1.1.0 review corrects overstated verification and coverage claims.

### Added

- Initial public release candidate: Bitfinex API v2 client (REST + WebSocket).
- Public REST: platform status, tickers, ticker, tickers history, trades,
  book (P0–P4 + R0), stats, candles, configs, derivatives status (+history),
  liquidations, rankings, funding stats, VASPs, calc/trade/avg, calc/fx.
- Authenticated REST: wallets; orders (active/submit/update/cancel/cancel-multi/
  multi-op/history/order-trades/trades/OTC history); ledgers; positions
  (active/history/snap/audit/claim/increase/increase-info/funding-type/deriv
  collateral set+limits/margin info); funding (offers CRUD, close, auto-renew,
  keep, loans, credits, trades, info); account (user info, summary, logins,
  permissions, token, audit, ledgers, calc order avail, alerts, settings,
  transfer, deposit address (+list), invoice, invoice payments, withdraw,
  movements, movement info, Thalex free transfer count).
- WebSocket v2: public channels (ticker/trades/book R0+P0–P4/candles/status),
  authenticated channel-0 streams (orders, positions, trades, funding,
  wallets, balance/margin/funding info, notifications) and typed inputs
  (on/ou/oc/oc_multi/ox_multi/fon/foc/calc); heartbeat, ping/pong, capped
  full-jitter reconnect, fresh-nonce re-auth, confirmed resubscribe;
  SEQ_ALL + OB_CHECKSUM integrity mode with CRC32 top-25 validation and
  automatic resync on sequence gaps/checksum mismatches.
- Exact lexeme-preserving `Decimal` (no float64 anywhere near money).
- Endpoint-aware rate limiter with ≥60s ERR_RATE_LIMIT cooldown; idempotent-
  only retries with capped full-jitter backoff; writes never auto-retried.
- Golden-vector signature tests, deterministic fixtures, fuzz tests for the
  positional decoder, race-tested concurrency.

### Deferred

- Thalex deposit/withdrawal request endpoints (`auth/w/ext/wallets/deposits/request`,
  `auth/w/ext/wallets/withdrawals/request`): wire contract needs a live
  verification pass before a typed DTO ships. See docs/endpoint-coverage.md.
- Authenticated WS `auth` event payload fields beyond typed streams
  (e.g. uncommon info keys) surface via `AuthEvent.Info`/`Raw`.
