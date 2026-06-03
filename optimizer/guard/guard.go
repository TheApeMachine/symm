package guard

import (
	"context"
	"math"
	"runtime"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/budget"
	profilepkg "github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
OverfitGuard scores and filters candidate trees.
*/
type OverfitGuard struct {
	ctx     context.Context
	options types.GuardOptions
	tape    replay.ReplayTape
	profile *profilepkg.Profile
}

func NewOverfitGuard(
	ctx context.Context,
	options types.GuardOptions,
	tape replay.ReplayTape,
	profile *profilepkg.Profile,
) *OverfitGuard {
	options = normalizeGuardOptions(options, profile, tape)

	return &OverfitGuard{
		ctx:     ctx,
		options: options,
		tape:    tape,
		profile: profile,
	}
}

func normalizeGuardOptions(
	options types.GuardOptions,
	profile *profilepkg.Profile,
	tape replay.ReplayTape,
) types.GuardOptions {
	if profile != nil && tape.Len() > 0 {
		searchBudget := budget.DeriveSearchBudget(profile, tape, runtime.NumCPU())

		if options.MaxReasoningSteps <= 0 {
			options.MaxReasoningSteps = searchBudget.MaxReasoningSteps
		}

		if options.MinRoundTrips <= 0 {
			options.MinRoundTrips = searchBudget.MinRoundTrips
		}

		options.ComplexityPenalty = searchBudget.ComplexityPenalty
	}

	if options.JitterEnabled && len(options.JitterFractions) == 0 && profile != nil {
		options.JitterFractions = budget.DeriveJitterFractions(profile)
	}

	if options.WalkForward.Enabled && profile != nil {
		options.WalkForward = budget.DeriveWalkForwardOptions(profile.Len(), options.WalkForward)
	}

	return options
}

/*
AdjustedScore subtracts an adaptive selectivity penalty per reasoning gate so
equally profitable shallow trees beat over-specific deep ones, while waiving
tax on gates that split the tape with genuine discriminatory power.
*/
func (guard *OverfitGuard) AdjustedScore(
	rawScore float64, branches perspectives.BranchList,
) float64 {
	penalty := guard.adaptiveComplexityPenalty(branches)

	return rawScore - penalty
}

/*
AcceptTrainCandidate rejects trees that fail minimum activity or jitter stress.
*/
func (guard *OverfitGuard) AcceptTrainCandidate(
	branches perspectives.BranchList,
) bool {
	result := guard.replayResult(branches).Result()

	if result.ClosedTrades < guard.options.MinRoundTrips {
		return false
	}

	if result.Score <= 0 {
		return false
	}

	if !guard.options.JitterEnabled {
		return true
	}

	if guard.profile == nil {
		return false
	}

	return robustUnderJitter(
		guard.ctx,
		branches,
		guard.tape,
		guard.options.JitterFractions,
		result.Score,
		guard.profile,
	)
}

/*
PersistCandidate is the minimum bar for writing an improved tree to YAML.
*/
func (guard *OverfitGuard) PersistCandidate(
	branches perspectives.BranchList,
) bool {
	result := guard.replayResult(branches).Result()

	if result.ClosedTrades < guard.options.MinRoundTrips {
		return false
	}

	return perspectives.IsCanonicalPlaybook(branches)
}

/*
ImprovesPersistedBest reports whether a scored replay should replace the best
candidate written to YAML. Inert candidates (zero closed round trips) are never
promoted; otherwise the highest realized PnL wins, including negative scores.
*/
func (guard *OverfitGuard) ImprovesPersistedBest(
	adjustedScore float64,
	closedTrades int,
	bestScore float64,
	bestClosedTrades int,
) bool {
	if closedTrades < guard.options.MinRoundTrips {
		return false
	}

	if bestClosedTrades < guard.options.MinRoundTrips {
		return true
	}

	return adjustedScore > bestScore
}

/*
WalkForwardResult summarizes holdout performance across rolling windows.
RegimeWins counts windows that pass regime-stratified holdout testing.
*/
type WalkForwardResult struct {
	Wins         int
	Windows      int
	HoldoutTotal float64
	RegimeWins   int
}

func (result WalkForwardResult) AvgTestPerTrade() float64 {
	if result.Wins <= 0 {
		return 0
	}

	return result.HoldoutTotal / float64(result.Wins)
}

/*
EvaluateWalkForward scores holdout slices without applying a binary pass/fail gate.
*/
func (guard *OverfitGuard) EvaluateWalkForward(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
) WalkForwardResult {
	windows := GenerateIndexWindows(
		len(rows),
		guard.options.WalkForward.TrainFraction,
		guard.options.WalkForward.TestFraction,
		guard.options.WalkForward.StepFraction,
	)

	if len(windows) == 0 {
		return WalkForwardResult{}
	}

	result := WalkForwardResult{Windows: len(windows)}
	regimeTags := budget.TagRowRegimes(rows)

	for _, window := range windows {
		chronoWin, chronoPerTrade := guard.evaluateChronologicalWindow(
			branches, rows, regimeTags, window,
		)

		if chronoWin {
			result.Wins++
			result.HoldoutTotal += chronoPerTrade
		}

		regimeWin, _ := guard.evaluateRegimeStratifiedWindow(
			branches, rows, regimeTags, window,
		)

		if regimeWin {
			result.RegimeWins++
		}
	}

	return result
}

/*
ValidateWalkForward scores holdout slices and returns whether the tree survives.
Decay compares return-per-trade on each window's train slice against its test slice.
*/
func (guard *OverfitGuard) ValidateWalkForward(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
) (bool, float64) {
	result := guard.EvaluateWalkForward(branches, rows)

	if result.Windows == 0 {
		return true, 0
	}

	effectiveWins := result.Wins

	if result.RegimeWins > effectiveWins {
		effectiveWins = result.RegimeWins
	}

	minWins := int(
		math.Ceil(float64(result.Windows) * guard.options.WalkForward.MinWinRate),
	)

	return effectiveWins >= minWins, result.HoldoutTotal
}

func (guard *OverfitGuard) replayResult(
	branches perspectives.BranchList,
) *replay.ReplaySimulation {
	return replay.NewReplaySimulationWithTape(guard.ctx, branches, guard.tape)
}

func (guard *OverfitGuard) evaluateChronologicalWindow(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	tags []budget.StructuralRegime,
	window IndexWindow,
) (win bool, testPerTrade float64) {
	trainRegimes := budget.RegimeSetInRange(tags, window.TrainStart, window.TrainEnd)

	if budget.TestSliceHasUnprecedentedRegime(
		trainRegimes, tags, window.TestStart, window.TestEnd,
	) {
		return false, 0
	}

	trainRows := rows[window.TrainStart:window.TrainEnd]
	testRows := rows[window.TestStart:window.TestEnd]

	trainResult := replay.NewReplaySimulationWithTape(
		guard.ctx, branches, replay.PrecompileTape(trainRows),
	).Result()
	testResult := replay.NewReplaySimulationWithTape(
		guard.ctx, branches, replay.PrecompileTape(testRows),
	).Result()

	if trainResult.ClosedTrades == 0 || testResult.ClosedTrades == 0 {
		return false, 0
	}

	trainPerTrade := trainResult.ReturnPerTrade()
	testPerTrade = testResult.ReturnPerTrade()

	if testPerTrade <= 0 {
		return false, 0
	}

	decay := replay.HoldoutDecay(trainPerTrade, testPerTrade)

	if decay > guard.options.WalkForward.MaxHoldoutDecay {
		return false, 0
	}

	return true, testPerTrade
}

func (guard *OverfitGuard) evaluateRegimeStratifiedWindow(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	tags []budget.StructuralRegime,
	window IndexWindow,
) (win bool, testPerTrade float64) {
	trainRegimes := budget.RegimeSetInRange(tags, window.TrainStart, window.TrainEnd)
	bestPerTrade := 0.0
	passed := false

	for regime := range trainRegimes {
		if regime == budget.StructuralRegimeUnclassified {
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
	tags []budget.StructuralRegime,
	window IndexWindow,
	regime budget.StructuralRegime,
) (win bool, testPerTrade float64) {
	trainSlice := rows[window.TrainStart:window.TrainEnd]
	testSlice := rows[window.TestStart:window.TestEnd]
	trainTags := tags[window.TrainStart:window.TrainEnd]
	testTags := tags[window.TestStart:window.TestEnd]

	trainRows := budget.FilterRowsByRegime(trainSlice, trainTags, regime)
	testRows := budget.FilterRowsByRegime(testSlice, testTags, regime)
	minRows := budget.RegimePairMinRows(regime, len(rows))

	if len(trainRows) < minRows || len(testRows) < 1 {
		return false, 0
	}

	trainResult := replay.NewReplaySimulationWithTape(
		guard.ctx, branches, replay.PrecompileTape(trainRows),
	).Result()
	testResult := replay.NewReplaySimulationWithTape(
		guard.ctx, branches, replay.PrecompileTape(testRows),
	).Result()

	if trainResult.ClosedTrades == 0 || testResult.ClosedTrades == 0 {
		return false, 0
	}

	trainPerTrade := trainResult.ReturnPerTrade()
	testPerTrade = testResult.ReturnPerTrade()

	if testPerTrade <= 0 {
		return false, 0
	}

	decay := replay.HoldoutDecay(trainPerTrade, testPerTrade)

	if decay > guard.options.WalkForward.MaxHoldoutDecay {
		return false, 0
	}

	return true, testPerTrade
}

const (
	extremePassRateHigh = 0.95
	extremePassRateLow  = 0.05
)

/*
adaptiveComplexityPenalty scales each reasoning gate by replay pass rate and
Shannon information gain on win/loss outcomes. Informative gates are waived;
extreme pass rates that fingerprint noise are penalized aggressively.
*/
func (guard *OverfitGuard) adaptiveComplexityPenalty(
	branches perspectives.BranchList,
) float64 {
	base := guard.options.ComplexityPenalty

	if base <= 0 {
		return 0
	}

	minSamples := 1

	if guard.tape.Len() > 0 {
		minSamples = budget.DeriveMinChainSupport(guard.tape.Len())
	}

	var collector *replay.GateStatsCollector

	if guard.tape.Len() > 0 {
		collector = replay.CollectGateReplayStats(guard.ctx, guard.tape, branches)
	}

	return sumGateComplexityPenalty(
		guard.profile,
		collector,
		branches,
		base,
		minSamples,
	)
}

func sumGateComplexityPenalty(
	profile *profilepkg.Profile,
	collector *replay.GateStatsCollector,
	branches perspectives.BranchList,
	basePenalty float64,
	minSamples int,
) float64 {
	total := 0.0

	for _, branch := range branches {
		total += gateComplexityPenalty(profile, collector, branch, basePenalty, minSamples)
	}

	return total
}

func gateComplexityPenalty(
	profile *profilepkg.Profile,
	collector *replay.GateStatsCollector,
	branch perspectives.Branch,
	basePenalty float64,
	minSamples int,
) float64 {
	penalty := 0.0

	if branch.ValueSet {
		weight := gateComplexityWeight(profile, collector, branch, minSamples)

		if weight > 0 {
			penalty += basePenalty * weight
		}
	}

	for _, child := range branch.Branches {
		penalty += gateComplexityPenalty(
			profile, collector, child, basePenalty, minSamples,
		)
	}

	return penalty
}

func gateComplexityWeight(
	profile *profilepkg.Profile,
	collector *replay.GateStatsCollector,
	branch perspectives.Branch,
	minSamples int,
) float64 {
	passRate := profileGatePassRate(profile, branch)
	replayStats := replay.GatePathStats{}

	if collector != nil {
		replayStats = collector.StatsFor(branch)

		if replayStats.TapeBefore > 0 {
			passRate = replayStats.TapePassRate()
		}
	}

	if replay.InformationGainSignificant(replayStats, minSamples) {
		return 0
	}

	selectivity := profilepkg.GateSelectivity(passRate)

	if passRate >= extremePassRateHigh || passRate <= extremePassRateLow {
		extremeWeight := 1 + (1 - selectivity)

		return math.Max(extremeWeight, 1)
	}

	if selectivity <= 0 {
		return 1
	}

	gain := replayStats.InformationGainBits()

	if gain > 0 {
		gainWeight := 1 - math.Min(1, gain)

		return math.Min(1-selectivity, gainWeight)
	}

	return 1 - selectivity
}

func profileGatePassRate(
	profile *profilepkg.Profile,
	branch perspectives.Branch,
) float64 {
	if profile == nil {
		return 0.5
	}

	passes := profile.GatePassCount(
		branch.Category, branch.Unit, branch.Condition, branch.Value,
	)
	categoryTotal := profile.CategoryCount(branch.Category)

	if categoryTotal <= 0 {
		return 0
	}

	return float64(passes) / float64(categoryTotal)
}
