package optimizer

import (
	"context"
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
GatePathStats holds replay-tape pass rates and win/loss entropy margins for
one reasoning gate.
*/
type GatePathStats struct {
	TapeBefore int
	TapePasses int
	BeforeWins int
	BeforeLoss int
	AfterWins  int
	AfterLoss  int
}

func (stats GatePathStats) TapePassRate() float64 {
	if stats.TapeBefore <= 0 {
		return 0
	}

	return float64(stats.TapePasses) / float64(stats.TapeBefore)
}

func (stats GatePathStats) InformationGainBits() float64 {
	before := binaryEntropyBits(stats.BeforeWins, stats.BeforeLoss)
	after := binaryEntropyBits(stats.AfterWins, stats.AfterLoss)

	return before - after
}

func binaryEntropyBits(wins int, losses int) float64 {
	total := wins + losses

	if total <= 0 {
		return 0
	}

	winRate := float64(wins) / float64(total)

	if winRate <= 0 || winRate >= 1 {
		return 0
	}

	return -(winRate*math.Log2(winRate) + (1-winRate)*math.Log2(1-winRate))
}

type gateStatsKey struct {
	category  perspectives.CategoryType
	unit      perspectives.UnitType
	condition perspectives.ConditionType
	value     float64
	depth     int
}

type gateStatsSlot struct {
	key       gateStatsKey
	ancestors []int
}

type gateStatsCollector struct {
	slots []gateStatsSlot
	stats []GatePathStats
}

func newGateStatsCollector(branches perspectives.BranchList) *gateStatsCollector {
	collector := &gateStatsCollector{
		slots: make([]gateStatsSlot, 0),
		stats: make([]GatePathStats, 0),
	}
	indexByKey := make(map[gateStatsKey]int)

	var walk func(branch perspectives.Branch, depth int, ancestors []int)

	walk = func(branch perspectives.Branch, depth int, ancestors []int) {
		slotIndex := -1

		if branch.ValueSet {
			key := gateStatsKey{
				category:  branch.Category,
				unit:      branch.Unit,
				condition: branch.Condition,
				value:     branch.Value,
				depth:     depth,
			}

			if existing, ok := indexByKey[key]; ok {
				slotIndex = existing
			} else {
				slotIndex = len(collector.slots)
				indexByKey[key] = slotIndex
				collector.slots = append(collector.slots, gateStatsSlot{
					key:       key,
					ancestors: append([]int(nil), ancestors...),
				})
				collector.stats = append(collector.stats, GatePathStats{})
			}
		}

		nextAncestors := ancestors

		if slotIndex >= 0 {
			nextAncestors = append(append([]int(nil), ancestors...), slotIndex)
		}

		for _, child := range branch.Branches {
			walk(child, depth+1, nextAncestors)
		}
	}

	for _, branch := range branches {
		walk(branch, 0, nil)
	}

	return collector
}

func (collector *gateStatsCollector) statsFor(branch perspectives.Branch) GatePathStats {
	if !branch.ValueSet {
		return GatePathStats{}
	}

	key := gateStatsKey{
		category:  branch.Category,
		unit:      branch.Unit,
		condition: branch.Condition,
		value:     branch.Value,
	}

	for index, slot := range collector.slots {
		if slot.key.category == key.category &&
			slot.key.unit == key.unit &&
			slot.key.condition == key.condition &&
			slot.key.value == key.value {
			return collector.stats[index]
		}
	}

	return GatePathStats{}
}

func collectGateReplayStats(
	ctx context.Context,
	tape ReplayTape,
	branches perspectives.BranchList,
) *gateStatsCollector {
	collector := newGateStatsCollector(branches)

	if collector == nil || len(collector.slots) == 0 || tape.Len() == 0 {
		return collector
	}

	canonical := perspectives.CanonicalPlaybookBranches(branches)
	entryGates := entryPathGateIndices(collector, canonical)
	simulation := NewReplaySimulationWithTape(ctx, canonical, tape)
	simulation.collectGateStats(collector, entryGates)

	return collector
}

func entryPathGateIndices(
	collector *gateStatsCollector,
	branches perspectives.BranchList,
) []int {
	entryIndex := perspectives.FindEntryIndex(branches)

	if entryIndex < 0 {
		return nil
	}

	indices := make([]int, 0)
	collectEntryGateIndices(collector, branches[entryIndex], 0, &indices)

	return indices
}

func collectEntryGateIndices(
	collector *gateStatsCollector,
	branch perspectives.Branch,
	depth int,
	indices *[]int,
) {
	if branch.ValueSet {
		key := gateStatsKey{
			category:  branch.Category,
			unit:      branch.Unit,
			condition: branch.Condition,
			value:     branch.Value,
			depth:     depth,
		}

		for index, slot := range collector.slots {
			if slot.key == key {
				*indices = append(*indices, index)

				break
			}
		}
	}

	for _, child := range branch.Branches {
		collectEntryGateIndices(collector, child, depth+1, indices)
	}
}

func (simulation *ReplaySimulation) collectGateStats(
	collector *gateStatsCollector,
	entryGateIndices []int,
) {
	ledger := acquireReplayLedger(simulation.costs)

	defer releaseReplayLedger(ledger)

	ledger.reentryTickCooldown = simulation.tape.ReentryTickCooldown

	if ledger.reentryTickCooldown <= 0 {
		ledger.reentryTickCooldown = 1
	}

	activeEntryGates := make([]int, 0, len(entryGateIndices))
	entryPassing := make([]bool, len(collector.slots))

	for _, tick := range simulation.tape.Ticks {
		if tick.Row.Symbol == "" {
			continue
		}

		branchContext := simulation.branchContext(
			tick.Row,
			tick.Snapshots,
			ledger,
		)
		evaluator := perspectives.NewBranchEvaluator(branchContext)
		passing := gatePassingMask(evaluator, simulation.branches, collector)
		simulation.updateGateTapeStats(collector, passing)

		actionType := evaluator.Action(simulation.branches)

		if evaluator.Err() != nil || actionType == nil {
			ledger.onTick(tick.Row.Symbol)

			continue
		}

		switch *actionType {
		case perspectives.ActionLimit, perspectives.ActionMarket, perspectives.ActionIceberg:
			activeEntryGates = activeEntryGates[:0]
			copy(entryPassing, passing)

			for _, gateIndex := range entryGateIndices {
				if passing[gateIndex] {
					activeEntryGates = append(activeEntryGates, gateIndex)
				}
			}
		case perspectives.ActionSettlePosition,
			perspectives.ActionStopLoss,
			perspectives.ActionStopLossLimit,
			perspectives.ActionTakeProfit,
			perspectives.ActionTakeProfitLimit,
			perspectives.ActionTrailingStop,
			perspectives.ActionTrailingStopLimit:
			if len(activeEntryGates) > 0 && ledger.holding(tick.Row.Symbol) {
				pnl := ledger.previewClosePnL(tick.Row.Symbol, tick.Row.Last, tick.Row.SpreadBPS)
				simulation.recordGateTradeOutcomes(
					collector, activeEntryGates, entryPassing, pnl,
				)
			}
		}

		ledger.apply(*actionType, tick.Row)
		ledger.onTick(tick.Row.Symbol)
	}
}

func gatePassingMask(
	evaluator *perspectives.BranchEvaluator,
	branches perspectives.BranchList,
	collector *gateStatsCollector,
) []bool {
	passing := make([]bool, len(collector.slots))
	fillGatePassingMask(evaluator, branches, collector, passing, 0)

	return passing
}

func fillGatePassingMask(
	evaluator *perspectives.BranchEvaluator,
	branches perspectives.BranchList,
	collector *gateStatsCollector,
	passing []bool,
	depth int,
) {
	for _, branch := range branches {
		if !evaluator.PassesBranch(branch) {
			continue
		}

		if branch.ValueSet {
			key := gateStatsKey{
				category:  branch.Category,
				unit:      branch.Unit,
				condition: branch.Condition,
				value:     branch.Value,
				depth:     depth,
			}

			for index, slot := range collector.slots {
				if slot.key == key {
					passing[index] = true
				}
			}
		}

		fillGatePassingMask(
			evaluator,
			perspectives.BranchList(branch.Branches),
			collector,
			passing,
			depth+1,
		)
	}
}

func (simulation *ReplaySimulation) updateGateTapeStats(
	collector *gateStatsCollector,
	passing []bool,
) {
	for index, slot := range collector.slots {
		if !ancestorsPass(passing, slot.ancestors) {
			continue
		}

		collector.stats[index].TapeBefore++

		if passing[index] {
			collector.stats[index].TapePasses++
		}
	}
}

func (simulation *ReplaySimulation) recordGateTradeOutcomes(
	collector *gateStatsCollector,
	activeEntryGates []int,
	entryPassing []bool,
	pnl float64,
) {
	win := pnl > 0
	active := make(map[int]struct{}, len(activeEntryGates))

	for _, gateIndex := range activeEntryGates {
		active[gateIndex] = struct{}{}
	}

	for index, slot := range collector.slots {
		if !ancestorsPass(entryPassing, slot.ancestors) {
			continue
		}

		if win {
			collector.stats[index].BeforeWins++
		} else {
			collector.stats[index].BeforeLoss++
		}

		if _, ok := active[index]; ok {
			if win {
				collector.stats[index].AfterWins++
			} else {
				collector.stats[index].AfterLoss++
			}
		}
	}
}

func ancestorsPass(passing []bool, ancestors []int) bool {
	for _, ancestorIndex := range ancestors {
		if !passing[ancestorIndex] {
			return false
		}
	}

	return true
}

func informationGainSignificant(
	stats GatePathStats,
	minSamples int,
) bool {
	afterTotal := stats.AfterWins + stats.AfterLoss

	if afterTotal < minSamples {
		return false
	}

	gain := stats.InformationGainBits()

	if gain <= 0 {
		return false
	}

	threshold := math.Sqrt(1 / float64(afterTotal))

	return gain >= threshold
}
