package resonance

import (
	"fmt"

	"github.com/theapemachine/symm/data"
)

/*
CoderArch is the predictive coder layer shape for featureDim inputs.
*/
func CoderArch(featureDim int) []int {
	return []int{featureDim, featureDim * 4, featureDim * 2, featureDim}
}

/*
PublishCoderReading encodes one predictive coder step as a resonance reading
Value (origin resonance, role reading, scope symbol).
*/
func PublishCoderReading(
	symbol string,
	tick *data.Value,
	out [12][]float64,
	arch []int,
) (*data.Value, error) {
	if tick == nil {
		return nil, fmt.Errorf("resonance: tick required to publish reading")
	}

	reading := data.NewValue(ReadingOrigin, ReadingRole, symbol)
	reading.Epoch, reading.Tick, reading.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp

	return publishReturns(reading, out, arch)
}
