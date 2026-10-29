# Endpoint coverage — Bitfinex API v2

Audit date: **2026-10-29**. Wire contracts verified against the sibling reference
implementation (`bitfinex-php` by the same author) and the official pages linked below;
the official docs remain the sole source of truth.

Legend: **implemented** = typed method + decoder + at least one deterministic test path;
**deferred** = intentionally not yet implemented (reason + plan given).

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
| `GET /platform/status` | `Market.PlatformStatus(ctx)` | — | `int` | implemented | positive | link |
| `GET /tickers` | `Market.Tickers(ctx, symbols...)` | symbols | `[]Ticker` | implemented | positive | link |
| `GET /ticker/{symbol}` | `Market.Ticker(ctx, symbol)` | symbol | `Ticker` | implemented | pos+neg | link |
| `GET /tickers/hist` | `Market.TickersHistory(ctx, symbols, q)` | symbols+`HistoryQuery` | `[]Ticker` | implemented | compile | link |
| `GET /trades/{symbol}/hist` | `Market.Trades(ctx, symbol, q)` | `HistoryQuery` | `[]Trade` | implemented | positive | link |
| `GET /book/{symbol}/{prec}` | `Market.Book(ctx, symbol, prec, q)` | prec P0–P4/R0 | `[]BookLevel` | implemented | pos+neg | link |
| `GET /stats1/{k}:{s}:{sym}:{side}/{sec}` | `Market.Stats(ctx, key,size,sym,side,sec,q)` | path params | `[]StatEntry` | implemented | compile | link |
| `GET /candles/{candle}/{sec}` | `Market.Candles(ctx, candle, sec, q)` | candle key | `[]Candle` | implemented | pos+neg | link |
| `GET /conf/pub:{a}:{o}:{d}` | `Market.Configs(ctx, a, o, d)` | path params | `any` (shape varies) | implemented | compile | link |
| `GET /status/deriv` | `Market.DerivativesStatus(ctx, keys...)` | keys | `[]DerivativeStatus` | implemented | compile | link |
| `GET /status/deriv/{key}/hist` | `Market.DerivativesStatusHistory(ctx, key, q)` | key | `[]DerivativeStatus` | implemented | compile | link |
| `GET /liquidations/hist` | `Market.Liquidations(ctx, q)` | `HistoryQuery` | `[]Liquidation` | implemented | compile | link |
| `GET /rankings/{k}:{tf}:{sym}/{sec}` | `Market.Rankings(ctx, k, tf, sym, sec, q)` | path params | `[]RankingRow` | implemented | compile | link |
| `GET /funding/stats/{sym}/hist` | `Market.FundingStats(ctx, sym, q)` | `HistoryQuery` | `[]FundingStatsEntry` | implemented | compile | link |
| `GET /ext/vasps` | `Market.VASPs(ctx)` | — | `any` (registry shape) | implemented | compile | link |
| `POST /calc/trade/avg` | `Market.MarketAveragePrice(ctx, symbols, amount)` | symbols+Decimal | `MarketAvgPrice` | implemented | compile+val | link |
| `POST /calc/fx` | `Market.ForeignExchangeRate(ctx, ccy1, ccy2)` | currencies | `Decimal` | implemented | compile+val | link |

