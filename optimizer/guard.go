package optimizer

import (
	"context"
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	DefaultMaxReasoningSteps = 8
	DefaultComplexityPenalty = 0.002
	DefaultMinRoundTrips     = 1
	DefaultJitterFractions   = 3
	DefaultWalkForwardMinWin = 0.7
	DefaultHoldoutDecayLimit = 0.4
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
}

func NewOverfitGuard(
	ctx context.Context,
	options GuardOptions,
	tape ReplayTape,
) *OverfitGuard {
	return &OverfitGuard{
		ctx:     ctx,
		options: normalizeGuardOptions(options),
		tape:    tape,
	}
}

func normalizeGuardOptions(options GuardOptions) GuardOptions {
	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = DefaultMaxReasoningSteps
	}

	if options.ComplexityPenalty <= 0 {
		options.ComplexityPenalty = DefaultComplexityPenalty
	}

	if options.MinRoundTrips <= 0 {
		options.MinRoundTrips = DefaultMinRoundTrips
	}

	if options.JitterEnabled && len(options.JitterFractions) == 0 {
		options.JitterFractions = []float64{-0.05, -0.02, 0.02, 0.05}
	}

	if options.WalkForward.Enabled {
		if options.WalkForward.TrainFraction <= 0 {
			options.WalkForward.TrainFraction = 0.7
		}

		if options.WalkForward.TestFraction <= 0 {
			options.WalkForward.TestFraction = 0.1
		}

		if options.WalkForward.StepFraction <= 0 {
			options.WalkForward.StepFraction = options.WalkForward.TestFraction
		}

		if options.WalkForward.MinWinRate <= 0 {
			options.WalkForward.MinWinRate = DefaultWalkForwardMinWin
		}

		if options.WalkForward.MaxHoldoutDecay <= 0 {
			options.WalkForward.MaxHoldoutDecay = DefaultHoldoutDecayLimit
		}
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

	return robustUnderJitter(
		guard.ctx, branches, guard.tape, guard.options.JitterFractions, result.Score,
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

	if result.Score <= 0 {
		return false
	}

	return perspectives.IsCanonicalPlaybook(branches)
}

/*
ImprovesPersistedBest reports whether a scored replay should replace the tree
written to YAML. Inert candidates (zero closed round trips, 0% return) are never
promoted; once an active playbook is recorded, only a higher adjusted score wins.
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

	if adjustedScore <= 0 {
		return false
	}

	if bestClosedTrades < guard.options.MinRoundTrips || bestScore <= 0 {
		return true
	}

	return adjustedScore > bestScore
}

/*
ValidateWalkForward scores holdout slices and returns whether the tree survives.
Decay compares return-per-trade on each window's train slice against its test slice.
*/
func (guard *OverfitGuard) ValidateWalkForward(
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
) (bool, float64) {
	windows := GenerateIndexWindows(
		len(rows),
		guard.options.WalkForward.TrainFraction,
		guard.options.WalkForward.TestFraction,
		guard.options.WalkForward.StepFraction,
	)

	if len(windows) == 0 {
		return true, 0
	}

	wins := 0
	holdoutTotal := 0.0

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

		wins++
		holdoutTotal += testPerTrade
	}

	minWins := int(math.Ceil(float64(len(windows)) * guard.options.WalkForward.MinWinRate))

	return wins >= minWins, holdoutTotal
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
