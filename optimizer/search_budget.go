package optimizer

import (
	"math"
	"runtime"
)

/*
SearchBudget holds scan, guard, MCTS, and simulation limits derived from the
measurement tape and profile. Nothing here is user-tunable.
*/
type SearchBudget struct {
	BeamWidth            int
	CandidateLimit       int
	MaxThresholds        int
	MaxReasoningSteps    int
	MeasurementSampleCap int
	HybridSeedCount      int
	MCTSIterations       int
	MCTSSeedPriorVisits  int
	MinChainSupport      int
	BeamPruneFactor      int
	MaxGatesPerSurvivor  int
	MaxWidensPerSurvivor int
	ReentryTickCooldown  int
	MinRoundTrips        int
	ComplexityPenalty    float64
	ExplorationWeight    float64
	MCTSRewardScale      float64
}

func (budget SearchBudget) IsZero() bool {
	return budget.BeamWidth <= 0
}

/*
DeriveSearchBudget sizes the optimizer from replay ticks, category breadth,
distinct thresholds on the tape, and available workers.
*/
func DeriveSearchBudget(
	profile *Profile,
	tape ReplayTape,
	workers int,
) SearchBudget {
	profile.PrepareCache()

	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	tickCount := tape.Len()
	rowCount := profile.Len()
	categoryCount := len(profile.Categories())

	if categoryCount <= 0 {
		categoryCount = 1
	}

	if tickCount <= 0 {
		tickCount = rowCount
	}

	if tickCount <= 0 {
		tickCount = 1
	}

	distinctThresholds := profileDistinctThresholdCount(profile)
	beamWidth := categoryCount * workers
	maxReasoningSteps := deriveMaxReasoningSteps(tape, profile)
	stagnationWindow := beamWidth

	if stagnationWindow <= 0 {
		stagnationWindow = 1
	}

	candidateLimit := beamWidth * stagnationWindow * maxReasoningSteps * 2

	if candidateLimit < beamWidth*workers {
		candidateLimit = beamWidth * workers
	}

	maxThresholds := distinctThresholds
	thresholdCap := beamWidth * workers

	if maxThresholds > thresholdCap {
		maxThresholds = thresholdCap
	}

	if maxThresholds < 1 {
		maxThresholds = 1
	}

	mctsIterations := candidateLimit / beamWidth

	if mctsIterations < workers {
		mctsIterations = workers
	}

	rewardScale := deriveMCTSRewardScale(profile)

	return SearchBudget{
		BeamWidth:            beamWidth,
		CandidateLimit:       candidateLimit,
		MaxThresholds:        distinctThresholds,
		MaxReasoningSteps:    maxReasoningSteps,
		MeasurementSampleCap: deriveMeasurementSampleCap(rowCount, workers),
		HybridSeedCount:      beamWidth,
		MCTSIterations:       mctsIterations,
		MCTSSeedPriorVisits:  int(math.Ceil(math.Sqrt(float64(beamWidth)))),
		MinChainSupport:      deriveMinChainSupport(tickCount),
		BeamPruneFactor:      2 + int(math.Log2(float64(max(2, beamWidth)))),
		MaxGatesPerSurvivor:  categoryCount,
		MaxWidensPerSurvivor: categoryCount,
		ReentryTickCooldown:  deriveReentryTickCooldown(tickCount, categoryCount),
		MinRoundTrips:        1,
		ComplexityPenalty:    0,
		ExplorationWeight:    math.Sqrt(2),
		MCTSRewardScale:      rewardScale,
	}
}

/*
DeriveMeasurementSampleCap limits JSONL loading for large captures without a
fixed row ceiling. Sample size grows with sqrt(file rows) and workers.
*/
func DeriveMeasurementSampleCap(totalRows int, workers int) int {
	return deriveMeasurementSampleCap(totalRows, workers)
}