## Authenticated REST — wallets / orders / trades / ledger

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/wallets` | `Wallets.All(ctx)` | — | `[]wallets.Wallet` | implemented | wire decode | link |
| `POST /auth/r/orders[/{sym}]` | `Orders.Active(ctx, sym)` | optional symbol | `[]Order` | implemented | wire decode | link |
| `POST /auth/w/order/submit` | `Orders.Submit(ctx, SubmitRequest)` | typed | `WriteResult` | implemented | validation+signing | link |
| `POST /auth/w/order/update` | `Orders.Update(ctx, UpdateRequest)` | typed | `WriteResult` | implemented | validation | link |
| `POST /auth/w/order/cancel` | `Orders.Cancel(ctx, CancelRequest)` | id or cid | `WriteResult` | implemented | validation | link |
| `POST /auth/w/order/cancel/multi` | `Orders.CancelMulti(ctx, CancelMultiRequest)` | ids/cids/gids/all | `WriteResult` | implemented | validation | link |
| `POST /auth/w/order/multi` | `Orders.Multi(ctx, []MultiOp)` | ops | `WriteResult` | implemented | validation | link |
| `POST /auth/r/orders/hist`, `/{sym}/hist` | `Orders.History(ctx, sym, q)` | `HistoryQuery` | `[]Order` | implemented | wire decode | link |
| `POST /auth/r/order/{sym}:{id}/trades` | `Orders.OrderTrades(ctx, sym, id)` | path | `[]TradeExecution` | implemented | validation | link |
| `POST /auth/r/trades[/{sym}]/hist` | `Orders.Trades(ctx, sym, q)` | `HistoryQuery` | `[]TradeExecution` | implemented | wire decode | link |
| `POST /auth/r/orders/otc/{sym}/hist` | `Orders.OTCHistory(ctx, sym, q)` | `HistoryQuery` | `[]Order` | implemented | wire decode | link |
| `POST /auth/r/ledgers[/{ccy}]/hist` | `Account.Ledgers(ctx, ccy, q)` | `HistoryQuery` | `[]LedgerEntry` | implemented | wire decode | link |

## Authenticated REST — positions / derivatives (`positions`)

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/info/margin/{key}` | `Positions.MarginInfo(ctx, key)` | key | `[]KeyValueInfo` | implemented | compile | link |
| `POST /auth/r/positions` | `Positions.Active(ctx)` | — | `[]Position` | implemented | wire decode | link |
| `POST /auth/w/position/claim` | `Positions.Claim(ctx, ClaimRequest)` | typed | `Notification` | implemented | validation | link |
| `POST /auth/w/position/increase` | `Positions.Increase(ctx, IncreaseRequest)` | typed | `Notification` | implemented | validation | link |
| `POST /auth/r/position/increase/info` | `Positions.IncreaseInfo(ctx, sym, amt)` | typed | `any` (preview) | implemented | validation | link |
| `POST /auth/r/positions/hist` | `Positions.History(ctx, q)` | `HistoryQuery` | `[]Position` | implemented | wire decode | link |
| `POST /auth/r/positions/snap` | `Positions.Snapshot(ctx, q)` | `HistoryQuery` | `[]Position` | implemented | wire decode | link |
| `POST /auth/r/positions/audit` | `Positions.Audit(ctx, id, q)` | id+query | `[]Position` | implemented | validation | link |
| `POST /auth/w/position/update/funding/type` | `Positions.UpdateFundingType(ctx, sym, type)` | 0/1 | `Notification` | implemented | validation | link |
| `POST /auth/w/deriv/collateral/set` | `Positions.SetDerivativeCollateral(ctx, sym, ccy)` | typed | `Notification` | implemented | validation | link |
| `POST /auth/calc/deriv/collateral/limits` | `Positions.DerivativeCollateralLimits(ctx, sym)` | symbol | `any` (limits) | implemented | compile | link |

