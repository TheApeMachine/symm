package probability

import (
	"fmt"
	"math"
)

/*
ArgmaxIndex returns the index of the largest value.
*/
func ArgmaxIndex(values []float64) int {
	if len(values) == 0 {
		return 0
	}

	bestIndex := 0
	bestValue := values[0]

	for index, value := range values[1:] {
		if value > bestValue {
			bestValue = value
			bestIndex = index + 1
		}
	}

	return bestIndex
}

/*
CompetitionMargin scores how decisively excess separates from a reference span.
*/
func CompetitionMargin(excess, span float64) (float64, error) {
	if excess <= 0 || span <= 0 {
		return 0, fmt.Errorf("probability: competition margin requires positive excess and span")
	}

	return excess / (excess + span), nil
}

/*
HypothesisSeparation returns the fraction of the dominant hypothesis that is not
matched by its strongest competitor. Equal competitors have zero separation;
an uncontested positive hypothesis has complete separation.
*/
func HypothesisSeparation(scores []float64) (float64, error) {
	if len(scores) < 2 {
		return 0, fmt.Errorf("probability: hypothesis separation requires competing scores")
	}

	dominant := 0.0
	competitor := 0.0

	for _, score := range scores {
		if score < 0 {
			return 0, fmt.Errorf("probability: hypothesis separation requires nonnegative scores")
		}

		if score >= dominant {
			competitor = dominant
			dominant = score
			continue
		}

		if score > competitor {
			competitor = score
		}
	}

	if dominant == 0 {
		return 0, nil
	}

	return (dominant - competitor) / dominant, nil
}

/*
MagnitudeMargin maps a positive magnitude into (0, 1).
*/
func MagnitudeMargin(value float64) (float64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("probability: magnitude margin requires positive value")
	}

	scale := math.Sqrt(value)

	return value / (scale + value), nil
}

/*
CategoryShareConfidence returns the selected category's share of total category
evidence. Each category carries one unit of pseudocount, so N equal scores yield
1/N, close calls stay near 1/N, and a lone category with finite evidence cannot
reach 1.0. categoryIndex is 1-based; when zero, the winning category is used.
*/
func CategoryShareConfidence(scores []float64, categoryIndex int) (float64, error) {
	if len(scores) == 0 {
		return 0, fmt.Errorf("probability: category share confidence requires scores")
	}

	index := ArgmaxIndex(scores)

	if categoryIndex > 0 && categoryIndex-1 < len(scores) {
		index = categoryIndex - 1
	}

	if index < 0 || index >= len(scores) {
		return 0, fmt.Errorf("probability: category share confidence index out of range")
	}

	selected := scores[index]

	if selected <= 0 {
		for _, score := range scores {
			if score > 0 {
				return 0, fmt.Errorf("probability: category share confidence requires positive selected evidence")
			}
		}

		return 1.0 / float64(len(scores)), nil
	}

	scaleExponent := categoryEvidenceExponent(scores)
	evidenceSum := 0.0

	for _, score := range scores {
		if score > 0 {
			evidenceSum += math.Ldexp(score, -scaleExponent)
		}
	}

	pseudocount := math.Ldexp(1, -scaleExponent)
	numerator := math.Ldexp(selected, -scaleExponent) + pseudocount
	denominator := evidenceSum + float64(len(scores))*pseudocount

	return numerator / denominator, nil
}

/*
CategoryEvidenceBaselines derives confidence gates from the current evidence
competition instead of from category count. The entry gate is the nearest
competitor's posterior share; the exit gate is the average non-winning evidence
floor, so protective decisions can react before the strongest competitor wins.
categoryIndex is 1-based; when zero, the winning category is used.
*/
func CategoryEvidenceBaselines(
	scores []float64,
	categoryIndex int,
) (float64, float64, float64, error) {
	if len(scores) == 0 {
		return 0, 0, 0, fmt.Errorf("probability: category baselines require scores")
	}

	index := ArgmaxIndex(scores)

	if categoryIndex > 0 && categoryIndex-1 < len(scores) {
		index = categoryIndex - 1
	}

	if index < 0 || index >= len(scores) {
		return 0, 0, 0, fmt.Errorf("probability: category baseline index out of range")
	}

	scaleExponent := categoryEvidenceExponent(scores)
	evidenceSum := 0.0
	runnerUp := 0.0
	nonWinningSum := 0.0
	nonWinningCount := 0

	for scoreIndex, score := range scores {
		if score <= 0 {
			if scoreIndex != index {
				nonWinningCount++
			}

			continue
		}

		scaledScore := math.Ldexp(score, -scaleExponent)
		evidenceSum += scaledScore

		if scoreIndex == index {
			continue
		}

		nonWinningCount++
		nonWinningSum += scaledScore

		if scaledScore > runnerUp {
			runnerUp = scaledScore
		}
	}

	pseudocount := math.Ldexp(1, -scaleExponent)
	denominator := evidenceSum + float64(len(scores))*pseudocount

	if denominator <= 0 {
		return 0, 0, 0, fmt.Errorf("probability: category baseline denominator required")
	}

	entryBaseline := (runnerUp + pseudocount) / denominator
	exitEvidence := runnerUp

	if nonWinningCount > 0 {
		exitEvidence = nonWinningSum / float64(nonWinningCount)
	}

	exitBaseline := (exitEvidence + pseudocount) / denominator
	confidenceBaseline := exitBaseline

	return confidenceBaseline, entryBaseline, exitBaseline, nil
}

/*
categoryEvidenceExponent returns a power-of-two exponent that bounds positive
evidence without changing any evidence-to-pseudocount ratio.
*/
func categoryEvidenceExponent(scores []float64) int {
	maxEvidence := 0.0

	for _, score := range scores {
		maxEvidence = math.Max(maxEvidence, score)
	}

	if maxEvidence <= 1 {
		return 0
	}

	_, scaleExponent := math.Frexp(maxEvidence)

	return scaleExponent
}
