package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

// KV owns values by address key. READ and WRITE retain their existing storage
// semantics. EVALUATE runs a key's persistent primitive against the message
// payload; its factory constructs fresh state exactly once for each new key.
// Callers must not mix stored data and executable state under the same key.
type KV struct {
	*core.PrimitiveError
	store   map[string]core.Primitive
	factory func() core.Primitive
}

func NewKV(factory ...func() core.Primitive) core.Primitive {
	op := &KV{
		PrimitiveError: core.NewPrimitiveError(),
		store:          make(map[string]core.Primitive),
	}

	if len(factory) > 1 {
		op.Error(core.ErrShape)
		return op
	}

	if len(factory) == 1 {
		op.factory = factory[0]
	}

	return op
}

func (op *KV) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil || in == nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			message := *(*data.Message)(arriving)

			switch message.Action {
			case data.NOOP:
				continue

			case data.READ:
				out := op.store[message.Key]

				for value := range data.NewValue(out).Next(nil) {
					if !yield(value) {
						return
					}
				}

			case data.WRITE:
				if message.Value == nil {
					op.Error(core.ErrShape)
					return
				}

				op.store[message.Key] = message.Value

				for value := range message.Value.Next(nil) {
					if !yield(value) {
						return
					}
				}

				if err := message.Value.Error(); err != nil {
					op.Error(err)
					return
				}

			case data.EVALUATE:
				if message.Value == nil || message.Key == "" {
					op.Error(core.ErrShape)
					return
				}

				target := op.store[message.Key]

				if target == nil {
					if op.factory == nil {
						op.Error(core.ErrNotHeld)
						return
					}

					target = op.factory()

					if target == nil {
						op.Error(core.ErrNotHeld)
						return
					}

					op.store[message.Key] = target
				}

				if err := target.Error(); err != nil {
					op.Error(err)
					return
				}

				for value := range target.Next(message.Value.Next(nil)) {
					if !yield(value) {
						return
					}
				}

				if err := op.Error(target.Error(), message.Value.Error()); err != nil {
					return
				}

			default:
				op.Error(core.ErrDomain)
				return
			}
		}
	}
}
