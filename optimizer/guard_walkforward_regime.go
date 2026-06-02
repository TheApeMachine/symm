package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

func (guard *OverfitGuard) evaluateChronologicalWindow(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	window IndexWindow,
) (win bool, testPerTrade float64) {
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

	if testSliceHasUnprecedentedRegime(
		trainRegimes, tags, window.TestStart, window.TestEnd,
	) {
		return false, 0
	}

	dominantTrain := dominantRegimeInRange(tags, window.TrainStart, window.TrainEnd)

	if dominantTrain == StructuralRegimeUnclassified {
		return false, 0
	}

	trainRows := filterRowsByRegime(
		rows[window.TrainStart:window.TrainEnd],
		tags[window.TrainStart:window.TrainEnd],
		dominantTrain,
	)
	testRows := filterRowsByRegime(
		rows[window.TestStart:window.TestEnd],
		tags[window.TestStart:window.TestEnd],
		dominantTrain,
	)

	if len(trainRows) < 2 || len(testRows) < 1 {
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

func dominantRegimeInRange(
	tags []StructuralRegime,
	start int,
	end int,
) StructuralRegime {
	counts := make(map[StructuralRegime]int)

	for index := start; index < end && index < len(tags); index++ {
		regime := tags[index]

		if regime == StructuralRegimeUnclassified {
			continue
		}

		counts[regime]++
	}

	bestRegime := StructuralRegimeUnclassified
	bestCount := 0

	for regime, count := range counts {
		if count > bestCount {
			bestCount = count
			bestRegime = regime
		}
	}

	return bestRegime
}
