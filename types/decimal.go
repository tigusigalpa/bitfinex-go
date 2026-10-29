package types

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Decimal is an immutable exact decimal value. It preserves the
// original lexeme, so values never pass through float64 and
// marshal back byte-identical. Use it for every money, price,
// amount, rate, fee, balance and P/L field.
//
// The zero value is invalid for arithmetic; construct via
// NewDecimal, MustDecimal, DecimalFromInt64, or DecimalZero.
type Decimal struct {
	lex string
}

var decimalRe = regexp.MustCompile(`^[+-]?(\d+(\.\d+)?|\.\d+)([eE][+-]?\d+)?$`)

// DecimalZero is the canonical zero value.
var DecimalZero = MustDecimal("0")

// NewDecimal parses a decimal lexeme such as "123.45", "-0.001",
// ".5" or "1e-8". The input is validated strictly; leading/trailing
// whitespace is rejected to keep wire representations canonical.
func NewDecimal(s string) (Decimal, error) {
	if s == "" || !decimalRe.MatchString(s) {
		return Decimal{}, fmt.Errorf("bitfinex: invalid decimal lexeme %q", abbrev(s))
	}
	return Decimal{lex: s}, nil
}

// MustDecimal is NewDecimal that panics on invalid input. Use it
// only for compile-time constants and tests.
func MustDecimal(s string) Decimal {
	d, err := NewDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// DecimalFromInt64 builds an exact integer decimal.
func DecimalFromInt64(v int64) Decimal {
	return Decimal{lex: big.NewInt(v).String()}
}

// String returns the original lexeme.
func (d Decimal) String() string { return d.lex }

// IsZero reports whether the value is exactly zero.
func (d Decimal) IsZero() bool { return d.Sign() == 0 }

// Sign returns -1, 0 or +1.
func (d Decimal) Sign() int { return d.rat().Sign() }

// IsNegative reports whether the value is strictly negative.
func (d Decimal) IsNegative() bool { return d.Sign() < 0 }

// Cmp returns -1, 0 or +1 comparing d to o.
func (d Decimal) Cmp(o Decimal) int { return d.rat().Cmp(o.rat()) }

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	if strings.HasPrefix(d.lex, "-") {
		return Decimal{lex: d.lex[1:]}
	}
	return Decimal{lex: "-" + d.lex}
}

// Abs returns |d|.
func (d Decimal) Abs() Decimal {
	if d.Sign() < 0 {
		return d.Neg()
	}
	return d
}

// Add returns d+o exactly.
func (d Decimal) Add(o Decimal) Decimal { return decFromRat(new(big.Rat).Add(d.rat(), o.rat())) }

// Sub returns d-o exactly.
func (d Decimal) Sub(o Decimal) Decimal { return decFromRat(new(big.Rat).Sub(d.rat(), o.rat())) }

// Mul returns d*o exactly.
func (d Decimal) Mul(o Decimal) Decimal { return decFromRat(new(big.Rat).Mul(d.rat(), o.rat())) }

// Float64 returns the nearest float64. LOSSY: provided only for
// display heuristics; never use it for order math or comparisons.
func (d Decimal) Float64() float64 {
	f, _ := d.rat().Float64()
	return f
}

// MarshalJSON emits the decimal as a JSON string ("123.45"), the
// representation Bitfinex documents for money fields. The lexeme
// is preserved byte-for-byte — no float round-trip ever occurs.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.lex + `"`), nil
}

// UnmarshalJSON accepts a JSON string or a bare JSON number and
// stores its exact lexeme.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*d = Decimal{}
		return nil
	}
	s = strings.Trim(s, `"`)
	nd, err := NewDecimal(s)
	if err != nil {
		return err
	}
	*d = nd
	return nil
}

// IsValid reports whether the Decimal holds a value. The zero
// value Decimal{} is invalid (absent); all methods below treat
// it as zero rather than panicking.
func (d Decimal) IsValid() bool { return d.lex != "" }

// rat parses the lexeme into an exact rational. NewDecimal's
// validation guarantees SetString succeeds; an empty (absent)
// Decimal behaves as zero.
func (d Decimal) rat() *big.Rat {
	if d.lex == "" {
		return new(big.Rat)
	}
	r, ok := new(big.Rat).SetString(d.lex)
	if !ok {
		panic(fmt.Sprintf("bitfinex: Decimal lexeme %q failed rational parse (bug)", d.lex))
	}
	return r
}

// decFromRat renders an exact rational back to a decimal lexeme.
// It succeeds iff the denominator's only prime factors are 2 and 5
// (always true for add/sub/mul of decimal inputs); otherwise it
// falls back to 50 significant fractional digits — never float64.
func decFromRat(r *big.Rat) Decimal {
	if r.Sign() == 0 {
		return DecimalZero
	}
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())

	// Strip factors 2 and 5 from the denominator.
	a, b := 0, 0
	two, five := big.NewInt(2), big.NewInt(5)
	rem := new(big.Int)
	q := new(big.Int)
	for {
		q.QuoRem(den, two, rem)
		if rem.Sign() != 0 {
			break
		}
		den.Set(q)
		a++
	}
	for {
		q.QuoRem(den, five, rem)
		if rem.Sign() != 0 {
			break
		}
		den.Set(q)
		b++
	}
	if den.Cmp(big.NewInt(1)) != 0 {
		// Non-terminating decimal (e.g. division): render with a
		// bounded, honest precision rather than silently rounding.
		return Decimal{lex: r.FloatString(30)}
	}

	scale := a
	if b > scale {
		scale = b
	}
	// value = num / (2^a * 5^b). Rendered with `scale` fractional
	// digits the integer is num * 2^(scale-a) * 5^(scale-b).
	scaled := new(big.Int).Set(num)
	if e := scale - a; e > 0 {
		scaled.Mul(scaled, new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(e)), nil))
	}
	if e := scale - b; e > 0 {
		scaled.Mul(scaled, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(e)), nil))
	}

	neg := scaled.Sign() < 0
	digits := scaled.Abs(scaled).String()
	for len(digits) <= scale {
		digits = "0" + digits
	}
	var lex string
	if scale == 0 {
		lex = digits
	} else {
		lex = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
		lex = strings.TrimRight(lex, "0")
		lex = strings.TrimRight(lex, ".")
		if lex == "" || lex == "-" {
			lex = "0"
		}
	}
	if neg && lex != "0" {
		lex = "-" + lex
	}
	return Decimal{lex: lex}
}

func abbrev(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}
