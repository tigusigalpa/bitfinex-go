package websocket

import (
	"hash/crc32"
	"sort"
	"strconv"
	"strings"

	"github.com/tigusigalpa/bitfinex-go/types"
)

// bookTop is the number of levels per side the checksum covers.
const bookTop = 25

// bookIntegrity is a local order book with Bitfinex checksum
// integrity. Enable with FlagChecksum; combine with FlagSeqAll
// for gap detection.
//
// P-precision books: checksum input = "price:amount" per level,
// bids sorted price-desc, asks price-asc, top 25 each.
// Raw R0 books: keyed by order id; ordering is price desc/asc
// and order-id asc within one price — do NOT mix the two rules.
type bookIntegrity struct {
	// aggregated P-books: price => amount
	bids map[string]string
	asks map[string]string

	// raw R0 books: orderID => [price, amount]
	rawBids map[int64][2]string
	rawAsks map[int64][2]string
}

func newBookIntegrity() *bookIntegrity {
	return &bookIntegrity{
		bids:    map[string]string{},
		asks:    map[string]string{},
		rawBids: map[int64][2]string{},
		rawAsks: map[int64][2]string{},
	}
}

func (b *bookIntegrity) clear() {
	b.bids = map[string]string{}
	b.asks = map[string]string{}
	b.rawBids = map[int64][2]string{}
	b.rawAsks = map[int64][2]string{}
}

// applyPrec applies one aggregated level. count=0 removes the
// price level; amount sign chooses bid/ask (positive = bid).
func (b *bookIntegrity) applyPrec(price string, count int64, amount string) {
	if count == 0 {
		delete(b.bids, price)
		delete(b.asks, price)
		return
	}
	if decIsNegative(amount) {
		b.asks[price] = amount
	} else {
		b.bids[price] = amount
	}
}

// applyRaw applies one raw order. price=0 removes the order id;
// amount sign chooses bid/ask.
func (b *bookIntegrity) applyRaw(orderID int64, price, amount string) {
	if decIsZero(price) {
		delete(b.rawBids, orderID)
		delete(b.rawAsks, orderID)
		return
	}
	if decIsNegative(amount) {
		delete(b.rawBids, orderID)
		b.rawAsks[orderID] = [2]string{price, amount}
	} else {
		delete(b.rawAsks, orderID)
		b.rawBids[orderID] = [2]string{price, amount}
	}
}

// checksum computes the signed CRC32 matching the server's 'cs'.
func (b *bookIntegrity) checksum(raw bool) int64 {
	var parts []string
	if raw {
		parts = b.rawParts()
	} else {
		parts = b.precParts()
	}
	crc := crc32.ChecksumIEEE([]byte(strings.Join(parts, ":")))
	if crc > 0x7FFFFFFF {
		return int64(crc) - 0x100000000
	}
	return int64(crc)
}

// verify compares the local checksum against the server's.
func (b *bookIntegrity) verify(serverChecksum int64, raw bool) bool {
	return b.checksum(raw) == serverChecksum
}

func (b *bookIntegrity) precParts() []string {
	bKeys := sortedDecKeys(b.bids, true)
	aKeys := sortedDecKeys(b.asks, false)
	parts := make([]string, 0, 4*bookTop)
	for i := 0; i < bookTop; i++ {
		if i < len(bKeys) {
			k := bKeys[i]
			parts = append(parts, k, b.bids[k])
		}
		if i < len(aKeys) {
			k := aKeys[i]
			parts = append(parts, k, b.asks[k])
		}
	}
	return parts
}

func (b *bookIntegrity) rawParts() []string {
	flatten := func(side map[int64][2]string, bids bool) [][2]string {
		// group by price → ids asc inside one price
		byPrice := map[string][]int64{}
		for id, pa := range side {
			byPrice[pa[0]] = append(byPrice[pa[0]], id)
		}
		prices := make([]string, 0, len(byPrice))
		for p := range byPrice {
			prices = append(prices, p)
		}
		sortDecStrings(prices, bids)
		out := make([][2]string, 0, len(side))
		for _, p := range prices {
			ids := byPrice[p]
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			for _, id := range ids {
				out = append(out, [2]string{strconv.FormatInt(id, 10), side[id][1]})
			}
		}
		return out
	}
	bids := flatten(b.rawBids, true)
	asks := flatten(b.rawAsks, false)
	parts := make([]string, 0, 4*bookTop)
	for i := 0; i < bookTop; i++ {
		if i < len(bids) {
			parts = append(parts, bids[i][0], bids[i][1])
		}
		if i < len(asks) {
			parts = append(parts, asks[i][0], asks[i][1])
		}
	}
	return parts
}

func sortedDecKeys(m map[string]string, desc bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortDecStrings(keys, desc)
	return keys
}

// sortDecStrings sorts decimal lexemes numerically (desc = bids).
func sortDecStrings(s []string, desc bool) {
	sort.Slice(s, func(i, j int) bool {
		di, _ := types.NewDecimal(s[i])
		dj, _ := types.NewDecimal(s[j])
		if desc {
			return di.Cmp(dj) > 0
		}
		return di.Cmp(dj) < 0
	})
}

func decIsNegative(s string) bool {
	d, err := types.NewDecimal(s)
	if err != nil {
		return false
	}
	return d.IsNegative()
}

func decIsZero(s string) bool {
	d, err := types.NewDecimal(s)
	if err != nil {
		return false
	}
	return d.IsZero()
}
