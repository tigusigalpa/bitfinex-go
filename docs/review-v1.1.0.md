# Review of v1.0.0 and hardening in v1.1.0

Review date: 2026-10-29. Baseline: immutable tag v1.0.0, commit
`20402ff27988d2aae49704ed64107728391925ec`. The initial snapshot was released before
code changes, as requested. No private Bitfinex operations or real funds were used.

## Findings addressed

| Priority | Baseline finding | Correction and regression evidence |
|---|---|---|
| P1 | Auth frame omitted mandatory `authNonce` | Include and validate nonce; `TestRepeatedReconnectReauth` checks signature and fresh nonce across four connections |
| P1 | First disconnect ended the read loop; later readiness signals could block | Distinguish handshake failure from established disconnect; notify initial waiter once |
| P1 | Close waited on a signal only its own read loop could close; handshake ignored caller cancellation | Lifecycle cancellation interrupts backoff, dial and handshake; bounded Close/cancellation tests |
| P1 | Concurrent sends called Gorilla WriteMessage without serialization | Context-aware write queue and deadlines; concurrent-writer regression |
| P1 | Every overflow created a waiting goroutine, risking send-on-closed and leaks | Synchronous terminal backpressure, stream closure and Err; 1000 undrained deliveries regression |
| P1 | BackpressureBlock could prevent shutdown | Lifecycle/subscription stop signals interrupt blocked delivery |
| P1 | Account snapshots could arrive before any subscriber existed | Install channel zero before returning authentication readiness |
| P1 | Decoder failures became apparently valid empty events; gaps still delivered corrupt updates | Propagate ProtocolError; discard corrupt deltas and rebuild all subscriptions |
| P1 | Arbitrary API body/message/headers and transport errors could expose private data | Redact untrusted error content; allowlist metadata; retain errors.Is/As access without unsafe formatted text |
| P1 | Default HTTP redirects could forward signed headers | Refuse signed redirects even for injected http.Client; two-server redirect regression |
| P1 | Funding offer types were lend/loan instead of documented order types | LIMIT/FRRDELTAFIX/FRRDELTAVAR, signed amount direction and exact input strings |
| P2 | Ticker history used current ticker indexes | Decode symbol/bid/ask/MTS at 0/1/3/12; validate length and trading-only history |
| P2 | REST book used limit instead of len; raw funding layout used trading indexes | Correct len query and add RawFundingBookEntry |
| P2 | Price/CID/cancel/history inputs allowed contradictory or malformed requests | Validate before I/O; order/window/nonce regression tables |
| P2 | Fractional/excessive decimal exponents could panic or allocate excessively; +value negation was malformed | Bounded parsing, exact canonical output and active Decimal fuzzing |
| P2 | JSON accepted trailing values; empty ticker rows could panic | Strict framing and length checks; malformed lists and active parser fuzzing |
| P2 | Rate limiter checked cooldown only before sleeping and could return spurious depletion | Recheck under lock; preserve the longest penalty and cool all local buckets |
| P2 | Retry budget excluded limiter waits/I/O; Retry-After dates were ignored; API wrappers hid base metadata | Bound the total idempotent request context, parse dates, retain typed unwrap, classify permanent failures conservatively |
| P2 | CI had invalid staticcheck/CodeQL references, obsolete Go versions and an inactive fuzz command | Verified pinned actions; Go 1.26/1.27; explicit active fuzz targets and tag checks |
| P2 | Baseline total coverage was 24.4% despite a 55% gate | Add cross-service success/error/routing tests and focused regressions; preserve the threshold |
| P2 | Coverage manifest had placeholder links and overstated complete typed support | Add concrete official references, deferred status and verification limits |

## Official contracts consulted

