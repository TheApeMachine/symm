package temporal

import (
	container "container/ring"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Velocity owns the previous observation. The first observation and
non-advancing time have zero rate with explicit definedness. The latest point
is always retained, including when its clock does not advance.
*/
type Velocity struct {
	*core.PrimitiveError
	store *container.Ring
	seen  bool
	out   float64
}

func NewVelocity() core.Primitive {
	return &Velocity{
		PrimitiveError: core.NewPrimitiveError(),
		store:          container.New(2),
	}
}

func (op *Velocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			pair := *(*[2]float64)(arriving)
			op.store.Value = pair

			if !op.seen {
				op.seen = true
				op.out = 0.0
				op.store = op.store.Next()

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}
				continue
			}

			prev := op.store.Prev().Value.([2]float64)
			dx := pair[0] - prev[0]
			dt := (pair[1] - prev[1]) / float64(time.Second)

			if dt > 0 {
				op.out = dx / dt
			} else {
				op.out = 0.0
			}

			op.store = op.store.Next()

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
