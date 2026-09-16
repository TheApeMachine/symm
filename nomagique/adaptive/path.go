package adaptive

import (
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
PathRetention owns the configured mean-shift policy for accepted observations.
*/
type PathRetention struct {
	*core.PrimitiveError

	window core.Primitive
	out    []temporal.Price
}

func NewPathRetention(window core.Primitive) *PathRetention {
	return &PathRetention{PrimitiveError: core.NewPrimitiveError(), window: window}
}

func (pathRetention *PathRetention) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if pathRetention.window != nil {
				if err := pathRetention.window.Error(); err != nil {
					pathRetention.Error(err)
				}
			}
		}()
		for arriving := range in {
			observations := *(*[]temporal.Price)(arriving)

			if len(observations) == 0 {
				pathRetention.Error(core.ErrShape)
				return
			}

			lastVal := observations[len(observations)-1].Value
			var capacity float64

			for wPtr := range pathRetention.window.Next(sequence.NewOne(unsafe.Pointer(&lastVal)).Next(nil)) {
				w := *(*WindowReading)(wPtr)
				capacity = w.Capacity
			}

			start := max(0, len(observations)-int(capacity))

			if start == 0 {
				pathRetention.out = observations
			} else {
				pathRetention.out = slices.Clone(observations[start:])
			}

			if !yield(unsafe.Pointer(&pathRetention.out)) {
				return
			}
		}
	}
}
