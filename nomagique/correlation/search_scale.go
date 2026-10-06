package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SearchScale owns sqrt(2 log(candidates) / observations). Each arrival is
[2]float64{candidates, observations}; it yields *float64.
*/
type SearchScale struct {
	*core.PrimitiveError
	out float64
}

func NewSearchScale() core.Primitive {
	return &SearchScale{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *SearchScale) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			input := *(*[2]float64)(arriving)
			op.out = math.Sqrt(2.0 * math.Log(input[0]) / input[1])

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
