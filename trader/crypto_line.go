package trader

import "math"

type entryLine struct {
	line   float64
	median float64
	mad    float64
}

func batchEntryLine(candidates []tradeCandidate) entryLine {
	scores := make([]float64, 0, len(candidates))

	for _, candidate := range candidates {
		if candidate.confidence <= 0 {
			continue
		}

		scores = append(scores, candidate.confidence)
	}

	if len(scores) == 0 {
		return entryLine{}
	}

	sorted := copySortedScores(scores)
	median := percentileScore(sorted, 0.5)
	mad := medianAbsoluteDeviation(sorted, median)
	line := median + mad

	if line <= 0 {
		line = median
	}

	return entryLine{
		line:   line,
		median: median,
		mad:    mad,
	}
}

func (crypto *Crypto) meetsEntryLine(candidate tradeCandidate, line entryLine) bool {
	if candidate.confidence <= 0 {
		return false
	}

	if line.line <= 0 {
		return candidate.confidence > 0
	}

	return candidate.confidence >= line.line
}

func medianAbsoluteDeviation(sorted []float64, median float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	deviations := make([]float64, len(sorted))

	for index, score := range sorted {
		deviations[index] = math.Abs(score - median)
	}

	sortScores(deviations)

	return percentileScore(deviations, 0.5)
}

func percentileScore(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if quantile <= 0 {
		return sorted[0]
	}

	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}

	position := quantile * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(position))
	upperIndex := int(math.Ceil(position))
	weight := position - float64(lowerIndex)

	return sorted[lowerIndex]*(1-weight) + sorted[upperIndex]*weight
}

func copySortedScores(values []float64) []float64 {
	cp := append([]float64(nil), values...)
	sortScores(cp)

	return cp
}

func sortScores(values []float64) {
	for index := 1; index < len(values); index++ {
		for inner := index; inner > 0 && values[inner] < values[inner-1]; inner-- {
			values[inner], values[inner-1] = values[inner-1], values[inner]
		}
	}
}
