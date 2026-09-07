package equation

import (
	"math"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Median owns reusable sorting storage; callers receive only a scalar result. */
type Median struct {
	core.PrimitiveError
	values  []float64
	seed    *transport.IO
	current core.Primitive
}

/*
NewMedian averages the two central order statistics. Any NaN makes the result
undefined; infinities remain ordered values. Empty runs report a shape error.
*/
func NewMedian() core.Primitive {
	return &Median{seed: transport.NewIO(core.From(0.0))}
}

func (median *Median) Next(input core.Primitive) core.Primitive {
	median.values = median.values[:0]
	result := core.Yield(median.seed, input, func(_ float64, value float64) float64 {
		median.values = append(median.values, value)
		return value
	}, median)

	if result == nil || median.Error() != nil {
		return result
	}

	if len(median.values) == 0 {
		median.Error(core.ErrShape)
		return nil
	}
	slices.Sort(median.values)
	count := len(median.values)
	value := (0.0 + median.values[(count-1)/2] + median.values[count/2]) * 0.5

	// The previous IsNaN gate propagated any undefined member, even away
	// from the central order statistics. Sorting places that member first.
	if math.IsNaN(median.values[0]) {
		value = median.values[0]
	}
	median.current = core.From(value)
	return median.current
}

func (median *Median) Read() any { return core.To[any](median.current) }
