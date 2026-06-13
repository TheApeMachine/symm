package logic

import (
	"math"
)

/*
EntryCandidate is a coherent setup used for sizing instead of independent spectrum peaks.
*/
type EntryCandidate struct {
	Symbol     string
	Strategy   string
	Sources    []SourceType
	Categories []CategoryType
	Confidence float64
	EdgeBps    float64
	CostBps    float64
	Strength   float64
	Novelty    float64
	Toxicity   float64
}

/*
BuildEntryCandidates groups decision-eligible measurements into sizing candidates.
Each measurement with positive edge confidence becomes its own candidate; coherent
multi-source clusters are merged when they share entry-oriented high-value categories.
*/
func BuildEntryCandidates(
	measurements []Measurement,
	costBps float64,
) []EntryCandidate {
	candidates := make([]EntryCandidate, 0, len(measurements))
	toxicityPeak := peakToxicityStrength(measurements)

	for _, measurement := range measurements {
		if measurement.Category == CategoryTypeNone || measurement.Strength <= 0 {
			continue
		}

		if !isEntryOrientedSource(measurement.Source) {
			continue
		}

		evidence := EvidenceFromMeasurement(measurement, costBps)
		edgeBps := evidence.ExpectedMoveBps - evidence.CostBps

		candidates = append(candidates, EntryCandidate{
			Symbol:     measurement.Symbol,
			Strategy:   string(measurement.Category),
			Sources:    []SourceType{measurement.Source},
			Categories: []CategoryType{measurement.Category},
			Confidence: evidence.EdgeConfidence,
			EdgeBps:    edgeBps,
			CostBps:    costBps,
			Strength:   evidence.RawStrength,
			Novelty:    evidence.NoveltySurprise,
			Toxicity:   toxicityPeak,
		})
	}

	return mergeCoherentCandidates(candidates)
}

/*
BestEntryCandidate selects the highest scoring coherent candidate for sizing.
*/
func BestEntryCandidate(
	measurements []Measurement,
	costBps float64,
) (EntryCandidate, bool) {
	candidates := BuildEntryCandidates(measurements, costBps)

	if len(candidates) == 0 {
		return EntryCandidate{}, false
	}

	best := candidates[0]
	bestScore := candidateScore(best)

	for _, candidate := range candidates[1:] {
		score := candidateScore(candidate)

		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}

	return best, true
}

func candidateScore(candidate EntryCandidate) float64 {
	costRatio := 1.0

	if candidate.CostBps > 0 {
		costRatio = math.Max(0, candidate.EdgeBps/candidate.CostBps)
	}

	confidence := clampUnit(candidate.Confidence, 0.01, 0.99)
	strengthAnchor := math.Max(candidate.Strength, 1e-9)

	return 0.45*math.Log1p(costRatio) +
		0.30*logit(confidence) +
		0.20*math.Log1p(candidate.Strength/strengthAnchor) +
		0.05*math.Log1p(candidate.Novelty) -
		0.35*candidate.Toxicity
}

func mergeCoherentCandidates(candidates []EntryCandidate) []EntryCandidate {
	if len(candidates) <= 1 {
		return candidates
	}

	merged := make([]EntryCandidate, 0, len(candidates))
	used := make([]bool, len(candidates))

	for index, candidate := range candidates {
		if used[index] {
			continue
		}

		cluster := candidate

		for otherIndex := index + 1; otherIndex < len(candidates); otherIndex++ {
			if used[otherIndex] {
				continue
			}

			other := candidates[otherIndex]

			if !coherentPair(cluster, other) {
				continue
			}

			cluster = mergeCandidates(cluster, other)
			used[otherIndex] = true
		}

		merged = append(merged, cluster)
	}

	return merged
}

func coherentPair(left, right EntryCandidate) bool {
	if left.Symbol != "" && right.Symbol != "" && left.Symbol != right.Symbol {
		return false
	}

	for _, leftCategory := range left.Categories {
		for _, rightCategory := range right.Categories {
			if complementaryEntryCategories(leftCategory, rightCategory) {
				return true
			}
		}
	}

	return false
}

func complementaryEntryCategories(left, right CategoryType) bool {
	pairs := [][2]CategoryType{
		{CategoryInefficientLag, CategoryAggressiveDrive},
		{CategoryVerticalIgnition, CategoryAggressiveDrive},
		{CategoryHiddenAbsorption, CategoryAggressiveDrive},
		{CategoryRiskOnSurge, CategoryOrganicTrend},
		{CategoryFrenzy, CategoryVerticalIgnition},
	}

	for _, pair := range pairs {
		if (left == pair[0] && right == pair[1]) || (left == pair[1] && right == pair[0]) {
			return true
		}
	}

	return false
}

func mergeCandidates(left, right EntryCandidate) EntryCandidate {
	merged := left
	merged.Sources = append(merged.Sources, right.Sources...)
	merged.Categories = append(merged.Categories, right.Categories...)
	merged.Confidence = math.Max(left.Confidence, right.Confidence)
	merged.EdgeBps = math.Max(left.EdgeBps, right.EdgeBps)
	merged.Strength = math.Max(left.Strength, right.Strength)
	merged.Novelty = math.Max(left.Novelty, right.Novelty)
	merged.Toxicity = math.Max(left.Toxicity, right.Toxicity)

	return merged
}

func peakToxicityStrength(measurements []Measurement) float64 {
	peak := 0.0

	for _, measurement := range measurements {
		if measurement.Source != SourceToxicity {
			continue
		}

		if measurement.Strength > peak {
			peak = measurement.Strength
		}
	}

	return peak
}

func isEntryOrientedSource(source SourceType) bool {
	switch source {
	case SourceToxicity, SourceLiquidity:
		return false
	default:
		return source != SourceNone
	}
}

func clampUnit(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}

	if value > upper {
		return upper
	}

	return value
}

func logit(probability float64) float64 {
	return math.Log(probability / (1 - probability))
}

/*
QualifiesForOpportunityEntryFromCandidate gates opportunity slots on a coherent candidate.
*/
func QualifiesForOpportunityEntryFromCandidate(
	candidate EntryCandidate,
	thresholdCtx ThresholdContext,
) bool {
	if candidate.Strength <= 0 {
		return false
	}

	confidenceBar := thresholdCtx.EntryConfidenceBaseline

	if candidate.Confidence < confidenceBar {
		return false
	}

	noveltyBar := noveltyBarForCandidate(candidate, thresholdCtx)

	if candidate.Novelty < noveltyBar {
		return false
	}

	return hasHighValueOpportunityCategoryFromCandidate(candidate)
}

func hasHighValueOpportunityCategoryFromCandidate(candidate EntryCandidate) bool {
	for _, source := range candidate.Sources {
		if source == SourcePumpDump {
			return true
		}
	}

	for _, category := range candidate.Categories {
		switch category {
		case CategoryFrenzy,
			CategoryVerticalIgnition,
			CategoryAggressiveDrive,
			CategoryRiskOnSurge,
			CategoryInefficientLag:
			return true
		default:
		}
	}

	return false
}
