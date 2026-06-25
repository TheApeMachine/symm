package broker

import (
	"strings"

	"github.com/theapemachine/datura"
)

/*
sizeBuy converts a risk fraction into an order quantity: the requested fraction
of free quote capital, priced at the live mark. It returns 0 (which the caller
rejects) when there is no mark or no free capital, so an unpriceable or unfunded
entry never dispatches a bogus size.
*/
func (desk *Desk) sizeBuy(symbol string, fraction float64) float64 {
	mark := desk.markFor(symbol)

	if mark <= 0 {
		return 0
	}

	free := desk.freeQuote()

	if free <= 0 {
		return 0
	}

	return (fraction * free) / mark
}

/*
freeQuote reads the free balance of the configured quote currency from the latest
balances frame in the tree — the same fill-mutated ledger paper and live both
publish through frame.Publish.
*/
func (desk *Desk) freeQuote() float64 {
	var (
		latest      *datura.Artifact
		latestStamp int64
	)

	for candidate := range desk.tree.Seek([]byte("balances/")) {
		if candidate.Timestamp() >= latestStamp {
			latest = candidate
			latestStamp = candidate.Timestamp()
		}
	}

	if latest == nil {
		return 0
	}

	for rowIndex := 0; ; rowIndex++ {
		asset := datura.Peek[string](latest, "asset", rowIndex, "asset")

		if asset == "" {
			return 0
		}

		if strings.ToUpper(asset) == desk.quote {
			return datura.Peek[float64](latest, "asset", rowIndex, "balance")
		}
	}
}
