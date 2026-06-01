package perspectives

import "github.com/theapemachine/symm/numeric/adaptive"

/*
SurpriseFloor turns a signal's per-symbol classification clarity into the temporal
signal-to-noise ratio carried on Measurement.SNR: how many of its own recent
standard deviations the latest Confidence stands above the symbol's running clarity
baseline. Each signal owns one floor; the baseline is kept per symbol, so a sudden
jump in clarity spikes regardless of the symbol's raw scale, while a permanently
clear signal settles toward zero as that clarity becomes its own norm.
*/
type SurpriseFloor struct {
	field *adaptive.SNRField
}

/*
NewSurpriseFloor builds an empty per-symbol surprise floor.
*/
func NewSurpriseFloor() *SurpriseFloor {
	return &SurpriseFloor{field: adaptive.NewSNRField()}
}

/*
Score folds the measurement's Confidence into its symbol's noise floor and writes
the resulting surprise to SNR. It is the single definition of what SNR means: call
it once, at emit time, after Confidence and Symbol are set. A nil floor is a no-op,
leaving SNR at zero.
*/
func (floor *SurpriseFloor) Score(measurement *Measurement) {
	if floor == nil || measurement == nil {
		return
	}

	measurement.SNR = floor.field.Score(measurement.Symbol, measurement.Confidence)
}
