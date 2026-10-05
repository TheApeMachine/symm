package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
StandardizeInput is a value and the center/scale that locate it.
*/
type StandardizeInput struct {
	Value  float64
	Center float64
	Scale  float64
}

/*
Standardize owns (value - center) / scale.
*/
type Standardize struct {
	*core.PrimitiveError
	center float64
	scale  float64
	fixed  bool
	out    float64
}

/*
NewStandardize constructs a Standardize primitive.
If center and scale are provided, it centers and scales arriving *float64 values.
Otherwise, arriving values are *StandardizeInput.
*/
func NewStandardize(params ...float64) *Standardize {
	op := &Standardize{
		PrimitiveError: core.NewPrimitiveError(),
		scale:          1,
	}

	if len(params) >= 1 {
		op.center = params[0]
		op.fixed = true
	}

	if len(params) >= 2 {
		op.scale = params[1]
	}

	return op
}

func (op *Standardize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if op.fixed {
				val := *(*float64)(arriving)
				op.out = 0

				if op.scale != 0 {
					op.out = (val - op.center) / op.scale
				}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			input := *(*StandardizeInput)(arriving)
			op.out = 0

			if input.Scale != 0 {
				op.out = (input.Value - input.Center) / input.Scale
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
