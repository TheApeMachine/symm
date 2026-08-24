package types

import (
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
PublishMeasurement stamps the owning symbol and the engine tick onto a
measurement before publishing it to the measurements channel. Every kernel
funnels through here, so no measurement can enter the bus without its symbol
identity — the identity every downstream stage keys on.
*/
func PublishMeasurement(
	thesis *Thesis,
	measurements *runtime.Channel[*nmtypes.Measurement],
	symbol string,
	measurement *nmtypes.Measurement,
) {
	if measurement == nil {
		return
	}

	measurement.Symbol = symbol

	if thesis != nil {
		measurement.Tick = thesis.Tick
	}

	measurements.Publish(measurement)
}
