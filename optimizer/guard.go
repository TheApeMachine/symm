package optimizer

import (
	"context"
	"math"
	"runtime"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
GuardOptions configures overfit rejection for branch-tree search.

Depth is measured as reasoning steps: nested children are sequential gates;
sibling branches at the same level are parallel filters, not extra depth.
*/
type GuardOptions struct {
	MaxReasoningSteps int
	ComplexityPenalty float64
	MinRoundTrips     int
	JitterEnabled     bool
	JitterFractions   []float64
	WalkForward       WalkForwardOptions
}

/*
WalkForwardOptions configures chronological train/test windows.
*/
type WalkForwardOptions struct {
	Enabled         bool
	TrainFraction   float64
	TestFraction    float64
	StepFraction    float64
	MinWinRate      float64
	MaxHoldoutDecay float64
}

/*
OverfitGuard scores and filters candidate trees.
*/
type OverfitGuard struct {
	ctx     context.Context
	options GuardOptions
	tape    ReplayTape
	profile *Profile
}

func NewOverfitGuard(
	ctx context.Context,
	options GuardOptions,
	tape ReplayTape,
	profile *Profile,
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
	options GuardOptions,
	profile *Profile,
	tape ReplayTape,
) GuardOptions {
	if profile != nil && tape.Len() > 0 {
		budget := DeriveSearchBudget(profile, tape, runtime.NumCPU())

		if options.MaxReasoningSteps <= 0 {
			options.MaxReasoningSteps = budget.MaxReasoningSteps
		}

		if options.MinRoundTrips <= 0 {
			options.MinRoundTrips = budget.MinRoundTrips
		}

		options.ComplexityPenalty = budget.ComplexityPenalty
	}

	if options.JitterEnabled && len(options.JitterFractions) == 0 && profile != nil {
		options.JitterFractions = deriveJitterFractions(profile)
	}

	if options.WalkForward.Enabled && profile != nil {
		options.WalkForward = deriveWalkForwardOptions(profile.Len(), options.WalkForward)
	}

	return options
}

/*
AdjustedScore subtracts a small penalty per reasoning step so equally
profitable shallow trees beat over-specific deep ones.
*/
func (guard *OverfitGuard) AdjustedScore(
	rawScore float64, branches perspectives.BranchList,
) float64 {
	if guard.options.ComplexityPenalty <= 0 {
		return rawScore
	}

	depth := reasoningDepth(branches)

	return rawScore - float64(depth)*guard.options.ComplexityPenalty
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
*/
type WalkForwardResult struct {
	Wins         int
	Windows      int
	HoldoutTotal float64
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

	for _, window := range windows {
		trainRows := rows[window.TrainStart:window.TrainEnd]
		testRows := rows[window.TestStart:window.TestEnd]

		trainResult := NewReplaySimulationWithTape(
			guard.ctx, branches, PrecompileTape(trainRows),
		).Result()
		testResult := NewReplaySimulationWithTape(
			guard.ctx, branches, PrecompileTape(testRows),
		).Result()

		if trainResult.ClosedTrades == 0 || testResult.ClosedTrades == 0 {
			continue
		}

		trainPerTrade := trainResult.ReturnPerTrade()
		testPerTrade := testResult.ReturnPerTrade()

		if testPerTrade <= 0 {
			continue
		}

		decay := holdoutDecay(trainPerTrade, testPerTrade)

		if decay > guard.options.WalkForward.MaxHoldoutDecay {
			continue
		}

		result.Wins++
		result.HoldoutTotal += testPerTrade
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

	minWins := int(
		math.Ceil(float64(result.Windows) * guard.options.WalkForward.MinWinRate),
	)

	return result.Wins >= minWins, result.HoldoutTotal
}

func (guard *OverfitGuard) replayResult(
	branches perspectives.BranchList,
) *ReplaySimulation {
	return NewReplaySimulationWithTape(guard.ctx, branches, guard.tape)
}

/*
reasoningDepth is the longest nested gate chain, not sibling count.
*/
func reasoningDepth(branches perspectives.BranchList) int {
	maxDepth := 0

	for _, branch := range branches {
		depth := 1

		if len(branch.Branches) > 0 {
			childDepth := reasoningDepth(perspectives.BranchList(branch.Branches))
			depth += childDepth
		}

		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

/*
isBranchCompatible rejects contradictory sequential gates.
*/
func isBranchCompatible(
	parent perspectives.Branch, child perspectives.Branch,
) bool {
	if parent.Category == child.Category &&
		parent.Unit == perspectives.UnitSNR &&
		child.Unit == perspectives.UnitConfidence {
		return false
	}

	if parent.Category != child.Category || parent.Regime != child.Regime {
		return true
	}

	if parent.Condition == perspectives.ConditionIsGreaterThan &&
		child.Condition == perspectives.ConditionIsLessThan &&
		parent.Value > child.Value {
		return false
	}

	if parent.Condition == perspectives.ConditionIsGreaterThanOrEqual &&
		child.Condition == perspectives.ConditionIsLessThanOrEqual &&
		parent.Value > child.Value {
		return false
	}

	return true
}
