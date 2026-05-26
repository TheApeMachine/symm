package engine

import "github.com/theapemachine/symm/kraken/asset"

type MeasurementType uint8

type Timeframe struct {
	Start int64
	End   int64
}

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
	Liquidity
)

/*
Measurement is one signal reading for downstream trading.
Confidence is unitless signal strength in (0, 1): relative to symbol-local history,
never certainty. Expected return and hold horizon are derived in the trader.
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
