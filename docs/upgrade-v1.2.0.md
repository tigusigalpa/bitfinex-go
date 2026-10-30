# Upgrading to v1.2.0

The module path and existing exported method signatures remain unchanged.
Use `go get github.com/tigusigalpa/bitfinex-go@v1.2.0`.

## Derivative collateral

The third argument to `SetDerivativeCollateral` is a decimal **amount**, never a
currency. The server returns `[0]` or `[1]`. The old method now parses the amount,
sends it as an exact string, and returns an SDK-created notification envelope
whose type is `collateral-status-adapter`. Prefer the native typed method:

```go
result, err := client.Positions.SetDerivativeCollateralAmount(
    ctx, "tBTCF0:USTF0", types.MustDecimal("100.25"),
)
if err != nil {
    return err
}
if !result.Data.Success {
    return errors.New("collateral change rejected")
}
```

Zero collateral is accepted; negative amounts and non-derivative symbols fail
locally. An ambiguous transport failure is never automatically retried.

## Typed replacements

Prefer these additive methods when replacing raw results:

| Area | Methods |
|---|---|
| Public data | `ConfigsTyped`, `TradingPairs`, `Currencies`, `VASPsTyped`, `MarketAveragePriceTyped` |
| Positions/funding | `MarginInfoTyped`, `IncreaseInfoTyped`, `DerivativeCollateralLimitsTyped`, `Funding.InfoTyped`, `AuditMany` |
| Account | `SummaryTyped`, `PermissionsTyped`, `GenerateTokenTyped`, `AuditHistoryTyped` |
| Settings/alerts | `SettingsTyped`, `SettingsSetTyped`, `SettingsDeleteTyped`, `AlertSetTyped`, `AlertDeleteTyped` |
| Payments | `DepositAddressesTyped`, `InvoicePaymentsTyped`, `MovementInfoTyped`, `InvoiceTyped` |
| Acknowledgements | `TransferTyped`, `DepositAddressTyped`, `WithdrawTyped` |
| Thalex | `ThalexDeposit`, `ThalexWithdraw`, `ThalexFreeTransferCountTyped` |
| OTC | `OTCHistoryTyped`, `OTCHistoryIterator` |

`types.Notification[T]` exposes the concrete data and server status. Check
`Success()` as well as the Go error. Failed notifications with a null payload
retain their status/code. Settings acknowledgements expose `Count` and `Success()`.

`GenerateTokenTyped` sends scope `api` and documented capability codes
`a/o/f/s/w/wd/bp`. Withdrawal capability requires explicit write permission.
`AuthToken.Value()` is the deliberate secret-access operation; formatting is
redacted and JSON serialization is refused. The legacy method also accepts its
former descriptive capability aliases and translates them to the documented codes.

Settings keys must have the `api:` prefix. Values are self-defined JSON, modeled
with immutable `types.JSONValue`; numbers retain their exact bytes. Accessors
return detached typed values. Opaque bank/browser metadata uses the same container.

Thalex requests require `Confirm: true`, a positive `Decimal`, a supported
currency (`USE`, `ETH`, `BTC`) and exactly one complete OTP/U2F factor. They are
never automatically retried. Transfer status/type names remain strings so new
server values can be reconciled by the application.

## Corrected behavior

- `DepositAddresses(ctx)` lacks the mandatory method parameter and now returns a
  local validation error. Use `DepositAddressesTyped` or its iterator.
- `InvoicePayments(ctx, limit)` accepts only zero, delegating to the payments
  list. This endpoint has actions and offsets, not a limit parameter. Migrate to
  `InvoicePaymentsTyped` or `InvoicePaymentsIterator` for explicit queries.
- `Invoice` returns an SDK `invoice-adapter` envelope around the native result.
  `InvoiceTyped` exposes the native five-field response. Requests require wallet
  `exchange`, currency `LNX` and 0.000001..0.02 BTC.
- Legacy `OTCHistory` maps native OTC rows to common Order fields with type `OTC`.
  The typed method preserves OTC initiator/counterparty details.
- Settings reads send `keys`; tokens send `scope: api`; Thalex quota reads send
  the documented capitalized `Provider` field.
- Average-price calculation uses POST query parameters and one symbol. Funding
  calculations require a period; `RateAvg` carries the funding rate.
- Position-history IDs/timestamps differ from active-position indexes and now
  decode correctly. Position audit sends an ID array. Login IP/timestamp/metadata,
  movement destination/transaction IDs, derivatives, liquidations, funding stats
  and rankings also follow the documented indexes.
- `Liquidation.Timestamp` preserves fractional milliseconds exactly; legacy
  `MTS` is the integer millisecond used by history filters. Funding statistics
  have `FundingBelowThreshold`; the legacy `AverageRate` field remains unset
  because the documented response has no such field.
- History limits and sort support are endpoint-specific. See [pagination](pagination.md).

WebSocket status events now populate `Event.DerivativeStatus` or
`Event.Liquidations`; authenticated calculations populate `AuthEvent.Balance`,
`Margin` or `Funding`. Legacy `Status`, `Info` and `Raw` remain available.
Unknown appended fields are tolerated; malformed required fields fail explicitly.
