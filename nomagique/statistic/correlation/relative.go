package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Relative tracks the focal-to-cohort relative return energy rate against its
own adaptive baseline.
*/
type Relative struct {
	*core.PrimitiveError

	baseline core.Primitive
}

func NewRelative() *Relative {
	return &Relative{PrimitiveError: core.NewPrimitiveError(), baseline: adaptive.NewBaseline(adaptive.NewWindow())}
}

func (relative *Relative) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return relative.baseline.Next(in)
}
