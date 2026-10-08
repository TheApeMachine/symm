package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Delta measures the rate of change for region activations.
*/
type Delta struct {
	*core.PrimitiveError
	regions  int
	previous []float64
}

func NewDelta(regions int) *Delta {
	effectiveRegions := max(1, regions)

	return &Delta{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
		previous:       make([]float64, effectiveRegions),
	}
}

func (op *Delta) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var levels []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			levels = append(levels, *(*float64)(arriving))
		}

		if len(levels) != op.regions {
			op.Error(core.ErrShape)
			return
		}

		for index := 0; index < op.regions; index++ {
			level := levels[index]
			change := level - op.previous[index]
			op.previous[index] = level
			regionId := float64(index + 1)

			for value := range data.NewValue(level, change, regionId).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