- [Requirements](https://docs.bitfinex.com/docs/requirements-and-limitations),
  [WebSocket general](https://docs.bitfinex.com/docs/ws-general),
  [authentication](https://docs.bitfinex.com/docs/ws-auth), and
  [checksum](https://docs.bitfinex.com/docs/ws-websocket-checksum).
- [Ticker](https://docs.bitfinex.com/reference/rest-public-ticker),
  [ticker history](https://docs.bitfinex.com/reference/rest-public-tickers-history),
  [book](https://docs.bitfinex.com/reference/rest-public-book),
  [candles](https://docs.bitfinex.com/reference/rest-public-candles).
- [Order submit](https://docs.bitfinex.com/reference/rest-auth-submit-order),
  [update](https://docs.bitfinex.com/reference/rest-auth-update-order),
  [cancel](https://docs.bitfinex.com/reference/rest-auth-cancel-order),
  [cancel-multi](https://docs.bitfinex.com/reference/rest-auth-cancel-orders-multiple).
- [Funding submit](https://docs.bitfinex.com/reference/rest-auth-submit-funding-offer),
  [transfer](https://docs.bitfinex.com/reference/rest-auth-transfer),
  [withdrawal](https://docs.bitfinex.com/reference/rest-auth-withdraw), and
  [derivative collateral](https://docs.bitfinex.com/reference/rest-auth-deriv-pos-collateral-set).

## Verification

The deterministic suite uses synthetic fixtures, fake connections and local HTTP
servers. The cross-service tests verify existing wrapper routing, signing and
success/error handling; they do not prove every legacy schema against the exchange.
Coverage includes imported packages exercised by cross-service tests through
`go test -coverpkg=./... -coverprofile=cover.out ./...`; the total gate is 55%.

The final release is checked with build, vet, unit tests, race tests, staticcheck,
govulncheck, four active fuzz targets and secret scanning. GitHub CI repeats the
quality/security suite on main and version tags. Exact final results are recorded
in the v1.1.0 release notes and the repository's Actions run.

Local Docker checks passed on Go 1.26.10 and 1.27.3, including race tests. The
Go 1.27 run passed build/vet/staticcheck and govulncheck reported no vulnerabilities.
Coverage reached 63.5% during hardening (final release notes record the final total).
All four five-second active fuzz campaigns passed. Gitleaks scanned the working
tree and Git history without live secrets; its sole baseline match was a public
synthetic HMAC test key, covered by an exact-value allowlist in `.gitleaks.toml`.

## Remaining work and limits

This review hardens the current SDK; it does not complete the original catalogue.

- Several services return `any`, raw notification data or partly typed information.
  Public wire decoders also remain exported. Additive typed replacements and a
  complete per-endpoint schema audit are planned for v1.2.0; removing legacy public
  signatures requires a major version.
- The legacy derivative collateral method takes a currency string although the
  documented API expects a collateral amount and returns a status array. It is
  marked deferred; do not rely on it. Other unaudited legacy wrappers may also
  need endpoint-specific contract corrections, even when their synthetic tests pass.
- History methods fetch one page. Reliable duplicate-safe iterators, retention
  verification, saturated timestamp-window handling and per-endpoint maxima remain
  deferred to v1.2.0.
- Shared rate-limit storage and nonce coordination are injectable dependencies,
  not built-in distributed implementations. Use separate keys per independent
  client; coordinate total traffic for clients sharing an IP. Connection pacing
  and cooldown are local to one client.
- CRC32 semantics for funding books and decimal formatting variants have not been
  fully audited. Trading-book CRC32 ordering has deterministic golden tests.
- Generic multi-operation maps are inherited and do not provide the same field
  validation as typed single-order requests. Validate/reconcile them in the caller.
- Rejected write notification status is data, not necessarily a Go error. Always
  check Success()/Status. Custom transport/nonce/dialer implementations must honor
  context and protect sensitive values; do not log unwrapped dependency errors.
- Real-API smoke/protected integration, optional SBOM/provenance and full exported
  API documentation remain future work. The repository does not contain trade keys.

The v1.0.0 tag remains unchanged. Use a current patched Go toolchain and review
the [upgrade notes](upgrade-v1.1.0.md) when adopting v1.1.0.
