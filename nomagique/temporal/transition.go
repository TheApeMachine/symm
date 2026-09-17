package temporal

import (
	"bytes"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Transition emits sequential transitions across consecutive signatures.
On the initial observation, it retains previous without emitting.
On each subsequent observation, it emits (previous, current) encoded as "previous->current".
*/
type Transition struct {
	*core.PrimitiveError
	previous []byte
	out      []byte
}

func NewTransition() *Transition {
	return &Transition{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (transition *Transition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if transition.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			current := *(*[]byte)(arriving)
			if len(current) == 0 {
				continue
			}

			if len(transition.previous) == 0 {
				transition.previous = bytes.Clone(current)
				continue
			}

			transition.out = bytes.Join([][]byte{transition.previous, current}, []byte("->"))
			transition.previous = bytes.Clone(current)

			if !yield(unsafe.Pointer(&transition.out)) {
				return
			}
		}
	}
}
