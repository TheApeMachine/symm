package broker

import (
	"strings"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
readingValue returns the defined value of a Perspective reading matched by its
metric slot's trailing "/<metric>/sample" suffix. It is the structural read the
continuation context needs from a Perspective without importing the advisor
package: the reading identity is the interned slot name the advisor publishes,
whose canonical suffix is "/<metric>/sample".
*/
func readingValue(perspective *types.Perspective, metric string) (float64, bool) {
	if perspective == nil {
		return 0, false
	}

	for index := 0; index < perspective.Count; index++ {
		reading := perspective.Readings[index]

		name, found := nmtypes.SymbolName(reading.Metric)

		if !found {
			continue
		}

		if strings.HasSuffix(name, "/"+metric+"/sample") {
			if !reading.Defined {
				return 0, false
			}

			return reading.Value, true
		}
	}

	return 0, false
}

/*
continuationSupportive is the long-position profit-stagnation continuation
context (the single soft-exit deferral allowed this pass). It returns true only
when the mandatory flow/price and flow/book alignments hold AND at least one
available liquidity/disposition comparison is supportive AND no available
comparison contradicts. It never fires on missing context: missing state simply
fails to contribute support, never suppresses an exit.

The exact conditions:
  1. Flow.flow_aligned_midpoint_return defined and > 0.
  2. Flow.flow_book_alignment defined and > 0.
  3. If entry and current Liquidity.depth_ratio:bid are both defined:
     current >= entry (else contradiction).
  4. If entry and current Liquidity.spread_ratio are both defined:
     current <= entry (else contradiction).
  5. If current OrderDisposition bid withdrawal and replenishment are both
     defined: net_withdrawal_fraction:bid <= net_replenishment_fraction:bid
     (else contradiction).

Mandatory alignments gate everything; at least one liquidity/disposition
comparison must be available and supportive; any available contradiction
refutes support.
*/
func continuationSupportive(
	reader PerspectiveReader,
	entry positionEntryContext,
	symbol string,
) bool {
	if reader == nil {
		return false
	}

	flow, flowFound := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindFlow})

	if !flowFound {
		return false
	}

	// 1. Flow/price alignment (mandatory).
	flowAligned, aligned := readingValue(&flow, "flow_aligned_midpoint_return")

	if !aligned || flowAligned <= 0 {
		return false
	}

	// 2. Flow/book alignment (mandatory).
	flowBook, bookAligned := readingValue(&flow, "flow_book_alignment")

	if !bookAligned || flowBook <= 0 {
		return false
	}

	// 3-5. Liquidity/disposition comparisons: every available one must be
	// supportive; at least one must be available.
	supportive := false
	contradicted := false

	liquidity, liquidityFound := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindLiquidity})

	if liquidityFound {
		currentBid, currentBidFound := readingValue(&liquidity, "depth_ratio:bid")
		entryBid, entryBidFound := readingValue(&entry.liquidity, "depth_ratio:bid")

		if currentBidFound && entryBidFound {
			supportive = true

			if currentBid < entryBid {
				contradicted = true
			}
		}

		currentSpread, currentSpreadFound := readingValue(&liquidity, "spread_ratio")
		entrySpread, entrySpreadFound := readingValue(&entry.liquidity, "spread_ratio")

		if currentSpreadFound && entrySpreadFound {
			supportive = true

			if currentSpread > entrySpread {
				contradicted = true
			}
		}
	}

	disposition, dispositionFound := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindOrderDisposition})

	if dispositionFound {
		withdrawal, withdrawalFound := readingValue(&disposition, "net_withdrawal_fraction:bid")
		replenishment, replenishmentFound := readingValue(&disposition, "net_replenishment_fraction:bid")

		if withdrawalFound && replenishmentFound {
			supportive = true

			if withdrawal > replenishment {
				contradicted = true
			}
		}
	}

	return supportive && !contradicted
}
