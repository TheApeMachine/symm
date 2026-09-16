package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Identity constructs I_n from an arriving dimension. The diagonal is written in
typed storage; there is no scalar graph per coefficient.
*/
type Identity struct {
	*core.PrimitiveError

	out [][]float64
}

func NewIdentity() *Identity {
	return &Identity{PrimitiveError: core.NewPrimitiveError()}
}

func (identity *Identity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			size := int(val)

			if float64(size) != val || size < 0 {
				identity.Error(core.ErrShape)
				return
			}

			identity.out = make([][]float64, size)
			values := make([]float64, size*size)

			for row := range identity.out {
				identity.out[row] = values[row*size : (row+1)*size]
				identity.out[row][row] = 1
			}

			if !yield(unsafe.Pointer(&identity.out)) {
				return
			}
		}
	}
}
