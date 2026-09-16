package sequence

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pair is corresponding values from two runs.
*/
type Pair[T, U any] struct {
	Left  T
	Right U
}

/*
Zip pairs corresponding yields from the inbound left run and the right run
held at construction. It stops when either run ends.
*/
type Zip[T, U any] struct {
	*core.PrimitiveError

	right iter.Seq[unsafe.Pointer]
}

/*
NewZip instantiates a Zip Primitive holding the right run.
*/
func NewZip[T, U any](right iter.Seq[unsafe.Pointer]) *Zip[T, U] {
	return &Zip[T, U]{PrimitiveError: core.NewPrimitiveError(), right: right}
}

func (zip *Zip[T, U]) Next(left iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		next, stop := iter.Pull(zip.right)
		defer stop()

		for arriving := range left {
			other, ok := next()

			if !ok {
				return
			}

			pair := Pair[T, U]{
				Left:  *(*T)(arriving),
				Right: *(*U)(other),
			}

			if !yield(unsafe.Pointer(&pair)) {
				return
			}
		}
	}
}
