package market

import (
	"github.com/tigusigalpa/bitfinex-go/internal/wire"
	"github.com/tigusigalpa/bitfinex-go/types"
)

// DerivativeStatusHistoryFromWire decodes history/WS status, which omits the key.
// Unstable positional helper for the isolated WebSocket subsystem.
func DerivativeStatusHistoryFromWire(a []any, key types.Symbol) (DerivativeStatus, error) {
	if e := wire.AssertMinLength(a, 23, "derivative status history"); e != nil {
		return DerivativeStatus{}, e
	}
	row := make([]any, 0, len(a)+1)
	row = append(row, key.String())
	row = append(row, a...)
	return derivStatusFromWire(row)
}

// LiquidationFromWire decodes the documented status/liquidations row.
// Unstable positional helper for the isolated WebSocket subsystem.
func LiquidationFromWire(a []any) (Liquidation, error) { return liquidationFromWire(a) }
func rankingFromWire(a []any) (RankingRow, error) {
	var out RankingRow
	if e := wire.AssertMinLength(a, 10, "ranking"); e != nil {
		return out, e
	}
	n, e := wire.MustInt(a, 0, "ranking timestamp")
	if e != nil {
		return out, e
	}
	out.MTS = types.Millis(n)
	out.Username, e = wire.RequiredString(a, 2)
	if e != nil {
		return out, e
	}
	out.Rank, e = wire.MustInt(a, 3, "ranking rank")
	if e != nil {
		return out, e
	}
	out.Value, e = wire.RequiredDecimal(a, 6)
	if e != nil {
		return out, e
	}
	out.TwitterHandle, e = wire.Str(a, 9)
	return out, e
}
