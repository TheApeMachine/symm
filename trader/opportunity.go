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
			tradableMeasurements := make([]engine.Measurement, 0, len(perspective.Measurements))
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

				if !predictions.Calibrated(measurement.Source) {
					continue
				}

				tradableMeasurements = append(tradableMeasurements, measurement)

				if predicted > maxPredicted {
					maxPredicted = predicted
					leadMeasurement = measurement
				}
			}

			jointConfidence, sourceCount := engine.FuseMeasurements(tradableMeasurements)
			edge := maxPredicted * jointConfidence

			if sourceCount < config.System.MinActivePerspectives {
				continue
			}

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
	Calibrated(source string) bool
}
