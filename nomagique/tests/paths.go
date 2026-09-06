package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Path builds timestamped observation records for estimator and lag-profile tests.
The slices must describe the same observations; malformed fixtures fail here.
*/
func Path(timestamps []int64, prices []float64) []core.Primitive {
	if len(timestamps) != len(prices) {
		panic("path fixture requires one timestamp per price")
	}

	observations := make([]core.Primitive, len(prices))

	for index, price := range prices {
		observations[index] = Record(map[string]any{"at": timestamps[index], "value": price})
	}

	return observations
}

/*
Observation supplies one pair of paths to the production delivery protocol.
*/
func Observation(left, right []core.Primitive) core.Primitive {
	return transport.NewIO(Record(map[string]any{"left": left, "right": right}))
}
