package trader

import "math"

func (reading CognitiveReading) PriorMass() float64 {
	treeMass := 0.0

	if reading.LookaheadPaths > 0 && reading.ClassConfidence > 0 {
		treeMass = reading.ClassConfidence * math.Exp(reading.LookaheadScore)
	}

	corpusMass := 0.0

	if reading.CorpusMatchCount > 0 &&
		reading.TopSimilarity > 0 &&
		reading.PredictedReturnBps > 0 {
		corpusMass = reading.TopSimilarity * reading.PredictedReturnBps / 100
	}

	return treeMass + corpusMass
}
