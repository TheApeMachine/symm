package logic

import "math"

/*
EntryCandidate is a coherent setup used for sizing instead of independent spectrum peaks.
*/
type EntryCandidate struct {
	Symbol            string
	Strategy          string
	Sources           []SourceType
	Categories        []CategoryType
	ExpectedDirection PositionType
	Confidence        float64
	EdgeBps           float64
	CostBps           float64
	Strength          float64
	Novelty           float64
	Toxicity          float64
}

/*
EntryCandidateLong reports whether a candidate is a long-only spot entry with positive edge.
*/
func EntryCandidateLong(candidate EntryCandidate) bool {
	return candidate.ExpectedDirection == PositionTypeLong && candidate.EdgeBps > 0
}

/*
BuildEntryCandidates groups decision-eligible measurements into sizing candidates.
Only measurements with positive calibrated edge enter candidate scoring.
*/
func BuildEntryCandidates(
	measurements []Measurement,
	executionCost ExecutionCost,
) []EntryCandidate {
	candidates := make([]EntryCandidate, 0, len(measurements))
	toxicityPeak := peakToxicityStrength(measurements)

	for _, measurement := range measurements {
		if measurement.Category == CategoryTypeNone || measurement.Strength <= 0 {
			continue
		}

		if measurement.DecisionGrade != DecisionGradeExecutable {
			continue
		}

		if !isEntryOrientedSource(measurement.Source) {
			continue
		}

		evidence := EvidenceFromMeasurement(measurement, executionCost.TotalBps)
		edgeBps := evidence.ExpectedMoveBps - evidence.CostBps

		if edgeBps <= 0 {
			continue
		}

		if evidence.EdgeConfidence <= 0 {
			continue
		}

		candidates = append(candidates, EntryCandidate{
			Symbol:            measurement.Symbol,
			Strategy:          string(measurement.Category),
			Sources:           []SourceType{measurement.Source},
			Categories:        []CategoryType{measurement.Category},
			ExpectedDirection: measurement.Position,
			Confidence:        evidence.EdgeConfidence,
			EdgeBps:           edgeBps,
			CostBps:           executionCost.TotalBps,
			Strength:          evidence.RawStrength,
			Novelty:           evidence.NoveltySurprise,
			Toxicity:          toxicityPeak,
		})
	}

	return mergeCoherentCandidates(candidates)
}

/*
BestEntryCandidate selects the highest scoring coherent candidate for sizing.
*/
func BestEntryCandidate(
	measurements []Measurement,
	executionCost ExecutionCost,
) (EntryCandidate, bool) {
	candidates := BuildEntryCandidates(measurements, executionCost)

	if len(candidates) == 0 {
		return EntryCandidate{}, false
	}

	anchors := BuildCandidateAnchors(measurements, executionCost)
	best := candidates[0]
	bestScore := candidateScore(best, anchors)

	for _, candidate := range candidates[1:] {
		score := candidateScore(candidate, anchors)

		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}

	return best, true
}

func candidateScore(candidate EntryCandidate, anchors CandidateAnchors) float64 {
	costRatio := 1.0

	if candidate.CostBps > 0 {
		costRatio = math.Max(0, candidate.EdgeBps/candidate.CostBps)
	}

	confidence := clampUnit(candidate.Confidence, 0.01, 0.99)
	strengthWeight := math.Log1p(normalizedStrength(candidate, anchors))

	return 0.45*math.Log1p(costRatio) +
		0.30*logit(confidence) +
		0.20*strengthWeight +
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

	confidences := []float64{left.Confidence, right.Confidence}
	edges := []float64{left.EdgeBps, right.EdgeBps}
	strengths := []float64{left.Strength, right.Strength}
	weights := confidences

	merged.Confidence = correlatedConfidence(
		confidences,
		correlationPenaltyForSources(merged.Sources),
	)
	merged.EdgeBps = weightedMeanPositiveEdge(edges, weights)
	merged.Strength = robustMean(strengths)
	merged.Novelty = math.Max(left.Novelty, right.Novelty)
	merged.Toxicity = math.Max(left.Toxicity, right.Toxicity)
	merged.ExpectedDirection = preferredLongDirection(left.ExpectedDirection, right.ExpectedDirection)

	return merged
}

func preferredLongDirection(left, right PositionType) PositionType {
	if left == PositionTypeLong || right == PositionTypeLong {
		return PositionTypeLong
	}

	if left == PositionTypeShort {
		return left
	}

	return right
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

/*
QualifiesForOpportunityEntryFromCandidate gates opportunity slots on positive edge first.
*/
func QualifiesForOpportunityEntryFromCandidate(
	candidate EntryCandidate,
	thresholdCtx ThresholdContext,
) bool {
	if candidate.EdgeBps <= 0 {
		return false
	}

	if candidate.Confidence < thresholdCtx.EntryConfidenceBaseline {
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
