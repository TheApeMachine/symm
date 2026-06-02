package optimizer

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

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
		minSamples = deriveMinChainSupport(guard.tape.Len())
	}

	var collector *gateStatsCollector

	if guard.tape.Len() > 0 {
		collector = collectGateReplayStats(guard.ctx, guard.tape, branches)
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
	profile *Profile,
	collector *gateStatsCollector,
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
	profile *Profile,
	collector *gateStatsCollector,
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
	profile *Profile,
	collector *gateStatsCollector,
	branch perspectives.Branch,
	minSamples int,
) float64 {
	passRate := profileGatePassRate(profile, branch)
	replayStats := GatePathStats{}

	if collector != nil {
		replayStats = collector.statsFor(branch)

		if replayStats.TapeBefore > 0 {
			passRate = replayStats.TapePassRate()
		}
	}

	if informationGainSignificant(replayStats, minSamples) {
		return 0
	}

	selectivity := gateSelectivity(passRate)

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
	profile *Profile,
	branch perspectives.Branch,
) float64 {
	if profile == nil {
		return 0.5
	}

	passes := profile.GatePassCount(
		branch.Category, branch.Unit, branch.Condition, branch.Value,
	)
	categoryTotal := profile.categoryCount(branch.Category)

	if categoryTotal <= 0 {
		return 0
	}

	return float64(passes) / float64(categoryTotal)
}
