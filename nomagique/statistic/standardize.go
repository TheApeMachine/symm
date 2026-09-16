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
	op := &Standardize{PrimitiveError: core.NewPrimitiveError(), scale: 1}
	if len(params) >= 2 {
		op.center = params[0]
		op.scale = params[1]
		op.fixed = true
	} else if len(params) == 1 {
		op.center = params[0]
		op.fixed = true
	}
	return op
}

func (standardize *Standardize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if standardize.fixed {
				val := *(*float64)(arriving)
				if standardize.scale == 0 {
					standardize.out = 0
				} else {
					standardize.out = (val - standardize.center) / standardize.scale
				}
			} else {
				input := *(*StandardizeInput)(arriving)
				if input.Scale == 0 {
					standardize.out = 0
				} else {
					standardize.out = (input.Value - input.Center) / input.Scale
				}
			}

			if !yield(unsafe.Pointer(&standardize.out)) {
				return
			}
		}
	}
}
