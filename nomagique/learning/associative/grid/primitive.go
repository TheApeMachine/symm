package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

const FormatVersion float64 = 1.0

/*
Primitive projects input channels across associative regions, tracks region
activation deltas, and encodes them into ternary condition tokens.
*/
type Primitive struct {
	*core.PrimitiveError
	regions  int
	channels int
	weights  []float64
	previous []float64
}

func NewPrimitive(regions, channels int, weights []float64) *Primitive {
	prim := &Primitive{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
		channels:       channels,
		weights:        weights,
		previous:       make([]float64, regions),
	}

	if len(weights) != regions*channels {
		prim.Error(core.ErrShape)
	}

	return prim
}

func computeToken(quantity uint64, level, change float64) float64 {
	state := uint64(0)

	if level > 0 {
		state |= 1
	}

	if level < 0 {
		state |= 2
	}

	if change > 0 {
		state |= 1 << 2
	}

	if change < 0 {
		state |= 2 << 2
	}

	out := uint64(1<<52 | quantity<<4 | state)
	return float64(out)
}

func (op *Primitive) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var channelValues []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			channelValues = append(channelValues, *(*float64)(arriving))
		}

		if len(op.weights) != op.regions*op.channels || len(channelValues) != op.channels {
			op.Error(core.ErrShape)
			return
		}

		for regionIndex := 0; regionIndex < op.regions; regionIndex++ {
			level := 0.0
			offset := regionIndex * op.channels

			for channelIndex := 0; channelIndex < op.channels; channelIndex++ {
				level += op.weights[offset+channelIndex] * channelValues[channelIndex]
			}

			change := level - op.previous[regionIndex]
			op.previous[regionIndex] = level
			token := computeToken(uint64(regionIndex+1), level, change)

			for value := range data.NewValue(token, level, change).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
