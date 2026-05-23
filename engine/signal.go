package engine

import (
	"context"
	"iter"

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
Signal is the interface that all signals must implement.
Run gathers measurements; Measure yields them for the trader.
*/
type Signal interface {
	Run()
	Measure(ctx context.Context) iter.Seq[Measurement]
}
