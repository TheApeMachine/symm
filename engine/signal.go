package engine

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/symm/kraken/asset"
)

/*
MeasurementType is the type of measurement.
*/
type MeasurementType uint8

/*
MeasurementType constants.
*/
const (
	Pump MeasurementType = iota
	Dump
	Momentum
	Flow
	Causal
	DepthFlow
	LeadLag
	Basis
	Sentiment
)

/*
Direction returns the signed trade side implied by one measurement type.
*/
func (measurementType MeasurementType) Direction() int {
	if measurementType == Dump {
		return -1
	}

	return 1
}

type Timeframe struct {
	Start int64
	End   int64
}

/*
Measurement is one signal reading for downstream trading.
Confidence is a unitless score for ranking and UI; expected return and hold
horizon are derived in the trader, not in the signal.
*/
type Measurement struct {
	Type       MeasurementType
	Source     string
	Regime     string
	Reason     string
	Pairs      []asset.Pair
	Confidence float64
	Timeframe  Timeframe
	Err        error
}

/*
Signal emits regime measurements and ingests settled prediction feedback.
*/
type Signal interface {
	Source() string
	Measure(ctx context.Context, now time.Time) iter.Seq[Measurement]
	Feedback(feedback PredictionFeedback)
}
