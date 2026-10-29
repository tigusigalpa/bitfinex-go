# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
