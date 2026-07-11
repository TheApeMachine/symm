package types

import "time"

/*
Forecasts holds calibrated observable predictions derived from typed physical readout.
Strategy consumes these instead of raw projection labels.
*/
type Forecasts struct {
	At               time.Time
	BidTouchSurvival float64
	AskTouchSurvival float64
	TimeToDepletion  float64
	SpreadNarrowing  float64
	MidMove          float64
	ExecutableReturn float64
	Replenishment    float64
	ImpactEstimate   float64
	Uncertainty      float64
}
