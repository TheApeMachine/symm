package replay

import (
	"math"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

type pendingMakerEntry struct {
	symbol      string
	side        trading.Side
	limitPrice  float64
	quantity    float64
	queueAhead  float64
	feePct      float64
	slippagePct float64
	at          types.Measurement
}

/*
replayMakerQueueAhead estimates resting queue depth ahead of a post-only order when
the replay tape lacks L2 history. Hostile microstructure and wider spreads imply
deeper queues and lower fill optimism than price reachability alone.
*/
func replayMakerQueueAhead(
	quantity float64,
	spreadBPS float64,
	stressMultiplier float64,
) float64 {
	if quantity <= 0 {
		return 0
	}

	spreadFactor := 1.0

	if spreadBPS > 0 {
		spreadFactor = 1 + spreadBPS/100
	}

	return quantity * spreadFactor * math.Max(1, stressMultiplier)
}

func (ledger *replayLedger) queueMakerEntry(
	symbol string,
	side trading.Side,
	limitPrice, quantity, feePct, slippagePct float64,
	fraction float64,
	measurement types.Measurement,
	snapshots []types.Measurement,
) {
	if !ledger.canReserveEntry(fraction, 0, quoteCurrency(symbol)) {
		ledger.fundBlocked++
		return
	}

	stress := executionStressMultiplier(snapshots)

	ledger.pendingMakers = append(ledger.pendingMakers, pendingMakerEntry{
		symbol:      symbol,
		side:        side,
		limitPrice:  limitPrice,
		quantity:    quantity,
		queueAhead:  replayMakerQueueAhead(quantity, measurement.SpreadBPS, stress),
		feePct:      feePct,
		slippagePct: slippagePct,
		at:          measurement,
	})
}

func (ledger *replayLedger) advanceMakerQueues(row types.Measurement) {
	if len(ledger.pendingMakers) == 0 || row.Symbol == "" || row.Last <= 0 {
		return
	}

	remaining := ledger.pendingMakers[:0]
	depletion := makerQueueDepletion(row)

	for _, pending := range ledger.pendingMakers {
		if pending.symbol != row.Symbol {
			remaining = append(remaining, pending)
			continue
		}

		if !makerPriceReachable(pending.side, pending.limitPrice, row.Last) {
			remaining = append(remaining, pending)
			continue
		}

		pending.queueAhead -= depletion

		if pending.queueAhead > 0 {
			remaining = append(remaining, pending)
			continue
		}

		fillRow := pending.at
		fillRow.Last = pending.limitPrice

		ledger.openEntryReserved(
			pending.symbol,
			pending.side,
			reasoning.Act{Type: reasoning.ActionLimit, Side: pending.side},
			fillRow,
			nil,
			pending.feePct,
			pending.at.At,
			1,
		)
	}

	ledger.pendingMakers = remaining
}

// makerQueueDepletion estimates how much resting maker liquidity is consumed as
// price trades through the queue. It returns 0.01% of last (row.Last * 0.0001):
// a conservative, price-proportional fill increment — larger on expensive pairs,
// smaller on cheap alts. The replay does not model full L2 queue position, so this
// proxy advances maker fills gradually rather than instantaneously. Tune upward for
// more aggressive maker fills in fast markets; downward for thin books.
func makerQueueDepletion(row types.Measurement) float64 {
	if row.Last <= 0 {
		return 0
	}

	return row.Last * 0.0001
}

func makerPriceReachable(side trading.Side, limitPrice, marketPrice float64) bool {
	if limitPrice <= 0 || marketPrice <= 0 {
		return false
	}

	if side == trading.Sell {
		return marketPrice >= limitPrice
	}

	return marketPrice <= limitPrice
}
