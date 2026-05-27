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
			supportedMeasurements := make([]engine.Measurement, 0, len(perspective.Measurements))
			maxPredicted := 0.0
			leadMeasurement := engine.Measurement{}

			for _, measurement := range perspective.Measurements {
				predicted := predictions.Record(
					perspective,
					measurement,
					anchorPrice(measurement),
					now,
				)

				if predicted > 0 {
					summary.PredictedSum += predicted
					summary.PredictedCount++
				}

				if predicted <= 0 {
					continue
				}

				supportedMeasurements = append(supportedMeasurements, measurement)

				if predicted > maxPredicted {
					maxPredicted = predicted
					leadMeasurement = measurement
				}
			}

			jointConfidence, sourceCount := engine.FuseMeasurements(supportedMeasurements)
			grossEdge := maxPredicted
			friction := entryFrictionReturn(leadMeasurement)
			edge := grossEdge - friction

			if edge <= bestEdge {
				continue
			}

			bestEdge = edge
			summary.Edge = edge
			summary.Opportunity = tradeOpportunity{
				Symbol:          symbol,
				PerspectiveType: perspectiveType,
				JointConfidence: jointConfidence,
				SourceCount:     sourceCount,
				PredictedReturn: maxPredicted,
				Edge:            edge,
				Friction:        friction,
				Measurement:     leadMeasurement,
				Perspective:     perspective,
			}
		}
	}

	return summary
}

type predictionRecorder interface {
	Record(
		perspective engine.Perspective,
		measurement engine.Measurement,
		anchorPrice float64,
		now time.Time,
	) float64
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
