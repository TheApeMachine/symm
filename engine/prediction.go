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
