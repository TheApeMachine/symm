package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Log owns natural logarithm transformation of an arrival.
*/
type Log struct {
	*core.PrimitiveError
}

func NewLog() core.Primitive {
	return &Log{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Log) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			val := *(*float64)(arriving)
			if val <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			out := math.Log(val)

			for value := range data.NewValue(out).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
