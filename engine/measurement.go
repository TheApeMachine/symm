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
Confidence is in (0, 1) and describes how completely the current observation
matches the signal's criteria — not historical warmup or certainty.
*/
type Measurement struct {
	Type       MeasurementType
	Source     string
	Regime     string
	Reason     string
	Pairs      []asset.Pair
	Confidence float64
	Last       float64
	Bid        float64
	Ask        float64
	Timeframe  Timeframe
	Err        error
}

/*
AnchorPrice returns the best available reference price from Last or the quote mid.
*/
func (measurement Measurement) AnchorPrice() float64 {
	if measurement.Last > 0 {
		return measurement.Last
	}

	if measurement.Bid > 0 && measurement.Ask > 0 {
		return (measurement.Bid + measurement.Ask) / 2
	}

	return 0
}
