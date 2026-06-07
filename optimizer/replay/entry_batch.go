package replay

import (
	"slices"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

type batchedReplayEntry struct {
	act         reasoning.Act
	measurement types.Measurement
	snapshots   []types.Measurement
	conviction  float64
	at          time.Time
}

func replayEntryBatchWindow() time.Duration {
	if window := viper.GetDuration("trading.entry.batch_window"); window > 0 {
		return window
	}

	if pace := viper.GetDuration("market.subscribe_pace"); pace > 0 {
		return pace
	}

	return 50 * time.Millisecond
}

func replayEntryPreemptionEnabled() bool {
	if viper.IsSet("trading.entry.preemption_enabled") {
		return viper.GetBool("trading.entry.preemption_enabled")
	}

	return true
}

func measurementConviction(measurement types.Measurement) float64 {
	return measurement.SNR * measurement.Confidence
}

func (ledger *replayLedger) queueEntryAction(
	act reasoning.Act,
	measurement types.Measurement,
	snapshots []types.Measurement,
) {
	at := measurement.At

	if at.IsZero() {
		at = time.Now().UTC()
	}

	if len(ledger.entryBatch) == 0 {
		ledger.entryBatchDeadline = at.Add(replayEntryBatchWindow())
	}

	ledger.entryBatch = append(ledger.entryBatch, batchedReplayEntry{
		act:         act,
		measurement: measurement,
		snapshots:   snapshots,
		conviction:  measurementConviction(measurement),
		at:          at,
	})
}

func (ledger *replayLedger) flushEntryBatch(at time.Time) {
	if len(ledger.entryBatch) == 0 {
		return
	}

	if !at.IsZero() && at.Before(ledger.entryBatchDeadline) {
		return
	}

	ranked := append([]batchedReplayEntry(nil), ledger.entryBatch...)
	slices.SortFunc(ranked, func(left, right batchedReplayEntry) int {
		switch {
		case left.conviction > right.conviction:
			return -1
		case left.conviction < right.conviction:
			return 1
		default:
			return 0
		}
	})

	ledger.entryBatch = ledger.entryBatch[:0]
	ledger.entryBatchDeadline = time.Time{}

	for _, item := range ranked {
		ledger.deployReplayEntry(item)
	}
}

func (ledger *replayLedger) deployReplayEntry(item batchedReplayEntry) {
	if ledger.reserveReplayEntry(item) {
		ledger.dispatchReplayEntry(item)
		return
	}

	if !replayEntryPreemptionEnabled() {
		ledger.fundBlocked++

		return
	}

	victim, victimScore, ok := ledger.weakestOpenConviction()

	if !ok || item.conviction <= victimScore {
		ledger.fundBlocked++

		return
	}

	ledger.preemptOpenPosition(victim, item.measurement, item.snapshots)

	if ledger.reserveReplayEntry(item) {
		ledger.dispatchReplayEntry(item)
	}
}

func (ledger *replayLedger) reserveReplayEntry(item batchedReplayEntry) bool {
	fraction, err := entryDeployFraction(ledger.costs, item.act, item.snapshots)

	if err != nil {
		return false
	}

	return ledger.canReserveEntry(fraction, 0)
}

func (ledger *replayLedger) dispatchReplayEntry(item batchedReplayEntry) {
	if ledger.executionLatency <= 0 {
		ledger.applyStressed(item.act, item.measurement, item.snapshots, 0)
		ledger.entryConviction[item.measurement.Symbol] = item.conviction

		return
	}

	ledger.queueActionImmediate(item.act, item.measurement, item.snapshots)
	ledger.entryConviction[item.measurement.Symbol] = item.conviction
}

func (ledger *replayLedger) weakestOpenConviction() (symbol string, score float64, ok bool) {
	for heldSymbol, position := range ledger.positions {
		if position.quantity <= 0 {
			continue
		}

		heldScore := ledger.entryConviction[heldSymbol]

		if !ok || heldScore < score {
			symbol = heldSymbol
			score = heldScore
			ok = true
		}
	}

	return symbol, score, ok
}

func (ledger *replayLedger) preemptOpenPosition(
	symbol string,
	measurement types.Measurement,
	snapshots []types.Measurement,
) {
	feePct := ledger.costs.feePct(reasoning.ActionSettlePosition)
	ledger.closePosition(symbol, measurement, snapshots, feePct)
	delete(ledger.entryConviction, symbol)
}
