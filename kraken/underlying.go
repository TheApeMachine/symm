package kraken

import "strings"

/*
Kraken names the same instrument twice.

A perpetual future is "PF_SOLUSD" on the futures venue and "SOL/USD" on the
spot venue. Nothing in either feed states that the two are the same market, so
anything that consumes both — a basis reading is meaningless otherwise — has to
resolve the relationship itself.

Leaving it unresolved is not a degraded reading, it is no reading at all: a
consumer that accumulates evidence per symbol files the derivative facts under
"PF_SOLUSD" and the spot facts under "SOL/USD", and the two never meet.
*/
const (
	perpetualPrefix = "PF_"
	inversePrefix   = "PI_"
)

/*
quoteSuffixes are the settlement currencies Kraken appends to a futures
product's base asset, longest first so "USDT" is not mistaken for "USD" with a
stray T on the base.
*/
var quoteSuffixes = []string{"USDT", "USDC", "USD", "EUR", "GBP", "XBT", "BTC"}

/*
venueAliases maps a futures venue's asset name onto the spot venue's name for
the same asset, where the two disagree.
*/
var venueAliases = map[string]string{
	"XBT": "BTC",
	"XDG": "DOGE",
}

/*
SpotSymbol resolves a futures product identity to the spot symbol for the same
underlying market, or returns false when the identity is not a futures product
this venue's naming covers.

It is deliberately a pure function of the identifier. Kraken publishes no field
relating the two venues, so the naming convention is the only fact available,
and a caller is better served by an explicit false than by a guess that quietly
files evidence against a market that does not exist.
*/
func SpotSymbol(product string) (string, bool) {
	remainder := ""

	switch {
	case strings.HasPrefix(product, perpetualPrefix):
		remainder = strings.TrimPrefix(product, perpetualPrefix)
	case strings.HasPrefix(product, inversePrefix):
		remainder = strings.TrimPrefix(product, inversePrefix)
	default:
		return "", false
	}

	for _, quote := range quoteSuffixes {
		if !strings.HasSuffix(remainder, quote) {
			continue
		}

		base := strings.TrimSuffix(remainder, quote)

		if base == "" {
			return "", false
		}

		if alias, found := venueAliases[base]; found {
			base = alias
		}

		if alias, found := venueAliases[quote]; found {
			quote = alias
		}

		return base + "/" + quote, true
	}

	return "", false
}
