package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LagShape owns neighbour projection around a selected candidate. Each arrival is
*[2][]float64 where [0] is the flattened 10-float profile and
[1] is {index, span, spacing}; it yields
[3]float64{shapeDefined, prominence, curvature}.
*/
type LagShape struct {
	*core.PrimitiveError
	out [3]float64
}

func NewLagShape() core.Primitive {
	return &LagShape{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *LagShape) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := (*[2][]float64)(arriving)

			if len(input[1]) < 3 {
				op.Error(core.ErrShape)
				return
			}

			profile := input[0]
			index := input[1][0]
			span := input[1][1]
			spacing := input[1][2]
			idx := int(index)
			op.out = [3]float64{}
			limit := len(profile) / 10

			if index > 0 && index < span*2 && idx > 0 && idx < limit-1 {
				lower := (idx - 1) * 10
				upper := (idx + 1) * 10
				center := idx * 10

				if profile[lower+5] == 1 && profile[upper+5] == 1 {
					leftVal := math.Abs(profile[lower+9])
					centerVal := math.Abs(profile[center+9])
					rightVal := math.Abs(profile[upper+9])
					diff := 2.0*centerVal - leftVal - rightVal
					seconds := spacing * 1e-9

					op.out = [3]float64{1, diff / 2.0, diff / (seconds * seconds)}
				}
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