## Authenticated REST — margin funding (`funding`)

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/funding/offers[/{sym}]` | `Funding.Offers(ctx, sym)` | optional | `[]FundingOffer` | implemented | wire decode | link |
| `POST /auth/w/funding/offer/submit` | `Funding.SubmitOffer(ctx, OfferRequest)` | typed | `Notification` | implemented | validation | link |
| `POST /auth/w/funding/offer/cancel` | `Funding.CancelOffer(ctx, id)` | id | `Notification` | implemented | validation | link |
| `POST /auth/w/funding/offer/cancel/all` | `Funding.CancelAllOffers(ctx, sym)` | symbol | `Notification` | implemented | validation | link |
| `POST /auth/w/funding/close` | `Funding.Close(ctx, id)` | id | `Notification` | implemented | validation | link |
| `POST /auth/w/funding/auto` | `Funding.AutoRenew(ctx, AutoRenewRequest)` | typed | `Notification` | implemented | body | link |
| `POST /auth/w/funding/keep` | `Funding.Keep(ctx, type, ids)` | loan/credit | `Notification` | implemented | validation | link |
| `POST /auth/r/funding/offers/{sym}/hist` | `Funding.OffersHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingOffer` | implemented | wire decode | link |
| `POST /auth/r/funding/loans/{sym}` | `Funding.Loans(ctx, sym)` | symbol | `[]FundingLoan` | implemented | wire decode | link |
| `POST /auth/r/funding/loans/{sym}/hist` | `Funding.LoansHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingLoan` | implemented | wire decode | link |
| `POST /auth/r/funding/credits/{sym}` | `Funding.Credits(ctx, sym)` | symbol | `[]FundingCredit` | implemented | wire decode | link |
| `POST /auth/r/funding/credits/{sym}/hist` | `Funding.CreditsHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingCredit` | implemented | wire decode | link |
| `POST /auth/r/funding/trades/{sym}/hist` | `Funding.TradesHistory(ctx, sym, q)` | `HistoryQuery` | `[]FundingTrade` | implemented | wire decode | link |
| `POST /auth/r/info/funding/{key}` | `Funding.Info(ctx, key)` | key | `[]KeyValueInfo` | implemented | compile | link |

## Authenticated REST — account / transfers / payments / alerts / settings (`account`)

| Endpoint | Method | Request | Response | Status | Tests | Docs |
|---|---|---|---|---|---|---|
| `POST /auth/r/info/user` | `Account.UserInfo(ctx)` | — | `UserInfo` | implemented | wire decode | link |
| `POST /auth/r/summary` | `Account.Summary(ctx)` | — | `any` (shape varies) | implemented | compile | link |
| `POST /auth/r/logins/hist` | `Account.LoginHistory(ctx, q)` | `HistoryQuery` | `[]LoginEntry` | implemented | wire decode | link |
| `POST /auth/r/permissions` | `Account.Permissions(ctx)` | — | `any` (matrix) | implemented | compile | link |
| `POST /auth/w/token` | `Account.GenerateToken(ctx, ttl, caps, write)` | typed | `any` | implemented | validation | link |
| `POST /auth/r/audit/hist` | `Account.AuditHistory(ctx, q)` | `HistoryQuery` | `any` (audit rows) | implemented | compile | link |
| `POST /auth/w/transfer` | `Account.Transfer(ctx, TransferRequest)` | typed+`Confirm` | `Notification` | implemented | validation | link |
| `POST /auth/calc/order/avail` | `Account.AvailableBalanceForOrder(ctx, sym, type, amt, rate, lev)` | typed | `Decimal` | implemented | validation | link |
| `POST /auth/w/deposit/address` | `Account.DepositAddress(ctx, DepositAddressRequest)` | typed | `Notification` | implemented | validation | link |
| `POST /auth/r/deposit/address/all` | `Account.DepositAddresses(ctx)` | — | `any` | implemented | compile | link |
| `POST /auth/w/deposit/invoice` | `Account.Invoice(ctx, InvoiceRequest)` | typed+`Confirm` | `Notification` | implemented | validation | link |
| `POST /auth/r/ext/invoice/payments` | `Account.InvoicePayments(ctx, limit)` | limit | `any` | implemented | compile | link |
| `POST /auth/w/withdraw` | `Account.Withdraw(ctx, WithdrawRequest)` | typed+`Confirm` | `Notification` | implemented | validation | link |
| `POST /auth/r/movements[/{ccy}]/hist` | `Account.Movements(ctx, ccy, q)` | `HistoryQuery` | `[]Movement` | implemented | wire decode | link |
| `POST /auth/r/movements/info` | `Account.MovementInfo(ctx, id)` | id | `any` | implemented | validation | link |
| `POST /auth/r/alerts` | `Account.Alerts(ctx)` | — | `[]Alert` | implemented | wire decode | link |
| `POST /auth/w/alert/set` | `Account.AlertSet(ctx, AlertSetRequest)` | typed | `any` | implemented | validation | link |
| `POST /auth/w/alert/price:{s}:{p}/del` | `Account.AlertDelete(ctx, sym, price)` | path | `any` | implemented | validation | link |
| `POST /auth/w/settings/set` | `Account.SettingsSet(ctx, map)` | settings | `any` | implemented | validation | link |
| `POST /auth/r/settings` | `Account.Settings(ctx, keys)` | keys | `any` | implemented | compile | link |
| `POST /auth/w/settings/del` | `Account.SettingsDelete(ctx, keys)` | keys | `any` | implemented | validation | link |
| `POST /auth/r/ext/wallets/transfers/free/count` | `Account.ThalexFreeTransferCount(ctx)` | — | `any` | implemented | compile | link |
| `POST /auth/w/ext/wallets/deposits/request` | — | — | — | **deferred** | — | Thalex wire contract requires a verified request/response schema before a typed DTO ships; planned for the next minor after a docs audit + sandbox verification (https://docs.bitfinex.com/reference/thalex-deposit-request) |
| `POST /auth/w/ext/wallets/withdrawals/request` | — | — | — | **deferred** | — | same reason (https://docs.bitfinex.com/reference/thalex-withdrawal-request) |

## Public WebSocket v2 (`websocket`)

| Channel/input | Method | Events | Status | Tests | Docs |
|---|---|---|---|---|---|
| `ticker` | `Client.Subscribe(ctx, SubscribeTicker(sym))` | `Event.Ticker` (snapshot→updates), `hb` | implemented | fake-dialer e2e | link |
| `trades` | `SubscribeTrades(sym)` | snapshot `[]Trade`, `te/tu` updates, `fte/ftu` for funding | implemented | fake-dialer e2e | link |
| `book` P0–P4 | `SubscribeBook(sym, prec, freq, len)` | `BookSnapshot`, `BookLevel` deltas, `cs` | implemented | checksum unit | link |
| `book` R0 | `SubscribeBook(sym, "R0", …)` | raw order-id semantics (no mixing with P-rules) | implemented | book unit | link |
| `candles` | `SubscribeCandles(key)` | `[]Candle` snapshots/updates | implemented | decode | link |
| `status` | `SubscribeStatus(key)` | `Status` raw payload | implemented | decode | link |

## Authenticated WebSocket v2 (`websocket`)

| Stream/input | Method | Events | Status | Tests | Docs |
|---|---|---|---|---|---|
| auth + filters | `ConnectAuth(ctx, AuthOptions{Filter, DeadManSwitch})` | `auth` ack; filter/dms opt-in only | implemented | auth-msg unit | link |
| `os/on/ou/oc` | `SubscribeAccount()` | `AuthEvent.Orders` | implemented | decode | link |
| `ps/pn/pu/pc` | `SubscribeAccount()` | `AuthEvent.Positions` | implemented | decode | link |
| `te/tu` | `SubscribeAccount()` | `AuthEvent.Trades` | implemented | decode | link |
| `fos/fon/fou/foc` | `SubscribeAccount()` | `AuthEvent.Offers` | implemented | decode | link |
| `fcs/fcn/fcu/fcc` | `SubscribeAccount()` | `AuthEvent.Credits` | implemented | decode | link |
| `fls/fln/flu/flc` | `SubscribeAccount()` | `AuthEvent.Loans` | implemented | decode | link |
| `ws/wu` | `SubscribeAccount()` | `AuthEvent.Wallets` | implemented | decode | link |
| `bu` | `SubscribeAccount()` | `AuthEvent.Info` (loose payload) | implemented | decode | link |
| `miu` | `SubscribeAccount()` | `AuthEvent.Info` | implemented | decode | link |
| `fiu` | `SubscribeAccount()` | `AuthEvent.Info` | implemented | decode | link |
| `fte/ftu` | `SubscribeAccount()` | `AuthEvent.FundingTrades` | implemented | decode | link |
| `n` | `SubscribeAccount()` | `AuthEvent.Notification` | implemented | decode | link |
| input `on` | `Client.OrderNew(ctx, orders.SubmitRequest)` | typed input | implemented | validation | link |
| input `ou` | `Client.OrderUpdate(ctx, orders.UpdateRequest)` | typed input | implemented | validation | link |
| input `oc` | `Client.OrderCancel(ctx, orders.CancelRequest)` | typed input | implemented | validation | link |
| input `oc_multi` | `Client.OrderCancelMulti(ctx, orders.CancelMultiRequest)` | typed input | implemented | validation | link |
| input `ox_multi` | `Client.OrderMulti(ctx, []orders.MultiOp)` | typed input | implemented | validation | link |
| input `fon` | `Client.OfferNew(ctx, funding.OfferRequest)` | typed input | implemented | validation | link |
| input `foc` | `Client.OfferCancel(ctx, offerID)` | typed input | implemented | validation | link |
| input `calc` | `Client.Calc(ctx, []string{…})` | typed input | implemented | validation | link |

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

- Responses marked `any` are endpoints whose payload shape varies or is loosely documented;
  they preserve the decoded wire data rather than inventing a wrong schema — none of them
  are money-critical paths that would hide a typed model.
- `rankings`, `configs`, `vasps`, `summary`, `permissions`, `audit`, `settings`,
  `deposit/address/all`, `invoice/payments`, `movements/info`, `calc/deriv/collateral/limits`,
  `position/increase/info`, Thalex free count return partially-typed or raw payloads.
- Integration tests against the real API are manual/nightly only; CI never calls Bitfinex.
