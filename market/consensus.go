package market

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

/*
consensusEntry derives a paper entry from the current live signal spectrum when
the configured playbook has no action.
*/
type consensusEntry struct {
	confidenceFloor float64
	surpriseFloor   float64
}

type consensusSummary struct {
	positiveSources   int
	riskSources       int
	classifiedSources int
	positiveScore     float64
	riskScore         float64
}

func newConsensusEntry(
	measurements []logic.Measurement,
	thresholdConfig config.ThresholdConfig,
) *consensusEntry {
	confidenceFloor := medianConfidence(measurements)

	if thresholdConfig.EntryConfidenceBaseline > confidenceFloor {
		confidenceFloor = thresholdConfig.EntryConfidenceBaseline
	}

	return &consensusEntry{
		confidenceFloor: confidenceFloor,
		surpriseFloor:   thresholdConfig.EntrySurpriseBaseline,
	}
}

func (consensusEntry *consensusEntry) Action(
	measurements []logic.Measurement,
	holdings *logic.Holdings,
) (*logic.Action, error) {
	if len(measurements) == 0 {
		return nil, nil
	}

	symbol, err := logic.SymbolFromMeasurements(measurements)

	if err != nil {
		return nil, err
	}

	if holdings != nil && holdings.IsHolding(symbol) {
		return nil, nil
	}

	summary := consensusEntry.summarize(measurements)

	if !summary.enters() {
		return nil, nil
	}

	return &logic.Action{
		Type:   logic.ActionMarket,
		Side:   trading.Buy,
		Symbol: symbol,
	}, nil
}

func (consensusEntry *consensusEntry) summarize(
	measurements []logic.Measurement,
) consensusSummary {
	summary := consensusSummary{}
	macroBalance, macroReady := consensusEntry.macroBalance(measurements)
	var buckets [consensusBucketCount]consensusBucketScore

	for _, measurement := range measurements {
		if !consensusEntry.accepts(measurement) {
			continue
		}

		if consensusTierForSource(measurement.Source) == consensusTierMacro {
			continue
		}

		bucket := consensusBucketForSource(measurement.Source)
		if bucket <= consensusBucketNone || bucket >= consensusBucketCount {
			continue
		}

		direction := directionForCategory(measurement.Category)
		score := adjustedConsensusScore(
			measurement.Confidence,
			direction,
			macroBalance,
			macroReady,
		)

		switch directionForCategory(measurement.Category) {
		case directionPositive:
			if score > buckets[bucket].positiveScore {
				buckets[bucket].positiveScore = score
			}
		case directionRisk:
			if score > buckets[bucket].riskScore {
				buckets[bucket].riskScore = score
			}
		}
	}

	summary.observeBuckets(buckets)

	return summary
}

func (consensusEntry *consensusEntry) macroBalance(
	measurements []logic.Measurement,
) (float64, bool) {
	positiveScore := 0.0
	riskScore := 0.0

	for _, measurement := range measurements {
		if !consensusEntry.accepts(measurement) {
			continue
		}

		if consensusTierForSource(measurement.Source) != consensusTierMacro {
			continue
		}

		switch directionForCategory(measurement.Category) {
		case directionPositive:
			if measurement.Confidence > positiveScore {
				positiveScore = measurement.Confidence
			}
		case directionRisk:
			if measurement.Confidence > riskScore {
				riskScore = measurement.Confidence
			}
		}
	}

	total := positiveScore + riskScore

	if total <= 0 {
		return 0, false
	}

	return (positiveScore - riskScore) / total, true
}

func (consensusEntry *consensusEntry) accepts(measurement logic.Measurement) bool {
	if measurement.BestEffort {
		return false
	}

	if measurement.Category == logic.CategoryTypeNone {
		return false
	}

	if consensusEntry.surpriseFloor > 0 && measurement.Surprise < consensusEntry.surpriseFloor {
		return false
	}

	if consensusEntry.confidenceFloor <= 0 {
		return measurement.Confidence > 0
	}

	return measurement.Confidence >= consensusEntry.confidenceFloor
}

func (summary consensusSummary) enters() bool {
	if summary.classifiedSources == 0 {
		return false
	}

	if summary.positiveSources == 0 {
		return false
	}

	return summary.positiveScore > summary.riskScore
}

func (summary *consensusSummary) observeBuckets(
	buckets [consensusBucketCount]consensusBucketScore,
) {
	for _, bucket := range buckets {
		if bucket.positiveScore <= 0 && bucket.riskScore <= 0 {
			continue
		}

		switch {
		case bucket.positiveScore > bucket.riskScore:
			summary.positiveSources++
			summary.classifiedSources++
			summary.positiveScore += bucket.positiveScore - bucket.riskScore
		case bucket.riskScore > bucket.positiveScore:
			summary.riskSources++
			summary.classifiedSources++
			summary.riskScore += bucket.riskScore - bucket.positiveScore
		}
	}
}

