package temporal

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Timestamp converts a time.Time arrival to signed Unix nanoseconds.
*/
type Timestamp struct {
	core.Base[time.Time, int64]
}

func NewTimestamp() *Timestamp {
	return &Timestamp{}
}

func (op *Timestamp) Next(
	in iter.Seq[core.Primitive[time.Time, time.Time]],
) iter.Seq[core.Primitive[int64, int64]] {
	return func(yield func(core.Primitive[int64, int64]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(arriving.Read().UnixNano())) {
				return
			}
		}
	}
}
