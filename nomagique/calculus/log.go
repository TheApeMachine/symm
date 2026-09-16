package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Log owns one field operation. What it hands over is the natural logarithm of each
arrival, operating in-place on the wire pointer.
*/
type Log struct {
	*core.PrimitiveError
}

func NewLog() *Log {
	return &Log{PrimitiveError: core.NewPrimitiveError()}
}

func (log *Log) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			*in = math.Log(*in)

			if !yield(arriving) {
				return
			}
		}
	}
}
