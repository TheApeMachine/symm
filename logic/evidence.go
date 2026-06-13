package logic

/*
SignalEvidence separates classification confidence from tradeable edge semantics.
CategoryConfidence reflects label quality; EdgeConfidence reflects calibrated edge > cost.
NoveltySurprise captures transition/KL novelty; EdgeSurprise captures forecast residual surprise.
*/
type SignalEvidence struct {
	Source             SourceType
	Category           CategoryType
	RawStrength        float64
	NormalizedEvidence float64
	CategoryConfidence float64
	EdgeConfidence     float64
	NoveltySurprise    float64
	EdgeSurprise       float64
	ExpectedMoveBps    float64
	CostBps            float64
}

/*
EvidenceFromMeasurement maps a published measurement into separated evidence fields.
*/
func EvidenceFromMeasurement(measurement Measurement, costBps float64) SignalEvidence {
	novelty := measurement.NoveltySurprise

	if novelty <= 0 {
		novelty = measurement.Surprise
	}

	edgeSurprise := measurement.EdgeSurprise

	if edgeSurprise <= 0 && measurement.Source == SourcePrediction {
		edgeSurprise = measurement.Surprise
	}

	edgeConfidence := measurement.EdgeConfidence

	if edgeConfidence <= 0 {
		edgeConfidence = measurement.Confidence
	}

	expectedMove := measurement.ExpectedMoveBps

	if expectedMove <= 0 && measurement.Source == SourcePrediction {
		expectedMove = measurement.Strength * 10000
	}

	return SignalEvidence{
		Source:             measurement.Source,
		Category:           measurement.Category,
		RawStrength:        measurement.Strength,
		NormalizedEvidence: measurement.Strength,
		CategoryConfidence: measurement.Confidence,
		EdgeConfidence:     edgeConfidence,
		NoveltySurprise:    novelty,
		EdgeSurprise:       edgeSurprise,
		ExpectedMoveBps:    expectedMove,
		CostBps:            costBps,
	}
}
