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
)

type Timeframe struct {
	Start int64
	End   int64
}

/*
Measurement is the result of a measurement.
Confidence is a unitless score for ranking and UI.
ExpectedReturn is the model-estimated fractional price return over Runway.
*/
type Measurement struct {
	Type           MeasurementType
	Source         string
	Regime         string
	Reason         string
	Pairs          []asset.Pair
	Confidence     float64
	ExpectedReturn float64
	Runway         time.Duration
	Timeframe      Timeframe
	Err            error
}

/*
Direction returns +1 for buy-side forecasts and -1 for sell-side.
*/
func (measurementType MeasurementType) Direction() int {
	if measurementType == Dump {
		return -1
	}

	return 1
}

/*
Signal evaluates microstructure on demand and yields queued measurements.
Scan is invoked by the trader scheduler; Measure drains the passive queue.
*/
type Signal interface {
	Scan(now time.Time) error
	Measure(ctx context.Context) iter.Seq[Measurement]
	Source() string
	Stats() QueueStats
}
