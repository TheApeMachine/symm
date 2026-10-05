package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Delta measures the rate of change for region activations and unrolls them
into individual region condition tuples.
Input: *[]float64 (length = regions).
Yields: *[3]float64{regionID, level, change} for each region.
*/
type Delta struct {
	*core.PrimitiveError
	regions  int
	previous []float64
}

func NewDelta(regions int) core.Primitive {
	return &Delta{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
		previous:       make([]float64, regions),
	}
}

func (op *Delta) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			current := *(*[]float64)(arriving)

			if len(current) != op.regions {
				op.Error(core.ErrShape)
				return
			}

			for index := 0; index < op.regions; index++ {
				level := current[index]
				change := level - op.previous[index]
				regionID := float64(index + 1)
				tuple := [3]float64{regionID, level, change}

				if !yield(unsafe.Pointer(&tuple)) {
					return
				}
			}

			copy(op.previous, current)
		}
	}
}
