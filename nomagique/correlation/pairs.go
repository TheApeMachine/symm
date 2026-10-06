package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pairs will own every symbol's price path and measure arrivals against retained
peers once the Measurement peer/write API migration lands. Until then it is a
pass-through Primitive that preserves the constructor shape learning expects.
*/
type Pairs struct {
	*core.PrimitiveError
	estimator core.Primitive
}

func NewPairs(estimator core.Primitive) core.Primitive {
	return &Pairs{
		PrimitiveError: core.NewPrimitiveError(),
		estimator:      estimator,
	}
}

func (op *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Relations retains measured pair facts. Pending Measurement-store migration it
pass-through yields arrivals unchanged.
*/
type Relations struct {
	*core.PrimitiveError
}

func NewRelations() core.Primitive {
	return &Relations{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Relations) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}