type consensusTier uint8

const (
	consensusTierMicro consensusTier = iota
	consensusTierMacro
)

type consensusBucket uint8

const (
	consensusBucketNone consensusBucket = iota
	consensusBucketFlow
	consensusBucketImpulse
	consensusBucketTiming
	consensusBucketRisk
	consensusBucketPrediction
	consensusBucketCount
)

type consensusBucketScore struct {
	positiveScore float64
	riskScore     float64
}

func consensusTierForSource(source logic.SourceType) consensusTier {
	switch source {
	case logic.SourceCausal,
		logic.SourceCorrelation,
		logic.SourceLiquidity,
		logic.SourceManifold,
		logic.SourceSentiment:
		return consensusTierMacro
	default:
		return consensusTierMicro
	}
}

func consensusBucketForSource(source logic.SourceType) consensusBucket {
	switch source {
	case logic.SourceCVD,
		logic.SourceDepthFlow,
		logic.SourceFluid,
		logic.SourceHawkes:
		return consensusBucketFlow
	case logic.SourcePumpDump:
		return consensusBucketImpulse
	case logic.SourceLeadLag:
		return consensusBucketTiming
	case logic.SourceExhaustion,
		logic.SourceToxicity:
		return consensusBucketRisk
	case logic.SourcePrediction:
		return consensusBucketPrediction
	default:
		return consensusBucketNone
	}
}

func adjustedConsensusScore(
	score float64,
	direction categoryDirection,
	macroBalance float64,
	macroReady bool,
) float64 {
	if !macroReady || score <= 0 {
		return score
	}

	support := math.Max(0, macroBalance)
	oppose := math.Max(0, -macroBalance)

	if direction == directionRisk {
		support, oppose = oppose, support
	}

	return score * (1 + support) / (1 + oppose)
}

type categoryDirection uint8

const (
	directionNeutral categoryDirection = iota
	directionPositive
	directionRisk
)

func directionForCategory(category logic.CategoryType) categoryDirection {
	switch category {
	case logic.CategoryLaminar,
		logic.CategoryInertial,
		logic.CategoryFrenzy,
		logic.CategoryOrganic,
		logic.CategoryHiddenAbsorption,
		logic.CategoryAggressiveDrive,
		logic.CategoryLoadedImbalance,
		logic.CategoryInefficientLag,
		logic.CategorySynchronizedDrift,
		logic.CategoryDecoupledMove,
		logic.CategoryVerticalIgnition,
		logic.CategoryCoiledCompression,
		logic.CategoryOrganicTrend,
		logic.CategoryExtremeScarcity,
		logic.CategoryRiskOnSurge,
		logic.CategoryDivergentMove,
		logic.CategoryHardSupport,
		logic.CategorySystemicHerd,
		logic.CategoryDecoupledAlpha,
		logic.CategoryEndogenousAlpha,
		logic.CategoryFragileExpansion:
		return directionPositive
	case logic.CategoryTurbulent,
		logic.CategorySaturation,
		logic.CategoryExhaustion,
		logic.CategoryVolumeStarvation,
		logic.CategorySpoofTrap,
		logic.CategoryBookThinning,
		logic.CategoryAnchorStall,
		logic.CategoryFadedExhaustion,
		logic.CategorySystemicSlump,
		logic.CategoryLiquidityVacuum,
		logic.CategoryToxicBluff,
		logic.CategoryStochasticNoise,
		logic.CategoryDivergentStress,
		logic.CategorySystemicBeta,
		logic.CategoryLiquidityShock,
		logic.CategoryCausalNoise,
		logic.CategoryMechanicalCollapse,
		logic.CategoryThermalExhaustion,
		logic.CategoryActiveReversal:
		return directionRisk
	default:
		return directionNeutral
	}
}

func medianConfidence(measurements []logic.Measurement) float64 {
	var stack [logic.SourceCount]float64
	values := stack[:0]

	if len(measurements) > len(stack) {
		values = make([]float64, 0, len(measurements))
	}

	for _, measurement := range measurements {
		if math.IsNaN(measurement.Confidence) || math.IsInf(measurement.Confidence, 0) {
			continue
		}

		values = append(values, measurement.Confidence)
	}

	if len(values) == 0 {
		return 0
	}

	sort.Float64s(values)
	middle := len(values) / 2

	if len(values)%2 == 1 {
		return values[middle]
	}

	return (values[middle-1] + values[middle]) / 2
}
