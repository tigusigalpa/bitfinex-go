package types

import (
	"fmt"
	"regexp"
)

// Symbol is a validated Bitfinex market symbol: trading symbols
// start with "t" ("tBTCUSD", "tBTCF0:USTF0"), funding symbols
// start with "f" ("fUSD"). Symbols are validated fail-closed at
// the boundary so a malformed symbol never reaches the wire.
type Symbol string

var symbolRe = regexp.MustCompile(`^[tf][A-Z0-9:_]+$`)

// ParseSymbol validates s and returns a Symbol.
func ParseSymbol(s string) (Symbol, error) {
	if !symbolRe.MatchString(s) {
		return "", &ValidationError{
			Field:   "symbol",
			Message: fmt.Sprintf("invalid Bitfinex symbol %q (want e.g. \"tBTCUSD\" or \"fUSD\")", abbrev(s)),
		}
	}
	return Symbol(s), nil
}

// MustSymbol is ParseSymbol that panics on invalid input — for
// constants and tests only.
func MustSymbol(s string) Symbol {
	sym, err := ParseSymbol(s)
	if err != nil {
		panic(err)
	}
	return sym
}

// String returns the wire form of the symbol.
func (s Symbol) String() string { return string(s) }

// IsFunding reports whether the symbol denotes a funding currency
// ("fUSD") rather than a trading pair.
func (s Symbol) IsFunding() bool { return len(s) > 0 && s[0] == 'f' }

// IsTrading reports whether the symbol denotes a trading pair.
func (s Symbol) IsTrading() bool { return len(s) > 0 && s[0] == 't' }
