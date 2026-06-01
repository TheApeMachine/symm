package optimizer

import (
	"context"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
ReplaySimulation walks a candidate tree across collected replay measurements.
*/
type ReplaySimulation struct {
	ctx      context.Context
	branches perspectives.BranchList
	rows     []perspectives.Measurement
}

func NewReplaySimulation(
	ctx context.Context,
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
) *ReplaySimulation {
	return &ReplaySimulation{
		ctx:      ctx,
		branches: branches.Clone(),
		rows:     rows,
	}
}

/*
Score replays measurements and returns realized plus marked PnL.
*/
func (simulation *ReplaySimulation) Score() float64 {
	if len(simulation.rows) == 0 {
		return 0
	}

	tree, err := perspectives.NewTreeFromBranches(simulation.ctx, simulation.branches)

	if err != nil {
		return 0
	}

	history := make([]perspectives.Measurement, 0, len(simulation.rows))
	ledger := newReplayLedger()

	for _, row := range simulation.rows {
		history = append(history, row)
		tree.ResetWalk()

		actionType := tree.Walk(history, tree.Branches()...)

		if actionType == nil {
			continue
		}

		ledger.apply(*actionType, row)
	}

	return ledger.totalReturn(simulation.lastPrices())
}

func (simulation *ReplaySimulation) lastPrices() map[string]float64 {
	lastPrices := make(map[string]float64)

	for _, row := range simulation.rows {
		if row.Last > 0 {
			lastPrices[row.Symbol] = row.Last
		}
	}

	return lastPrices
}

type replayPosition struct {
	entryPrice float64
	quantity   float64
}

type replayLedger struct {
	positions map[string]replayPosition
	realized  float64
}

func newReplayLedger() *replayLedger {
	return &replayLedger{
		positions: make(map[string]replayPosition),
	}
}

func (ledger *replayLedger) apply(
	actionType perspectives.ActionType, measurement perspectives.Measurement,
) {
	if measurement.Last <= 0 {
		return
	}

	switch actionType {
	case perspectives.ActionLimit, perspectives.ActionMarket, perspectives.ActionIceberg:
		ledger.openLong(measurement.Symbol, measurement.Last)
	case perspectives.ActionSettlePosition,
		perspectives.ActionStopLoss,
		perspectives.ActionStopLossLimit,
		perspectives.ActionTakeProfit,
		perspectives.ActionTakeProfitLimit,
		perspectives.ActionTrailingStop,
		perspectives.ActionTrailingStopLimit:
		ledger.closeLong(measurement.Symbol, measurement.Last)
	case perspectives.ActionNone:
		return
	}
}

func (ledger *replayLedger) openLong(symbol string, price float64) {
	if _, open := ledger.positions[symbol]; open {
		return
	}

	ledger.positions[symbol] = replayPosition{
		entryPrice: price,
		quantity:   1,
	}
}

func (ledger *replayLedger) closeLong(symbol string, price float64) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	ledger.realized += (price - position.entryPrice) / position.entryPrice
	delete(ledger.positions, symbol)
}

func (ledger *replayLedger) totalReturn(lastPrices map[string]float64) float64 {
	total := ledger.realized

	for symbol, position := range ledger.positions {
		lastPrice, ok := lastPrices[symbol]

		if !ok || lastPrice <= 0 || position.entryPrice <= 0 {
			continue
		}

		total += (lastPrice - position.entryPrice) / position.entryPrice
	}

	return total
}
