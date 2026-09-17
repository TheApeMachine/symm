package store

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"golang.design/x/lockfree"
)

/*
KV executes Input requests against the supplied lock-free map. Writes update
that map directly; reads yield a borrowed result value. A write yields its
borrowed payload as acknowledgement. This primitive has one stream consumer;
separate instances may share a concurrency-safe backing map.
Origin matches the sender identity type in Input; Key and Value match the map.
*/
type KV[Origin any, Key comparable, Value any] struct {
	*core.PrimitiveError
	current lockfree.Map[Key, Value]
	out     Value
}

func NewKV[Origin any, Key comparable, Value any](current lockfree.Map[Key, Value]) *KV[Origin, Key, Value] {
	return &KV[Origin, Key, Value]{PrimitiveError: core.NewPrimitiveError(), current: current}
}

/*
Next processes each request and its actions in order, stopping at the first failure.
*/
func (kv *KV[Origin, Key, Value]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*core.Input[Origin, Key, Value])(arriving)

			if input.Action == core.None {
				kv.Error(fmt.Errorf("%w: KV request has no action", core.ErrDomain))
				return
			}

			switch input.Action {
			case core.Read:
				value, found := kv.current.Get(input.Key)

				if !found {
					kv.Error(fmt.Errorf("%w: KV key %v", core.ErrNotHeld, input.Key))
					return
				}

				kv.out = value

				if !yield(unsafe.Pointer(&kv.out)) {
					return
				}

			case core.Write:
				if input.Value == nil {
					kv.Error(fmt.Errorf("%w: KV write for key %v has no value", core.ErrShape, input.Key))
					return
				}

				kv.current.Set(input.Key, *input.Value)

				if !yield(unsafe.Pointer(input.Value)) {
					return
				}

			default:
				kv.Error(fmt.Errorf("%w: KV does not support action %d", core.ErrDomain, input.Action))
				return
			}
		}
	}
}
