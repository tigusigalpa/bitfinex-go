# v1.2.0 contract and pagination review

Baseline: immutable v1.1.0, commit `e8767fe1a95804eef569b200e4c34c175be65740`.
Review date: 2026-10-30. The original specification is used as acceptance
criteria; no credentials or real trading operations were needed.

## Corrections and evidence

| Finding | Resolution | Regression evidence |
|---|---|---|
| Collateral was treated as a currency and decoded as a notification | Exact amount input and native status model, compatible legacy adapter | `TestV120TypedContracts/collateral*`, validation/no-retry cases |
| History had only one-page access | 22 lazy timestamp iterators plus address/Lightning cursors | `TestHistoryEndpointIterators`, `TestHistoryIteratorBoundaries`, saturation/cancellation/error cases |
| Summary, permissions, tokens, audit and settings were raw | Concrete account models and exact immutable self-defined JSON values | Typed REST fixtures, token redaction/serialization tests, JSON fuzzing |
| Payment, Thalex and OTC contracts were incomplete | Typed request/result schemas, OTP/U2F validation, native invoice and OTC decoding | Exact-body fixtures, malformed responses, rejection and HTTP 503 cases |
| Configurations lacked concrete variants | 20 ConfigKey variants and a typed union | `TestAllConfigurationVariants` including ordered batch routing |
| Positional models drifted from documented indexes | Correct history positions, logins, movements, derivative/history status, liquidations, rankings, funding stats | Public decoder and REST fixtures; fractional timestamp precision |
| Average-price, settings, token, audit and quota requests differed from docs | Correct POST query/body/ID-array/provider contracts | Complete outgoing-body checks |
| WS status and calculations remained raw | Typed derivative/liquidation and balance/margin/funding fields | `TestTypedStatusChannels`, `TestTypedAccountCalculations` |
| Liquidation throttling exceeded the endpoint budget | Separate 3 requests/minute bucket | Endpoint routing and default limiter configuration |

All new money amounts use `Decimal`. Unknown future Thalex statuses, permission
scopes and appended positional fields do not lose known data. Optional pending
margin values remain absent decimals. Unknown configuration keys fail before I/O.
Legacy raw return signatures are deprecated in favor of additive typed APIs.

## Sources

All formerly deferred endpoint pages were inspected, including their current
OpenAPI request schemas and native response examples. The exact endpoint links
remain in [the inventory](endpoint-coverage.md); complete external documentation
is not copied into this repository.

- [Collateral](https://docs.bitfinex.com/reference/rest-auth-deriv-pos-collateral-set),
  [collateral limits](https://docs.bitfinex.com/reference/rest-auth-calc-deriv-collateral-limits),
  [position history](https://docs.bitfinex.com/reference/rest-auth-positions-hist).
- [Public configurations](https://docs.bitfinex.com/reference/rest-public-conf),
  [average price](https://docs.bitfinex.com/reference/rest-public-market-average-price),
  [derivative status](https://docs.bitfinex.com/reference/rest-public-derivatives-status),
  [liquidations](https://docs.bitfinex.com/reference/rest-public-liquidations).
- [Lightning queries](https://docs.bitfinex.com/reference/lnx-invoice-payments),
  [invoice generation](https://docs.bitfinex.com/reference/rest-auth-deposit-invoice),
  [Thalex deposit](https://docs.bitfinex.com/reference/thalex-deposit-request),
  [withdrawal](https://docs.bitfinex.com/reference/thalex-withdrawal-request).
- Funding and position timestamp choices are cross-checked against the official
  [report generator](https://github.com/bitfinexcom/bfx-report/blob/0ecac1a/workers/loc.api/generate-report-file/report.file.job.data.js).
  Order history follows its newer documented MTS_CREATE filter. The paginator
  rejects inconsistent order/window data; it does not silently sort it.

## Verification scope

The suite uses synthetic documented shapes and local/fake transports. It checks
complete outgoing bodies and query strings, response values, exact decimals,
unknown tails, malformed data, HTTP failures, cancellation and pagination overlap.
New write cases enable retries before an HTTP 503 to verify a single submission.
Pagination constructors perform no I/O. Saturated millisecond windows fail
explicitly, including windows that cannot prove completeness at the server cap.

Coverage measured with `go test -coverpkg=./... -coverprofile=... ./...`: **74.2%**
on both CI toolchains. The existing 55% CI gate is retained.

[Release candidate CI](https://github.com/tigusigalpa/bitfinex-go/actions/runs/41001296487)
passed Go 1.26.10 and Go 1.27 tests, race detection, vet/build, all six active
fuzz targets, Staticcheck v0.8.2, Govulncheck v1.9.0, Gitleaks and CodeQL.
A separate Windows Go 1.26.4 run under severe host resource pressure passed the
new REST/pagination suites but timed out an existing socket-writer stress test;
the complete Linux CI runs, including that regression, passed on both versions.

This does not certify live private API behavior, snapshot isolation across
mutable exchange histories, or recovery of data outside exchange retention.
Shared IP/API-key deployments still coordinate limiter/nonce state through the
existing injection interfaces. See [migration](upgrade-v1.2.0.md) and
[pagination constraints](pagination.md) before upgrading private integrations.
