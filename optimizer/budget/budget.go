package budget

import (
	"math"
	"runtime"

	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
DeriveSearchBudget sizes the optimizer from replay ticks, category breadth,
distinct thresholds on the tape, and available workers.
*/
func DeriveSearchBudget(
	profile *profile.Profile,
	tape replay.ReplayTape,
	workers int,
) types.SearchBudget {
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

	return types.SearchBudget{
		BeamWidth:                  beamWidth,
		CandidateLimit:             candidateLimit,
		MaxThresholds:              distinctThresholds,
		MaxReasoningSteps:          maxReasoningSteps,
		MeasurementSampleCap:       deriveMeasurementSampleCap(rowCount, workers),
		HybridSeedCount:            beamWidth,
		MCTSIterations:             mctsIterations,
		MCTSSeedPriorVisits:        int(math.Ceil(math.Sqrt(float64(beamWidth)))),
		MinChainSupport:            deriveMinChainSupport(tickCount),
		BeamPruneFactor:            2 + int(math.Log2(float64(max(2, beamWidth)))),
		MaxGatesPerSurvivor:        categoryCount,
		MaxWidensPerSurvivor:       categoryCount,
		ReentryTickCooldown:        deriveReentryTickCooldown(tickCount, categoryCount),
		MinRoundTrips:              deriveMinRoundTrips(tickCount),
		ComplexityPenalty:          deriveComplexityPenalty(profile, tape, maxReasoningSteps),
		ExplorationWeight:          math.Sqrt(2),
		MCTSRewardScale:            rewardScale,
		NearMissTickJitter:         deriveNearMissTickJitter(tickCount),
		TheoreticalUCTDiscount:     deriveTheoreticalUCTDiscount(beamWidth),
		AdversarialRolloutInterval: deriveAdversarialRolloutInterval(mctsIterations, beamWidth),
		AdversarialRolloutFraction: deriveAdversarialRolloutFraction(beamWidth),
	}
}

/*
DeriveMeasurementSampleCap limits JSONL loading for large captures without a
fixed row ceiling. Sample size grows with sqrt(file rows) and workers.
*/
func DeriveMeasurementSampleCap(totalRows int, workers int) int {
	return deriveMeasurementSampleCap(totalRows, workers)
}
