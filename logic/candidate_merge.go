package logic

/*
correlatedConfidence combines confidences conservatively for correlated evidence.
*/
func correlatedConfidence(values []float64, correlationPenalty float64) float64 {
	if len(values) == 0 {
		return 0
	}

	productMiss := 1.0

	for _, probability := range values {
		probability = clampUnit(probability, 0.01, 0.99)
		productMiss *= 1 - probability*correlationPenalty
	}

	return 1 - productMiss
}

func correlationPenaltyForSources(sources []SourceType) float64 {
	momentumSources := 0

	for _, source := range sources {
		switch source {
		case SourceCVD, SourcePumpDump, SourceDepthFlow, SourceExhaustion:
			momentumSources++
		default:
		}
	}

	if momentumSources >= 2 {
		return 0.35
	}

	if len(sources) >= 2 {
		return 0.55
	}

	return 0.75
}

func weightedMeanPositiveEdge(edges []float64, weights []float64) float64 {
	if len(edges) == 0 {
		return 0
	}

	numerator := 0.0
	denominator := 0.0

	for index, edge := range edges {
		if edge <= 0 {
			continue
		}

		weight := 1.0

		if index < len(weights) && weights[index] > 0 {
			weight = weights[index]
		}

		numerator += edge * weight
		denominator += weight
	}

	if denominator <= 0 {
		return 0
	}

	return numerator / denominator
}

func robustMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	return medianPositive(values)
}
