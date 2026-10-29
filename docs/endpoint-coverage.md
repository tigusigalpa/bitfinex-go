# Endpoint coverage — Bitfinex API v2

Version: **v1.1.0**. Review dates: **2026-10-29–30** (Europe/Moscow).

This is a coverage inventory, not a claim of full exchange compatibility. The
original v1.0.0 inventory overstated test coverage and used placeholder links.

**implemented** means a typed offline implementation with deterministic tests.
**deferred** means the requested complete typed contract is not delivered; some
legacy v1.0.0 wrappers remain callable for compatibility. Their raw/partial data
must be checked by the application. Typed replacements and remaining schema
verification are planned for **v1.2.0**; this is a roadmap, not a guarantee.

Fixtures for authenticated REST routing/signing, success decoding, and HTTP 503
errors are inline in `client_test.go::TestRESTServiceContracts`. These synthetic
fixtures exercise existing wrappers; they do not independently prove that every
legacy positional schema matches the exchange. Public fixture and malformed-data
tests are in `market/*test.go`; WebSocket tests are in `websocket/*test.go`.
The original labels below describe implementation scope; consult the review report
for the exact verification limits. No private exchange operations were performed.

Re-audited official contracts: ticker/history, REST book, candles, order submit/
update/cancel/cancel-multi, funding offer submit, transfer, withdrawal, WebSocket
requirements/auth/general/checksum. Other pages are reference links, not a claim
that a fresh complete schema audit was performed. See [review-v1.1.0.md](review-v1.1.0.md).

## Common specifications

| Spec | URL | Status |
|---|---|---|
| Introduction | https://docs.bitfinex.com/docs/introduction | n/a (read) |
| Requirements & limitations | https://docs.bitfinex.com/docs/requirements-and-limitations | n/a (enforced: nonce cap 2^53-1, rate limits, ws caps) |
| Public REST | https://docs.bitfinex.com/docs/rest-public | n/a |
| Authenticated REST | https://docs.bitfinex.com/docs/rest-auth | n/a |
| REST doc conventions | https://docs.bitfinex.com/docs/rest-reading-the-api-documentation | n/a |
| WS general | https://docs.bitfinex.com/docs/ws-general | n/a |
| Public WS | https://docs.bitfinex.com/docs/ws-public | n/a |
| Authenticated WS | https://docs.bitfinex.com/docs/ws-auth | n/a |
| WS checksum | https://docs.bitfinex.com/docs/ws-websocket-checksum | implemented (`websocket` bookIntegrity, CRC32 top-25) |
| Event/error glossary | https://docs.bitfinex.com/docs/abbreviations-glossary | n/a (numeric codes used) |
| Flag values | https://docs.bitfinex.com/docs/flag-values | implemented (`orders.Flag*`) |

