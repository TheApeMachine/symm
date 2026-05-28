package engine

import "time"

type PredictionType uint8

const (
	PredictionTypePump PredictionType = iota
	PredictionTypeDump
	PredictionTypeMomentum
	PredictionTypeFlow
	PredictionTypeCausal
	PredictionTypeDepthFlow
	PredictionTypeLeadLag
	PredictionTypeBasis
	PredictionTypeSentiment
)

type Prediction struct {
	Type           PredictionType
	Perspective    Perspective
	Confidence     float64
	ExpectedReturn float64
	ActualReturn   float64
	Direction      int
	Runway         time.Duration
	DueAt          time.Time
	PredictedAt    time.Time
	Err            float64
}

/*
LeadMeasurement returns the highest-confidence priced observation recorded with the prediction.
*/
func (prediction Prediction) LeadMeasurement() (Measurement, bool) {
	best := Measurement{}

	for _, measurement := range prediction.Perspective.Measurements {
		if len(measurement.Pairs) == 0 {
			continue
		}

		if measurement.AnchorPrice() <= 0 {
			continue
		}

		if measurement.Confidence <= best.Confidence {
			continue
		}

		best = measurement
	}

	return best, best.Confidence > 0
}

/*
Error settles one due prediction against ground-truth price and returns the signed forecast error.
*/
func (prediction *Prediction) Error(groundTruth Measurement) (float64, bool) {
	lead, ok := prediction.LeadMeasurement()

	if !ok || len(groundTruth.Pairs) == 0 || len(lead.Pairs) == 0 {
		return 0, false
	}

	if groundTruth.Pairs[0].Wsname != lead.Pairs[0].Wsname {
		return 0, false
	}

	anchor := lead.AnchorPrice()
	lastPrice := groundTruth.AnchorPrice()

	if anchor <= 0 || lastPrice <= 0 {
		return 0, false
	}

	prediction.ActualReturn = float64(prediction.Direction) * (lastPrice - anchor) / anchor
	prediction.Err = prediction.ExpectedReturn - prediction.ActualReturn

	return prediction.Err, true
}
