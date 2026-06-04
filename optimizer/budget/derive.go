package budget

import (
	"math"
	"runtime"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

var searchUnits = []perspectives.UnitType{
	perspectives.UnitSNR,
	perspectives.UnitConfidence,
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

/*
deriveMaxReasoningSteps bounds reasoning depth from reachable entry chains, deny
wrappers observed on the tape, and log-scaled room for deepen passes.
*/
func deriveMaxReasoningSteps(tape replay.ReplayTape, profile *profile.Profile) int {
	index := cooccurrence.NewCoOccurrenceIndex(tape, 0)
	maxEntryChain := 0

	for _, chain := range decisionEntryChains {
		for _, prefix := range reachableEntryChainPrefixes(index, chain) {
			if len(prefix) > maxEntryChain {
				maxEntryChain = len(prefix)
			}
		}
	}

	denySeen := 0

	for _, category := range decisionDenyCategories {
		if index.CategorySeen(category) {
			denySeen++
		}
	}

	depth := maxEntryChain + denySeen
	extraDeepening := int(math.Ceil(math.Log2(float64(max(2, tape.Len())))))

	depth += extraDeepening

	if depth < 2 {
		depth = 2
	}

	categoryCount := len(profile.Categories())

	if categoryCount > 0 && depth > categoryCount {
		depth = categoryCount
	}

	return depth
}

func DeriveMinChainSupport(tickCount int) int {
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

/*
deriveMinRoundTrips sets the minimum closed trades a candidate needs before it is
eligible as "best". A single lucky round trip is statistical noise — letting it
win (the old hardcoded floor of 1) is the optimizer fooling itself — so the floor
scales gently with the tape. It stays well below DeriveMinChainSupport so real
multi-trade strategies still qualify; if nothing clears the bar, that honestly
means the tape carries no credible edge rather than surfacing a fluke.
*/
func deriveMinRoundTrips(tickCount int) int {
	if tickCount <= 0 {
		return 1
	}

	trips := int(math.Ceil(math.Sqrt(float64(tickCount)) / 3))

	if trips < 20 {
		return 20
	}

	return trips
}

func deriveMinChainSupport(tickCount int) int {
	return DeriveMinChainSupport(tickCount)
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

func deriveMCTSRewardScale(profile *profile.Profile) float64 {
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

func profileDistinctThresholdCount(profile *profile.Profile) int {
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

func DeriveJitterFractions(profile *profile.Profile) []float64 {
	return deriveJitterFractions(profile)
}

func deriveJitterFractions(profile *profile.Profile) []float64 {
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

func DeriveWalkForwardOptions(
	rowCount int,
	options types.WalkForwardOptions,
) types.WalkForwardOptions {
	return deriveWalkForwardOptions(rowCount, options)
}

func deriveWalkForwardOptions(
	rowCount int,
	options types.WalkForwardOptions,
) types.WalkForwardOptions {
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

	windowCount := countWalkForwardWindows(
		rowCount,
		options.TrainFraction,
		options.TestFraction,
		options.StepFraction,
	)

	if windowCount == 0 {
		return options
	}

	if options.MinWinRate <= 0 {
		options.MinWinRate = float64(windowCount-1) / float64(windowCount)
	}

	if options.MaxHoldoutDecay <= 0 {
		options.MaxHoldoutDecay = 1 / sqrtRows
	}

	return options
}

func deriveComplexityPenalty(
	profile *profile.Profile,
	tape replay.ReplayTape,
	maxReasoningSteps int,
) float64 {
	if profile.Len() == 0 || maxReasoningSteps <= 0 {
		return 0
	}

	rewardScale := deriveMCTSRewardScale(profile)

	if rewardScale <= 0 {
		return 0
	}

	sqrtRows := math.Sqrt(float64(max(1, tape.Len())))

	return 1 / (rewardScale * sqrtRows * float64(maxReasoningSteps))
}

func deriveNearMissTickJitter(tickCount int) int {
	if tickCount <= 1 {
		return 1
	}

	jitter := int(math.Ceil(math.Sqrt(float64(tickCount))))

	if jitter < 1 {
		return 1
	}

	return jitter
}

func deriveTheoreticalUCTDiscount(beamWidth int) float64 {
	if beamWidth <= 0 {
		return 0.25
	}

	return 1 / math.Sqrt(float64(beamWidth))
}

func deriveAdversarialRolloutFraction(beamWidth int) float64 {
	if beamWidth <= 0 {
		return 0.1
	}

	return 1 / float64(beamWidth)
}

func deriveAdversarialRolloutInterval(iterations int, beamWidth int) int {
	if iterations <= 0 || beamWidth <= 0 {
		return 0
	}

	interval := iterations / (beamWidth * 2)

	if interval < 1 {
		return 1
	}

	return interval
}

func countWalkForwardWindows(
	rowCount int,
	trainFraction float64,
	testFraction float64,
	stepFraction float64,
) int {
	if rowCount < 4 || trainFraction <= 0 || testFraction <= 0 || stepFraction <= 0 {
		return 0
	}

	trainSize := int(float64(rowCount) * trainFraction)
	testSize := int(float64(rowCount) * testFraction)
	stepSize := int(float64(rowCount) * stepFraction)

	if trainSize < 2 || testSize < 1 || stepSize < 1 {
		return 0
	}

	count := 0

	for trainStart := 0; trainStart+trainSize+testSize <= rowCount; trainStart += stepSize {
		count++
	}

	return count
}