## Public REST v2 (`market`)

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `GET /platform/status` | `Market.PlatformStatus(ctx)` | — | `int` | deferred (contract fixture, v1.2.0) | transport-only fixture | [Reference](https://docs.bitfinex.com/reference/rest-public-platform-status) |
| `GET /tickers` | `Market.Tickers(ctx, symbols...)` | symbols | `[]Ticker` | implemented | positive | [Reference](https://docs.bitfinex.com/reference/rest-public-tickers) |
| `GET /ticker/{symbol}` | `Market.Ticker(ctx, symbol)` | symbol | `Ticker` | implemented | pos+neg | [Reference](https://docs.bitfinex.com/reference/rest-public-ticker) |
| `GET /tickers/hist` | `Market.TickersHistory(ctx, symbols, q)` | symbols+`HistoryQuery` | `[]Ticker` | implemented | `market/hardening_test.go`: success + malformed history | [Reference](https://docs.bitfinex.com/reference/rest-public-tickers-history) |
| `GET /trades/{symbol}/hist` | `Market.Trades(ctx, symbol, q)` | `HistoryQuery` | `[]Trade` | implemented | positive | [Reference](https://docs.bitfinex.com/reference/rest-public-trades) |
| `GET /book/{symbol}/{prec}` | `Market.Book(ctx, symbol, prec, q)` | prec P0–P4/R0 | `[]BookLevel` | implemented | pos+neg | [Reference](https://docs.bitfinex.com/reference/rest-public-book) |
| `GET /stats1/{k}:{s}:{sym}:{side}/{sec}` | `Market.Stats(ctx, key,size,sym,side,sec,q)` | path params | `[]StatEntry` | deferred (contract fixtures, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-stats) |
| `GET /candles/{candle}/{sec}` | `Market.Candles(ctx, candle, sec, q)` | candle key | `[]Candle` | implemented | pos+neg | [Reference](https://docs.bitfinex.com/reference/rest-public-candles) |
| `GET /conf/pub:{a}:{o}:{d}` | `Market.Configs(ctx, a, o, d)` | path params | `any` (shape varies) | deferred (typed contract, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-conf) |
| `GET /status/deriv` | `Market.DerivativesStatus(ctx, keys...)` | keys | `[]DerivativeStatus` | deferred (contract fixtures, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-derivatives-status) |
| `GET /status/deriv/{key}/hist` | `Market.DerivativesStatusHistory(ctx, key, q)` | key | `[]DerivativeStatus` | deferred (contract fixtures, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-derivatives-status-history) |
| `GET /liquidations/hist` | `Market.Liquidations(ctx, q)` | `HistoryQuery` | `[]Liquidation` | deferred (contract fixtures, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-liquidations) |
| `GET /rankings/{k}:{tf}:{sym}/{sec}` | `Market.Rankings(ctx, k, tf, sym, sec, q)` | path params | `[]RankingRow` | deferred (typed contract, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-rankings) |
| `GET /funding/stats/{sym}/hist` | `Market.FundingStats(ctx, sym, q)` | `HistoryQuery` | `[]FundingStatsEntry` | deferred (contract fixtures, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-funding-stats) |
| `GET /ext/vasps` | `Market.VASPs(ctx)` | — | `any` (registry shape) | deferred (typed contract, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/virtual-asset-service-providers) |
| `POST /calc/trade/avg` | `Market.MarketAveragePrice(ctx, symbols, amount)` | symbols+Decimal | `MarketAvgPrice` | deferred (contract fixtures, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-market-average-price) |
| `POST /calc/fx` | `Market.ForeignExchangeRate(ctx, ccy1, ccy2)` | currencies | `Decimal` | deferred (contract fixtures, v1.2.0) | wrapper compile only; contract fixtures deferred | [Reference](https://docs.bitfinex.com/reference/rest-public-foreign-exchange-rate) |

## Authenticated REST — wallets / orders / trades / ledger

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/wallets` | `Wallets.All(ctx)` | — | `[]wallets.Wallet` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-wallets) |
| `POST /auth/r/orders[/{sym}]` | `Orders.Active(ctx, sym)` | optional symbol | `[]Order` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-retrieve-orders) |
| `POST /auth/w/order/submit` | `Orders.Submit(ctx, SubmitRequest)` | typed | `WriteResult` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-submit-order) |
| `POST /auth/w/order/update` | `Orders.Update(ctx, UpdateRequest)` | typed | `WriteResult` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-update-order) |
| `POST /auth/w/order/cancel` | `Orders.Cancel(ctx, CancelRequest)` | id or cid | `WriteResult` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-cancel-order) |
| `POST /auth/w/order/cancel/multi` | `Orders.CancelMulti(ctx, CancelMultiRequest)` | ids/cids/gids/all | `WriteResult` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-cancel-orders-multiple) |
| `POST /auth/w/order/multi` | `Orders.Multi(ctx, []MultiOp)` | ops | `WriteResult` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-order-multi) |
| `POST /auth/r/orders/hist`, `/{sym}/hist` | `Orders.History(ctx, sym, q)` | `HistoryQuery` | `[]Order` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-orders-history) |
| `POST /auth/r/order/{sym}:{id}/trades` | `Orders.OrderTrades(ctx, sym, id)` | path | `[]TradeExecution` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-order-trades) |
| `POST /auth/r/trades[/{sym}]/hist` | `Orders.Trades(ctx, sym, q)` | `HistoryQuery` | `[]TradeExecution` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-trades) |
| `POST /auth/r/orders/otc/{sym}/hist` | `Orders.OTCHistory(ctx, sym, q)` | `HistoryQuery` | `[]Order` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/otc-orders-history) |
| `POST /auth/r/ledgers[/{ccy}]/hist` | `Account.Ledgers(ctx, ccy, q)` | `HistoryQuery` | `[]LedgerEntry` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-ledgers) |

## Authenticated REST — positions / derivatives (`positions`)

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/info/margin/{key}` | `Positions.MarginInfo(ctx, key)` | key | `[]KeyValueInfo` | deferred (contract fixtures, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-info-margin) |
| `POST /auth/r/positions` | `Positions.Active(ctx)` | — | `[]Position` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-positions) |
| `POST /auth/w/position/claim` | `Positions.Claim(ctx, ClaimRequest)` | typed | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-position-claim) |
| `POST /auth/w/position/increase` | `Positions.Increase(ctx, IncreaseRequest)` | typed | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-position-increase) |
| `POST /auth/r/position/increase/info` | `Positions.IncreaseInfo(ctx, sym, amt)` | typed | `any` (preview) | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-increase-position-info) |
| `POST /auth/r/positions/hist` | `Positions.History(ctx, q)` | `HistoryQuery` | `[]Position` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-positions-hist) |
| `POST /auth/r/positions/snap` | `Positions.Snapshot(ctx, q)` | `HistoryQuery` | `[]Position` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-positions-snap) |
| `POST /auth/r/positions/audit` | `Positions.Audit(ctx, id, q)` | id+query | `[]Position` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-positions-audit) |
| `POST /auth/w/position/update/funding/type` | `Positions.UpdateFundingType(ctx, sym, type)` | 0/1 | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/update-position-funding-type) |
| `POST /auth/w/deriv/collateral/set` | `Positions.SetDerivativeCollateral(ctx, sym, ccy)` | typed | `Notification` | deferred (typed contract, v1.2.0) | no contract fixture; legacy mismatch documented | [Reference](https://docs.bitfinex.com/reference/rest-auth-deriv-pos-collateral-set) |
| `POST /auth/calc/deriv/collateral/limits` | `Positions.DerivativeCollateralLimits(ctx, sym)` | symbol | `any` (limits) | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-calc-deriv-collateral-limits) |

