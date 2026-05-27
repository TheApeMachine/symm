package trader

import (
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

type tradeOpportunity struct {
	Symbol          string
	PerspectiveType engine.PerspectiveType
	JointConfidence float64
	SourceCount     int
	PredictedReturn float64
	Edge            float64
	Friction        float64
	Regime          string
	Measurement     engine.Measurement
	Perspective     engine.Perspective
}

type scoreSummary struct {
	Opportunity    tradeOpportunity
	Edge           float64
	PredictedSum   float64
	PredictedCount int
}

func scoreOpportunities(
	predictions predictionRecorder,
	perspectives map[string]map[engine.PerspectiveType]engine.Perspective,
	now time.Time,
) scoreSummary {
	summary := scoreSummary{}
	bestEdge := 0.0

	for symbol, byType := range perspectives {
		for perspectiveType, perspective := range byType {
			opportunity, predictedSum, predictedCount := scorePerspective(
				predictions, symbol, perspectiveType, perspective, now,
			)
			summary.PredictedSum += predictedSum
			summary.PredictedCount += predictedCount

			if opportunity.Edge <= bestEdge {
				continue
			}

			bestEdge = opportunity.Edge
			summary.Edge = opportunity.Edge
			summary.Opportunity = opportunity
		}
	}

	return summary
}

func scorePerspective(
	predictions predictionRecorder,
	symbol string,
	perspectiveType engine.PerspectiveType,
	perspective engine.Perspective,
	now time.Time,
) (tradeOpportunity, float64, int) {
	predicted := predictions.RecordPerspective(symbol, perspective, now)

	if predicted <= 0 {
		return tradeOpportunity{}, 0, 0
	}

	return perspectiveOpportunity(symbol, perspectiveType, perspective, predicted), predicted, 1
}

func perspectiveOpportunity(
	symbol string,
	perspectiveType engine.PerspectiveType,
	perspective engine.Perspective,
	predictedReturn float64,
) tradeOpportunity {
	measurements := supportedMeasurements(symbol, perspective)

	if len(measurements) == 0 {
		return tradeOpportunity{}
	}

	jointConfidence, sourceCount := engine.FuseMeasurements(measurements)
	leadMeasurement := leadMeasurement(measurements)
	friction := entryFrictionReturn(leadMeasurement)
	edge := predictedReturn - friction

	if edge <= 0 {
		return tradeOpportunity{}
	}

	return tradeOpportunity{
		Symbol:          symbol,
		PerspectiveType: perspectiveType,
		JointConfidence: jointConfidence,
		SourceCount:     sourceCount,
		PredictedReturn: predictedReturn,
		Edge:            edge,
		Friction:        friction,
		Regime:          leadMeasurement.Regime,
		Measurement:     leadMeasurement,
		Perspective:     perspective,
	}
}

func supportedMeasurements(
	symbol string,
	perspective engine.Perspective,
) []engine.Measurement {
	measurements := make([]engine.Measurement, 0, len(perspective.Measurements))

	for _, measurement := range perspective.Measurements {
		if len(measurement.Pairs) == 0 || measurement.Pairs[0].Wsname != symbol {
			continue
		}

		if measurement.Confidence <= 0 || anchorPrice(measurement) <= 0 {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements
}

func leadMeasurement(measurements []engine.Measurement) engine.Measurement {
	lead := measurements[0]

	for _, measurement := range measurements[1:] {
		if measurement.Confidence <= lead.Confidence {
			continue
		}

		lead = measurement
	}

	return lead
}

type predictionRecorder interface {
	RecordPerspective(symbol string, perspective engine.Perspective, now time.Time) float64
}

func entryFrictionReturn(measurement engine.Measurement) float64 {
	feePct := config.System.TakerFeePct * 2

	if config.System.UseMakerEntries {
		feePct = config.System.MakerFeePct + config.System.TakerFeePct
	}

	feeReturn := feePct / 100
	spreadReturn := quoteSpreadBPS(
		anchorPrice(measurement), measurement.Bid, measurement.Ask,
	) / 10000

	return feeReturn + spreadReturn/2
}
