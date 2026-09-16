package calculus

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RelativeChangeInput is one prior/current pair.
*/
type RelativeChangeInput struct {
	Previous float64
	Current  float64
}

/*
RelativeChange owns one stateless transformation:

	(Current - Previous) / Previous

A zero previous value is undefined.
*/
type RelativeChange struct {
	*core.PrimitiveError

	out float64
}

func NewRelativeChange() *RelativeChange {
	return &RelativeChange{PrimitiveError: core.NewPrimitiveError()}
}

func (relativeChange *RelativeChange) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := *(*RelativeChangeInput)(arriving)

			if input.Previous == 0 {
				relativeChange.Error(fmt.Errorf("%w: relative change of a zero prior is undefined", core.ErrDomain))
				return
			}

			relativeChange.out = (input.Current - input.Previous) / input.Previous

			if !yield(unsafe.Pointer(&relativeChange.out)) {
				return
			}
		}
	}
}
