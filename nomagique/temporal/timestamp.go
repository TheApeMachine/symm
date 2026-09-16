package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Timestamp converts a time.Time arrival to signed Unix nanoseconds.
*/
type Timestamp struct {
	*core.PrimitiveError

	out int64
}

func NewTimestamp() *Timestamp {
	return &Timestamp{PrimitiveError: core.NewPrimitiveError()}
}

func (timestamp *Timestamp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			timestamp.out = (*time.Time)(arriving).UnixNano()

			if !yield(unsafe.Pointer(&timestamp.out)) {
				return
			}
		}
	}
}
