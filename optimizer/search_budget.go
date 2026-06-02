package optimizer

import (
	"math"
	"runtime"

	"github.com/theapemachine/symm/market/perspectives"
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
	candidateLimit := tickCount * categoryCount * workers

	if candidateLimit < beamWidth {
		candidateLimit = beamWidth * workers
	}

	maxReasoningSteps := categoryCount

	if maxReasoningSteps > tickCount {
		maxReasoningSteps = tickCount
	}

	if maxReasoningSteps < 1 {
		maxReasoningSteps = 1
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
		MinChainSupport:        deriveMinChainSupport(tickCount),
		BeamPruneFactor:        2 + int(math.Log2(float64(max(2, beamWidth)))),
		MaxGatesPerSurvivor:    categoryCount,
		MaxWidensPerSurvivor:   categoryCount,
		ReentryTickCooldown:    deriveReentryTickCooldown(tickCount, categoryCount),
		MinRoundTrips:          1,
		ComplexityPenalty:      0,
		ExplorationWeight:      math.Sqrt(2),
		MCTSRewardScale:        rewardScale,
	}
}

/*
DeriveMeasurementSampleCap limits JSONL loading for large captures without a
fixed row ceiling. Sample size grows with sqrt(file rows) and workers.
*/
func DeriveMeasurementSampleCap(totalRows int, workers int) int {
	return deriveMeasurementSampleCap(totalRows, workers)
}

func deriveMeasurementSampleCap(totalRows int, workers int) int {
	if totalRows <= 0 {
		return 0
	}

	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	if totalRows <= workers*workers {
		return totalRows
	}

	sample := int(math.Ceil(math.Sqrt(float64(totalRows)) * float64(workers)))

	if sample > totalRows {
		return totalRows
	}

	return sample
}

func deriveMinChainSupport(tickCount int) int {
	if tickCount <= 1 {
		return 1
	}

	support := int(math.Ceil(math.Sqrt(float64(tickCount))))

	if support < 2 {
		return 2
	}

	if support > tickCount {
		return tickCount
	}

	return support
}

func deriveReentryTickCooldown(tickCount int, categoryCount int) int {
	if tickCount <= 1 {
		return 1
	}

	if categoryCount <= 0 {
		categoryCount = 1
	}

	cooldown := tickCount / (categoryCount * categoryCount)

	if cooldown < 1 {
		return 1
	}

	return cooldown
}

func deriveMCTSRewardScale(profile *Profile) float64 {
	spread := 0.0
	categories := profile.Categories()

	for _, category := range categories {
		center := profile.Quantile(category, perspectives.UnitSNR, 0.5)
		spread += profile.JitterScale(category, perspectives.UnitSNR, center)
	}

	if spread <= 0 {
		return 1
	}

	return 1 / spread
}

func profileDistinctThresholdCount(profile *Profile) int {
	maxCount := 0

	for _, category := range profile.Categories() {
		for _, unit := range searchUnits {
			count := len(profile.Values(category, unit, 0))

			if count > maxCount {
				maxCount = count
			}
		}
	}

	if maxCount < 1 {
		return 1
	}

	return maxCount
}

func applyBudgetToTuneOptions(options *TuneOptions, budget SearchBudget) {
	if options.Workers <= 0 {
		options.Workers = runtime.NumCPU()
	}

	if options.MaxMeasurements <= 0 {
		options.MaxMeasurements = budget.MeasurementSampleCap
	}

	if options.MaxThresholds <= 0 {
		options.MaxThresholds = budget.MaxThresholds
	}

	if options.BeamWidth <= 0 {
		options.BeamWidth = budget.BeamWidth
	}

	if options.CandidateLimit <= 0 {
		options.CandidateLimit = budget.CandidateLimit
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.HybridSeedCount <= 0 {
		options.HybridSeedCount = budget.HybridSeedCount
	}

	if options.MCTSIterations <= 0 {
		options.MCTSIterations = budget.MCTSIterations
	}

	if options.Guard.MaxReasoningSteps <= 0 {
		options.Guard.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.Guard.MinRoundTrips <= 0 {
		options.Guard.MinRoundTrips = budget.MinRoundTrips
	}

	options.Guard.ComplexityPenalty = budget.ComplexityPenalty
}

func applyBudgetToScanOptions(options *ScanOptions, budget SearchBudget) ScanOptions {
	if options.Workers <= 0 {
		options.Workers = runtime.NumCPU()
	}

	if options.MaxThresholds <= 0 {
		options.MaxThresholds = budget.MaxThresholds
	}

	if options.BeamWidth <= 0 {
		options.BeamWidth = budget.BeamWidth
	}

	if options.CandidateLimit <= 0 {
		options.CandidateLimit = budget.CandidateLimit
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.Guard.MaxReasoningSteps <= 0 {
		options.Guard.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.Guard.MinRoundTrips <= 0 {
		options.Guard.MinRoundTrips = budget.MinRoundTrips
	}

	options.Guard.ComplexityPenalty = budget.ComplexityPenalty
	options.Budget = budget

	return options
}

func deriveJitterFractions(profile *Profile) []float64 {
	fractions := make([]float64, 0, 4)

	for _, category := range profile.Categories() {
		center := profile.Quantile(category, perspectives.UnitSNR, 0.5)
		scale := profile.JitterScale(category, perspectives.UnitSNR, center)

	if scale <= 0 || center == 0 {
			continue
		}

		fractions = append(
			fractions,
			-scale/math.Abs(center),
			-scale/(math.Abs(center)*2),
			scale/(math.Abs(center)*2),
			scale/math.Abs(center),
		)

		break
	}

	return fractions
}

func deriveWalkForwardOptions(
	rowCount int,
	options WalkForwardOptions,
) WalkForwardOptions {
	if rowCount < 4 {
		return options
	}

	sqrtRows := math.Sqrt(float64(rowCount))

	if options.TestFraction <= 0 {
		options.TestFraction = 1 / sqrtRows
	}

	if options.TrainFraction <= 0 {
		options.TrainFraction = 1 - (2 * options.TestFraction)
	}

	if options.StepFraction <= 0 {
		options.StepFraction = options.TestFraction
	}

	windows := GenerateIndexWindows(
		rowCount,
		options.TrainFraction,
		options.TestFraction,
		options.StepFraction,
	)

	if len(windows) == 0 {
		return options
	}

	if options.MinWinRate <= 0 {
		options.MinWinRate = float64(len(windows)-1) / float64(len(windows))
	}

	if options.MaxHoldoutDecay <= 0 {
		options.MaxHoldoutDecay = 1 / sqrtRows
	}

	return options
}

func applyBudgetToMCTSOptions(options MCTSOptions, budget SearchBudget) MCTSOptions {
	if options.Iterations <= 0 {
		options.Iterations = budget.MCTSIterations
	}

	if options.SeedPriorVisits <= 0 {
		options.SeedPriorVisits = budget.MCTSSeedPriorVisits
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.MaxThresholds <= 0 {
		options.MaxThresholds = budget.MaxThresholds
	}

	return options
}
