# Pagination in v1.2.0

Existing history methods return one page. Their additive `...Iterator` methods
fetch lazily through `Next(ctx)`, return the current record through `Value()`,
and expose terminal errors through `Err()`. Call `Close()` when stopping early.
An iterator is for one consumer and starts no background goroutines.

```go
it, err := client.Market.TradesIterator("tBTCUSD", &types.HistoryQuery{
    Limit: 100,
    Sort: types.SortAsc,
})
if err != nil {
    return err
}
defer it.Close()
for it.Next(ctx) {
    trade := it.Value()
    fmt.Println(trade.ID, trade.Price.String())
}
return it.Err()
```

The query is copied. If `End` is absent, construction freezes it at the current
time so an ascending traversal remains finite. A zero limit selects the smaller
of 100 and the endpoint maximum. Cancellation is checked before buffered records
as well as during HTTP, throttling and retries. Each page uses the client's
existing read retry and rate-limiting policy.

## Timestamp cursors

| Service / iterator | Maximum page size | Cursor field | Sort |
|---|---:|---|---|
| Market.TickersHistoryIterator | 250 | MTS | ascending / descending |
| Market.TradesIterator | 10000 | MTS | ascending / descending |
| Market.StatsIterator | 10000 | MTS | ascending / descending |
| Market.CandlesIterator | 10000 | MTS | ascending / descending |
| Market.DerivativesStatusHistoryIterator | 5000 | MTS | ascending / descending |
| Market.LiquidationsIterator | 500 | MTS, preserving exact fractional identity | ascending / descending |
| Market.RankingsIterator | 10000 | MTS + username identity | ascending / descending |
| Market.FundingStatsIterator | 250 | MTS | descending |
| Orders.HistoryIterator | 2500 | MTS_CREATE | descending |
| Orders.TradesIterator | 10000 | MTS_CREATE | ascending / descending |
| Orders.OTCHistoryIterator | 500 | MTS_CREATE | descending |
| Positions.HistoryIterator | 100 | MTS_UPDATE | descending |
| Positions.SnapshotIterator | 500 | MTS_UPDATE | descending |
| Positions.AuditIterator | 500 | MTS_UPDATE + position ID identity | descending |
| Funding.OffersHistoryIterator | 500 | MTS_UPDATE | descending |
| Funding.LoansHistoryIterator | 500 | MTS_UPDATE | descending |
| Funding.CreditsHistoryIterator | 500 | MTS_UPDATE | descending |
| Funding.TradesHistoryIterator | 10000 | MTS_CREATE | ascending / descending |
| Account.LedgersIterator | 2500 | MTS | descending |
| Account.LoginHistoryIterator | 250 | MTS | descending |
| Account.AuditHistoryIterator | 500 | MTS + complete observation identity | descending |
| Account.MovementsIterator | 1000 | MTS_UPDATED | descending |

Unsupported ascending sorts and excessive limits fail before I/O. Descending
sort is omitted from endpoints whose schema has no sort parameter.

The next page includes the last timestamp. Boundary identities are remembered,
so overlapping pages do not duplicate records. Full pages that remain at the
same millisecond cause the request limit to grow up to the endpoint maximum.
Position observations use ID plus update time, so multiple updates to one
position remain distinct. Only the current page and boundary identity set are
retained by timestamp iterators.

The API provides no ID cursor for overflowing milliseconds. If a full boundary
window remains at the maximum, the iterator yields any newly found observations
and then returns `*types.PaginationError`. It never advances by one millisecond
and silently drops possible records. Even a boundary containing exactly the
maximum cannot prove that no additional records exist. Narrow the request using
another server filter or reconcile through another source. Splitting only by
time cannot resolve multiple records in that same millisecond.

Malformed ordering, records outside the requested window, missing identities,
and responses larger than the requested limit return `ProtocolError`.

## Numbered and offset pages

`Account.DepositAddressesIterator(method, pageSize)` requires a deposit method.
Pages start at 1; page size is 20 by default and must be 20..100. A short page
ends traversal. `Account.InvoicePaymentsIterator(action)` supports
`getInvoicesByUser` and `getPaymentsByUser`, starts at offset 0 and advances by
the number of server records. An empty list ends traversal. ID lookups use
`InvoicePaymentsTyped` directly.

These iterators remember previously emitted address/wallet pairs or payment
hashes, using memory proportional to unique records. A replayed full address
page or a non-empty Lightning page with no new identity fails explicitly. Cursor
overflow also fails. They cannot provide snapshot isolation if the server inserts
or removes records while offsets/page numbers are being traversed.

Exchange retention still applies: order history about two weeks, order trades
about ten days, funding loan/credit history about three days, ledgers up to six
years. An empty response cannot distinguish expired history from absent data.
Liquidations use the separate documented 3 requests/minute bucket.

The endpoint links in [the inventory](endpoint-coverage.md) are authoritative for
limits. Funding/position cursors also agree with the official
[report generator](https://github.com/bitfinexcom/bfx-report/blob/0ecac1a/workers/loc.api/generate-report-file/report.file.job.data.js).
Order history follows its current documented MTS_CREATE filter; older report
code uses MTS_UPDATE. Responses inconsistent with the selected cursor fail the
ordering/window checks instead of being silently reordered.
