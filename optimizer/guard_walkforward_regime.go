package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

func (guard *OverfitGuard) evaluateChronologicalWindow(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	tags []StructuralRegime,
	window IndexWindow,
) (win bool, testPerTrade float64) {
	trainRegimes := regimeSetInRange(tags, window.TrainStart, window.TrainEnd)

	if testSliceHasUnprecedentedRegime(
		trainRegimes, tags, window.TestStart, window.TestEnd,
	) {
		return false, 0
	}

	trainRows := rows[window.TrainStart:window.TrainEnd]
	testRows := rows[window.TestStart:window.TestEnd]

	trainResult := NewReplaySimulationWithTape(
		guard.ctx, branches, PrecompileTape(trainRows),
	).Result()
	testResult := NewReplaySimulationWithTape(
		guard.ctx, branches, PrecompileTape(testRows),
	).Result()

	if trainResult.ClosedTrades == 0 || testResult.ClosedTrades == 0 {
		return false, 0
	}

	trainPerTrade := trainResult.ReturnPerTrade()
	testPerTrade = testResult.ReturnPerTrade()

	if testPerTrade <= 0 {
		return false, 0
	}

	decay := holdoutDecay(trainPerTrade, testPerTrade)

	if decay > guard.options.WalkForward.MaxHoldoutDecay {
		return false, 0
	}

	return true, testPerTrade
}

func (guard *OverfitGuard) evaluateRegimeStratifiedWindow(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	tags []StructuralRegime,
	window IndexWindow,
) (win bool, testPerTrade float64) {
	trainRegimes := regimeSetInRange(tags, window.TrainStart, window.TrainEnd)
	bestPerTrade := 0.0
	passed := false

	for regime := range trainRegimes {
		if regime == StructuralRegimeUnclassified {
			continue
		}

		regimeWin, perTrade := guard.evaluateRegimePairWindow(
			branches, rows, tags, window, regime,
		)

		if !regimeWin {
			continue
		}

		passed = true

		if perTrade > bestPerTrade {
			bestPerTrade = perTrade
		}
	}

	return passed, bestPerTrade
}

func (guard *OverfitGuard) evaluateRegimePairWindow(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	tags []StructuralRegime,
	window IndexWindow,
	regime StructuralRegime,
) (win bool, testPerTrade float64) {
	trainSlice := rows[window.TrainStart:window.TrainEnd]
	testSlice := rows[window.TestStart:window.TestEnd]
	trainTags := tags[window.TrainStart:window.TrainEnd]
	testTags := tags[window.TestStart:window.TestEnd]

	trainRows := filterRowsByRegime(trainSlice, trainTags, regime)
	testRows := filterRowsByRegime(testSlice, testTags, regime)
	minRows := regimePairMinRows(regime, len(rows))

	if len(trainRows) < minRows || len(testRows) < 1 {
		return false, 0
	}

	trainResult := NewReplaySimulationWithTape(
		guard.ctx, branches, PrecompileTape(trainRows),
	).Result()
	testResult := NewReplaySimulationWithTape(
		guard.ctx, branches, PrecompileTape(testRows),
	).Result()

	if trainResult.ClosedTrades == 0 || testResult.ClosedTrades == 0 {
		return false, 0
	}

	trainPerTrade := trainResult.ReturnPerTrade()
	testPerTrade = testResult.ReturnPerTrade()

	if testPerTrade <= 0 {
		return false, 0
	}

	decay := holdoutDecay(trainPerTrade, testPerTrade)

	if decay > guard.options.WalkForward.MaxHoldoutDecay {
		return false, 0
	}

	return true, testPerTrade
}
