package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
KV associates incoming keys and values with a configured map. It stores values
under keys and looks up values by key according to the configured Action.
*/
type KV struct {
	*core.PrimitiveError
	store map[string]core.Primitive
}

func NewKV() core.Primitive {
	op := &KV{
		PrimitiveError: core.NewPrimitiveError(),
		store:          make(map[string]core.Primitive),
	}

	return op
}

func (op *KV) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			message := *(*data.Message)(arriving)

			switch message.Action {
			case data.READ:
				out := op.store[message.Key]

				for v := range data.NewValue(out).Next(nil) {
					if !yield(v) {
						return
					}
				}
			case data.WRITE:
				op.store[message.Key] = message.Value

				for v := range message.Value.Next(nil) {
					if !yield(v) {
						return
					}
				}
			}
		}
	}
}
