package pumpdump

import (
	"sync"

	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// The pump/dump signal honors the original Signal contract: measurements are
// structured raw market data whose generating values accept top-down Feedback.
var _ market.Signal = (*Signal)(nil)

var lastMeasurementMu sync.RWMutex

func (signal *Signal) rememberMeasurement(measurement types.Measurement) {
	lastMeasurementMu.Lock()
	defer lastMeasurementMu.Unlock()

	signal.lastMeasurement = measurement
}

/*
Measure applies top-down feedback to the signal's latest observation and returns
the re-derived measurement: the feedback scale adjusts the raw ignition
observation BEFORE banding, so category, confidence and surprise all flow from
the corrected value. This is the market.Signal contract, finally implemented —
the streaming emission path applies the same correction live via
types.AdjustSourceValue on every publish.
*/
func (signal *Signal) Measure(feedback perspectives.Feedback) types.Measurement {
	lastMeasurementMu.RLock()
	measurement := signal.lastMeasurement
	lastMeasurementMu.RUnlock()

	if measurement.Symbol == "" || feedback == nil || feedback.Samples() <= 0 {
		return measurement
	}

	scale := feedback.Scale()

	if scale <= 0 {
		return measurement
	}

	if scale < 0.5 {
		scale = 0.5
	}

	if scale > 2 {
		scale = 2
	}

	observation := measurement.Strength * scale
	code, err := signal.classifier.Code(observation)

	if err != nil {
		return measurement
	}

	measurement.Strength = observation
	measurement.Confidence = signal.classifier.Confidence(observation)
	measurement.Category = signal.categories[signal.classifier.Label(code)]

	return measurement
}
