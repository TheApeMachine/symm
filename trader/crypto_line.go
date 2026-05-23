package trader

import (
	"github.com/theapemachine/symm/stats"
)

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

	sorted := stats.CopySorted(scores)
	median := stats.PercentileSorted(sorted, 0.5)
	mad := stats.MedianAbsoluteDeviation(sorted, median)
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

func copySortedScores(values []float64) []float64 {
	return stats.CopySorted(values)
}

func sortScores(values []float64) {
	stats.SortFloats(values)
}

func percentileScore(sorted []float64, quantile float64) float64 {
	return stats.PercentileSorted(sorted, quantile)
}

func medianAbsoluteDeviation(sorted []float64, median float64) float64 {
	return stats.MedianAbsoluteDeviation(sorted, median)
}
