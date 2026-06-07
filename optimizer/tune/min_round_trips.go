package tune

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

const maxEffectiveMinRoundTrips = 12

/*
effectiveMinRoundTrips keeps sparse multi-strategy forests from being zeroed by a
symbol-count floor on short capture tapes.
*/
func effectiveMinRoundTrips(
	requested int,
	rows []types.Measurement,
	walletCurrency string,
) int {
	minRoundTrips := requested

	if minRoundTrips <= 0 {
		minRoundTrips = fundableSymbolCount(rows, walletCurrency)
	}

	if minRoundTrips > maxEffectiveMinRoundTrips {
		minRoundTrips = maxEffectiveMinRoundTrips
	}

	return minRoundTrips
}
