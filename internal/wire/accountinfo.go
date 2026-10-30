package wire

import "github.com/tigusigalpa/bitfinex-go/types"

// MarginInfo decodes base/sym calculations shared by REST and WebSocket.
func MarginInfo(v any) (types.MarginInfo, error) {
	var out types.MarginInfo
	a, e := AsArr(v, "margin info")
	if e != nil {
		return out, e
	}
	if e = AssertMinLength(a, 2, "margin info"); e != nil {
		return out, e
	}
	out.Type, e = RequiredString(a, 0)
	if e != nil {
		return out, e
	}
	index := 1
	if out.Type == "sym" {
		s, e := RequiredString(a, 1)
		if e != nil {
			return out, e
		}
		out.Symbol = types.Symbol(s)
		index = 2
	} else if out.Type != "base" {
		return out, &types.ProtocolError{Message: "unknown margin calculation type"}
	}
	data, e := AsArr(At(a, index), "margin calculation")
	if e != nil {
		return out, e
	}
	count := 4
	if out.Type == "base" {
		count = 5
	}
	if e = AssertMinLength(data, count, "margin calculation"); e != nil {
		return out, e
	}
	d := make([]types.Decimal, count)
	for i := range d {
		d[i], _, e = Decimal(data, i)
		if e != nil {
			return out, e
		}
	}
	if out.Type == "base" {
		out.Base = &types.MarginBase{PL: d[0], Swaps: d[1], Balance: d[2], Net: d[3], Required: d[4]}
	} else {
		out.Market = &types.MarginSymbol{TradableBalance: d[0], GrossBalance: d[1], Buy: d[2], Sell: d[3]}
	}
	return out, nil
}

// FundingInfo decodes the documented sym envelope, including pending null values.
func FundingInfo(v any) (types.FundingInfo, error) {
	var out types.FundingInfo
	a, e := AsArr(v, "funding info")
	if e != nil {
		return out, e
	}
	if e = AssertMinLength(a, 3, "funding info"); e != nil {
		return out, e
	}
	out.Type, e = RequiredString(a, 0)
	if e != nil {
		return out, e
	}
	s, e := RequiredString(a, 1)
	if e != nil {
		return out, e
	}
	out.Symbol = types.Symbol(s)
	d, e := AsArr(a[2], "funding calculation")
	if e != nil {
		return out, e
	}
	if e = AssertMinLength(d, 4, "funding calculation"); e != nil {
		return out, e
	}
	fields := []*types.Decimal{&out.YieldLoan, &out.YieldLend, &out.DurationLoan, &out.DurationLend}
	for i, p := range fields {
		*p, _, e = Decimal(d, i)
		if e != nil {
			return out, e
		}
	}
	return out, nil
}

// Status decodes exactly the documented bool/0/1 first status field.
func Status(v any) (types.OperationStatus, error) {
	a, e := AsArr(v, "operation status")
	if e != nil {
		return types.OperationStatus{}, e
	}
	if e = AssertMinLength(a, 1, "operation status"); e != nil {
		return types.OperationStatus{}, e
	}
	if b, ok := a[0].(bool); ok {
		return types.OperationStatus{Success: b}, nil
	}
	n, e := MustInt(a, 0, "operation status")
	if e != nil {
		return types.OperationStatus{}, e
	}
	if n != 0 && n != 1 {
		return types.OperationStatus{}, &types.ProtocolError{Message: "invalid operation status"}
	}
	return types.OperationStatus{Success: n == 1}, nil
}

// Notification decodes a typed notification using a domain payload decoder.
func Notification[T any](v any, decode func(any) (T, error)) (types.Notification[T], error) {
	var n types.Notification[T]
	a, e := AsArr(v, "notification")
	if e != nil {
		return n, e
	}
	if e = AssertMinLength(a, 8, "notification"); e != nil {
		return n, e
	}
	n.MTS, _, e = Mts(a, 0)
	if e != nil {
		return n, e
	}
	n.Type, e = RequiredString(a, 1)
	if e != nil {
		return n, e
	}
	n.MessageID, _, e = Int(a, 2)
	if e != nil {
		return n, e
	}
	n.Code, _, e = Int(a, 5)
	if e != nil {
		return n, e
	}
	n.Status, e = RequiredString(a, 6)
	if e != nil {
		return n, e
	}
	n.Text, e = Str(a, 7)
	if e != nil {
		return n, e
	}
	if n.Status != "SUCCESS" && a[4] == nil {
		return n, nil
	}
	n.Data, e = decode(a[4])
	return n, e
}
