package broker

import (
	"strings"
	"time"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
reading returns the defined value AND observed instant of a Perspective reading
matched by its metric slot's trailing "/<metric>/sample" suffix. Definability
and observation time are both reported so a caller can enforce causal freshness:
an undefined reading has no value, and a zero/absent ObservedAt carries no
usable observation instant.

It is the structural read the continuation context needs from a Perspective
without importing the advisor package: the reading identity is the interned slot
name the advisor publishes, whose canonical suffix is "/<metric>/sample".
*/
func reading(perspective *types.Perspective, metric string) (float64, time.Time, bool) {
	if perspective == nil {
		return 0, time.Time{}, false
	}

	for index := 0; index < perspective.Count; index++ {
		reading := perspective.Readings[index]

		name, found := nmtypes.SymbolName(reading.Metric)

		if !found {
			continue
		}

		if strings.HasSuffix(name, "/"+metric+"/sample") {
			if !reading.Defined {
				return 0, time.Time{}, false
			}

			return reading.Value, reading.ObservedAt, true
		}
	}

	return 0, time.Time{}, false
}

/*
fresh reports whether an observed reading is causally current for the given
observation instant and the position's activity-clock forecast horizon. Advice
older than the market horizon that justified the position is no longer advice
about the same moment and cannot defer an exit.
*/
func fresh(observedAt, observationTime time.Time, horizon time.Duration) bool {
	if observedAt.IsZero() || horizon <= 0 || observedAt.After(observationTime) {
		return false
	}

	return observationTime.Sub(observedAt) <= horizon
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
	observationTime time.Time,
	horizon time.Duration,
) bool {
	if reader == nil {
		return false
	}

	flow, flowFound := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindFlow})

	if !flowFound {
		return false
	}

	// 1. Flow/price alignment (mandatory): defined value, causally current.
	flowAligned, flowAlignedAt, aligned := reading(&flow, "flow_aligned_midpoint_return")

	if !aligned || flowAligned <= 0 || !fresh(flowAlignedAt, observationTime, horizon) {
		return false
	}

	// 2. Flow/book alignment (mandatory).
	flowBook, flowBookAt, bookAligned := reading(&flow, "flow_book_alignment")

	if !bookAligned || flowBook <= 0 || !fresh(flowBookAt, observationTime, horizon) {
		return false
	}

	// 3-5. Liquidity/disposition comparisons: every available one must be
	// supportive and causally current; at least one must be available; any
	// available contradiction refutes support.
	supportive := false
	contradicted := false

	liquidity, liquidityFound := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindLiquidity})

	if liquidityFound {
		currentBid, currentBidAt, currentBidFound := reading(&liquidity, "depth_ratio:bid")
		entryBid, _, entryBidFound := reading(&entry.liquidity, "depth_ratio:bid")

		if currentBidFound && entryBidFound && fresh(currentBidAt, observationTime, horizon) {
			supportive = true

			if currentBid < entryBid {
				contradicted = true
			}
		}

		currentSpread, currentSpreadAt, currentSpreadFound := reading(&liquidity, "spread_ratio")
		entrySpread, _, entrySpreadFound := reading(&entry.liquidity, "spread_ratio")

		if currentSpreadFound && entrySpreadFound && fresh(currentSpreadAt, observationTime, horizon) {
			supportive = true

			if currentSpread > entrySpread {
				contradicted = true
			}
		}
	}

	disposition, dispositionFound := reader.Latest(types.PerspectiveKey{Symbol: symbol, Kind: types.KindOrderDisposition})

	if dispositionFound {
		withdrawal, withdrawalAt, withdrawalFound := reading(&disposition, "net_withdrawal_fraction:bid")
		replenishment, replenishmentAt, replenishmentFound := reading(&disposition, "net_replenishment_fraction:bid")

		if withdrawalFound && replenishmentFound &&
			fresh(withdrawalAt, observationTime, horizon) &&
			fresh(replenishmentAt, observationTime, horizon) {
			supportive = true

			if withdrawal > replenishment {
				contradicted = true
			}
		}
	}

	return supportive && !contradicted
}
