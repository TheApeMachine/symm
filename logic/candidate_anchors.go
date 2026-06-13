package logic

import (
	"math"
	"sort"
)

/*
CandidateAnchors holds per-spectrum robust scales for strength and novelty normalization.
*/
type CandidateAnchors struct {
	StrengthBySource map[SourceType]float64
	NoveltyMedian    float64
	CostMedian       float64
}

/*
BuildCandidateAnchors derives source-local strength anchors from the measurement window.
*/
func BuildCandidateAnchors(
	measurements []Measurement,
	executionCost ExecutionCost,
) CandidateAnchors {
	strengthsBySource := make(map[SourceType][]float64)
	novelties := make([]float64, 0, len(measurements))

	for _, measurement := range measurements {
		if measurement.Strength <= 0 {
			continue
		}

		if measurement.Source != SourceNone {
			strengthsBySource[measurement.Source] = append(
				strengthsBySource[measurement.Source],
				measurement.Strength,
			)
		}

		novelty := measurement.NoveltySurprise

		if novelty <= 0 {
			novelty = measurement.Surprise
		}

		if novelty > 0 {
			novelties = append(novelties, novelty)
		}
	}

	strengthBySource := make(map[SourceType]float64, len(strengthsBySource))

	for source, strengths := range strengthsBySource {
		strengthBySource[source] = robustScale(strengths)
	}

	return CandidateAnchors{
		StrengthBySource: strengthBySource,
		NoveltyMedian:    medianPositive(novelties),
		CostMedian:       executionCost.TotalBps,
	}
}

func normalizedStrength(
	candidate EntryCandidate,
	anchors CandidateAnchors,
) float64 {
	scale := 0.0

	for _, source := range candidate.Sources {
		sourceScale := anchors.StrengthBySource[source]

		if sourceScale > scale {
			scale = sourceScale
		}
	}

	if scale <= 0 {
		scale = candidate.Strength
	}

	if scale <= 0 {
		return 0
	}

	return candidate.Strength / scale
}

func robustScale(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	median := medianPositive(values)
	deviations := make([]float64, 0, len(values))

	for _, value := range values {
		deviations = append(deviations, math.Abs(value-median))
	}

	mad := medianPositive(deviations)

	return median + mad
}

func medianPositive(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	return sorted[len(sorted)/2]
}

/*
NormalizedStrengthForCandidate exposes source-local strength normalization for sizing.
*/
func NormalizedStrengthForCandidate(
	candidate EntryCandidate,
	anchors CandidateAnchors,
) float64 {
	return normalizedStrength(candidate, anchors)
}
