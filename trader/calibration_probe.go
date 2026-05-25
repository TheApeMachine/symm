package trader

import (
	"time"

	"github.com/theapemachine/symm/engine"
)

/*
recordCalibrationProbe stores a non-executable forward-return sample for cold models.
*/
func (scorer *Scorer) recordCalibrationProbe(
	state *PairState,
	measurement engine.Measurement,
	now time.Time,
	symbol string,
	reason string,
) {
	if reason != forecastRejectUncalibrated {
		return
	}

	baselineQuote, ok := scorer.quotePrice(symbol)

	if !ok {
		return
	}

	state.RecordCalibrationProbe(
		now,
		measurement,
		forecastRunway(measurement),
		baselineQuote,
	)
}