## Authenticated REST — margin funding (`funding`)

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/funding/offers[/{sym}]` | `Funding.Offers(ctx, sym)` | optional | `[]FundingOffer` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-offers) |
| `POST /auth/w/funding/offer/submit` | `Funding.SubmitOffer(ctx, OfferRequest)` | typed | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-submit-funding-offer) |
| `POST /auth/w/funding/offer/cancel` | `Funding.CancelOffer(ctx, id)` | id | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-cancel-funding-offer) |
| `POST /auth/w/funding/offer/cancel/all` | `Funding.CancelAllOffers(ctx, sym)` | symbol | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-cancel-all-funding-offers) |
| `POST /auth/w/funding/close` | `Funding.Close(ctx, id)` | id | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-close) |
| `POST /auth/w/funding/auto` | `Funding.AutoRenew(ctx, AutoRenewRequest)` | typed | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-auto-renew) |
| `POST /auth/w/funding/keep` | `Funding.Keep(ctx, type, ids)` | loan/credit | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-keep-funding) |
| `POST /auth/r/funding/offers/{sym}/hist` | `Funding.OffersHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingOffer` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-offers-hist) |
| `POST /auth/r/funding/loans/{sym}` | `Funding.Loans(ctx, sym)` | symbol | `[]FundingLoan` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-loans) |
| `POST /auth/r/funding/loans/{sym}/hist` | `Funding.LoansHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingLoan` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-loans-hist) |
| `POST /auth/r/funding/credits/{sym}` | `Funding.Credits(ctx, sym)` | symbol | `[]FundingCredit` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-credits) |
| `POST /auth/r/funding/credits/{sym}/hist` | `Funding.CreditsHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingCredit` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-credits-hist) |
| `POST /auth/r/funding/trades/{sym}/hist` | `Funding.TradesHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingTrade` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-funding-trades-hist) |
| `POST /auth/r/info/funding/{key}` | `Funding.Info(ctx, key)` | key | `[]KeyValueInfo` | deferred (contract fixtures, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-info-funding) |

## Authenticated REST — account / transfers / payments / alerts / settings (`account`)

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/info/user` | `Account.UserInfo(ctx)` | — | `UserInfo` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-info-user) |
| `POST /auth/r/summary` | `Account.Summary(ctx)` | — | `any` (shape varies) | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-summary) |
| `POST /auth/r/logins/hist` | `Account.LoginHistory(ctx, q)` | `HistoryQuery` | `[]LoginEntry` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-logins-hist) |
| `POST /auth/r/permissions` | `Account.Permissions(ctx)` | — | `any` (matrix) | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/key-permissions) |
| `POST /auth/w/token` | `Account.GenerateToken(ctx, ttl, caps, write)` | typed | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/generate-token) |
| `POST /auth/r/audit/hist` | `Account.AuditHistory(ctx, q)` | `HistoryQuery` | `any` (audit rows) | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-audit-hist) |
| `POST /auth/w/transfer` | `Account.Transfer(ctx, TransferRequest)` | typed+`Confirm` | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-transfer) |
| `POST /auth/calc/order/avail` | `Account.AvailableBalanceForOrder(ctx, sym, type, amt, rate, lev)` | typed | `Decimal` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-calc-order-avail) |
| `POST /auth/w/deposit/address` | `Account.DepositAddress(ctx, DepositAddressRequest)` | typed | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-deposit-address) |
| `POST /auth/r/deposit/address/all` | `Account.DepositAddresses(ctx)` | — | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/deposit-address-all) |
| `POST /auth/w/deposit/invoice` | `Account.Invoice(ctx, InvoiceRequest)` | typed+`Confirm` | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-deposit-invoice) |
| `POST /auth/r/ext/invoice/payments` | `Account.InvoicePayments(ctx, limit)` | limit | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/lnx-invoice-payments) |
| `POST /auth/w/withdraw` | `Account.Withdraw(ctx, WithdrawRequest)` | typed+`Confirm` | `Notification` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-withdraw) |
| `POST /auth/r/movements[/{ccy}]/hist` | `Account.Movements(ctx, ccy, q)` | `HistoryQuery` | `[]Movement` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-movements) |
| `POST /auth/r/movements/info` | `Account.MovementInfo(ctx, id)` | id | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/movement-info) |
| `POST /auth/r/alerts` | `Account.Alerts(ctx)` | — | `[]Alert` | implemented | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-alerts) |
| `POST /auth/w/alert/set` | `Account.AlertSet(ctx, AlertSetRequest)` | typed | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-alert-set) |
| `POST /auth/w/alert/price:{s}:{p}/del` | `Account.AlertDelete(ctx, sym, price)` | path | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-alert-del) |
| `POST /auth/w/settings/set` | `Account.SettingsSet(ctx, map)` | settings | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-settings-set) |
| `POST /auth/r/settings` | `Account.Settings(ctx, keys)` | keys | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-settings) |
| `POST /auth/w/settings/del` | `Account.SettingsDelete(ctx, keys)` | keys | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/rest-auth-settings-del) |
| `POST /auth/r/ext/wallets/transfers/free/count` | `Account.ThalexFreeTransferCount(ctx)` | — | `any` | deferred (typed contract, v1.2.0) | synthetic success + HTTP 503 (`client_test.go`) | [Reference](https://docs.bitfinex.com/reference/thalex-free-transfer-count) |
| `POST /auth/w/ext/wallets/deposits/request` | — | — | — | **deferred** | — | Thalex wire contract requires a verified request/response schema before a typed DTO ships; planned for v1.2.0 after official schema verification (https://docs.bitfinex.com/reference/thalex-deposit-request) |
| `POST /auth/w/ext/wallets/withdrawals/request` | — | — | — | **deferred** | — | Typed Thalex contract deferred to v1.2.0 pending official schema verification (https://docs.bitfinex.com/reference/thalex-withdrawal-request) |

## Public WebSocket v2 (`websocket`)

| Channel/input | Method | Events | Status | Tests | Docs |
|---|---|---|---|---|---|
| `ticker` | `Client.Subscribe(ctx, SubscribeTicker(sym))` | `Event.Ticker` (snapshot→updates), `hb` | implemented | fake-dialer e2e | [Reference](https://docs.bitfinex.com/reference/ws-public-ticker) |
| `trades` | `SubscribeTrades(sym)` | snapshot `[]Trade`, `te/tu` updates, `fte/ftu` for funding | implemented | fake-dialer e2e | [Reference](https://docs.bitfinex.com/reference/ws-public-trades) |
| `book` P0–P4 | `SubscribeBook(sym, prec, freq, len)` | `BookSnapshot`, `BookLevel` deltas, `cs` | implemented | checksum unit | [Reference](https://docs.bitfinex.com/reference/ws-public-books) |
| `book` R0 | `SubscribeBook(sym, "R0", …)` | raw order-id semantics (no mixing with P-rules) | implemented | book unit | [Reference](https://docs.bitfinex.com/reference/ws-public-raw-books) |
| `candles` | `SubscribeCandles(key)` | `[]Candle` snapshots/updates | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-public-candles) |
| `status` | `SubscribeStatus(key)` | `Status` raw payload | deferred (typed contract, v1.2.0) | decode | [Reference](https://docs.bitfinex.com/reference/ws-public-status) |

## Authenticated WebSocket v2 (`websocket`)

| Stream/input | Method | Events | Status | Tests | Docs |
|---|---|---|---|---|---|
| auth + filters | `ConnectAuth(ctx, AuthOptions{Filter, DeadManSwitch})` | `auth` ack; filter/dms opt-in only | implemented | auth-msg unit | [Reference](https://docs.bitfinex.com/reference/ws-auth-account-info) |
| `os/on/ou/oc` | `SubscribeAccount()` | `AuthEvent.Orders` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-orders) |
| `ps/pn/pu/pc` | `SubscribeAccount()` | `AuthEvent.Positions` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-positions) |
| `te/tu` | `SubscribeAccount()` | `AuthEvent.Trades` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-trades) |
| `fos/fon/fou/foc` | `SubscribeAccount()` | `AuthEvent.Offers` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-funding-offers) |
| `fcs/fcn/fcu/fcc` | `SubscribeAccount()` | `AuthEvent.Credits` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-funding-credits) |
| `fls/fln/flu/flc` | `SubscribeAccount()` | `AuthEvent.Loans` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-funding-loans) |
| `ws/wu` | `SubscribeAccount()` | `AuthEvent.Wallets` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-wallets) |
| `bu` | `SubscribeAccount()` | `AuthEvent.Info` (loose payload) | deferred (typed contract, v1.2.0) | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-balance-info) |
| `miu` | `SubscribeAccount()` | `AuthEvent.Info` | deferred (typed contract, v1.2.0) | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-margin-info) |
| `fiu` | `SubscribeAccount()` | `AuthEvent.Info` | deferred (typed contract, v1.2.0) | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-funding) |
| `fte/ftu` | `SubscribeAccount()` | `AuthEvent.FundingTrades` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-funding-trades) |
| `n` | `SubscribeAccount()` | `AuthEvent.Notification` | implemented | decode | [Reference](https://docs.bitfinex.com/reference/ws-auth-notifications) |
| input `on` | `Client.OrderNew(ctx, orders.SubmitRequest)` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-order-new) |
| input `ou` | `Client.OrderUpdate(ctx, orders.UpdateRequest)` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-order-update) |
| input `oc` | `Client.OrderCancel(ctx, orders.CancelRequest)` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-order-cancel) |
| input `oc_multi` | `Client.OrderCancelMulti(ctx, orders.CancelMultiRequest)` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-order-cancel-multi) |
| input `ox_multi` | `Client.OrderMulti(ctx, []orders.MultiOp)` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-order-multi-op) |
| input `fon` | `Client.OfferNew(ctx, funding.OfferRequest)` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-offer-new) |
| input `foc` | `Client.OfferCancel(ctx, offerID)` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-offer-cancel) |
| input `calc` | `Client.Calc(ctx, []string{…})` | typed input | implemented | validation | [Reference](https://docs.bitfinex.com/reference/ws-auth-input-calc) |

## Cross-cutting invariants (implemented + tested)

| Invariant | Where | Test |
|---|---|---|
| Signature input = exact transmitted bytes (`/api/`+apiPath+nonce+rawBody, HMAC-SHA384, lowercase hex) | `auth.Signer`, `httpx.Transport` | golden vectors + header test |
| Monotonic numeric nonce ≤ 2^53-1, concurrency-safe | `auth.MillisecondNonceProvider` | monotonic + concurrent tests |
| Money never float64 (`UseNumber`, lexeme `Decimal`) | `types.Decimal`, `internal/wire` | decimal tests, fuzz |
| Positional arrays: min length, tolerant tail | `internal/wire` | unit + fuzz |
| No retry on writes; classified retry on idempotent reads | `internal/retry`, `httpx` | classification tests |
| ≥60s cooldown after ERR_RATE_LIMIT/429, `Retry-After` | `internal/ratelimit` | cooldown tests |
| ctx honored in limiter wait/retry/IO | transport, limiter | cancellation test |
| No secrets in logs/errors/serializers | credentials, transport, models | redaction test |
| WS reconnect+re-auth+resubscribe, codes 20051/20060/20061 | `websocket` | unit coverage |
| Book integrity SEQ_ALL+OB_CHECKSUM, CRC32 top-25, resync | `websocket` | checksum + gap tests |
| Backpressure: no silent drop | `websocket` | ErrConsumerTooSlow test |

## Notes & honest limitations

- Raw `any` results, exposed wire-decoder helpers and untyped notification payloads
  are inherited from v1.0.0. They do not meet the original fully typed API goal.
  Removing or changing their signatures needs a future major version; additive
  typed replacements are planned for v1.2.0.
- A complete endpoint-specific schema/retention audit, persistent/shared rate
  limiting, cross-client nonce coordination, and duplicate-safe history iterators
  remain deferred to v1.2.0. History methods fetch one page only.
- Default throttling and WebSocket connection pacing are per client/process;
  applications sharing an API key or IP must coordinate their own aggregate use.
- Integrity checking applies to trading books. Funding-book checksum semantics
  are not verified. Raw R0 funding levels use `BookLevel.RawFunding`.
- Account streams `bu`/`miu`/`fiu`, status, and uncommon tags retain raw data.
- Rejected write notifications are returned as data: check `Success()` and the
  notification status as well as the Go error.
- Real-API public contract smoke and protected read-only integration workflows
  are not supplied by this release; normal CI makes no Bitfinex requests.
